package store

import "context"

// ReaderEvent is one progress update (or a device opening the book) for a
// document that is linked to a book on the shelf.
type ReaderEvent struct {
	Document   string
	Source     string // device | crosspoint | open | library (Pocket Library's feed)
	Device     string
	Percentage float64
	Timestamp  int64 // unix seconds
	BookID     int64
	BookTitle  string
	BookAuth   string
	BookPages  int // the book's own page count, else what CrossPoint Sync says
}

// ReaderEvents lists the progress updates of the person's linked documents,
// oldest first; bookID 0 means every book.
func (s *Store) ReaderEvents(ctx context.Context, userID, bookID int64) ([]ReaderEvent, error) {
	// this server's KOSync, and Pocket Library's feed (its web reader and its
	// own e-reader sync) as one document per book
	kf, lf := "", ""
	args := []any{userID}
	if bookID > 0 {
		kf, lf = ` AND b.id = ?`, ` AND b.id = ?`
		args = append(args, bookID)
	}
	args = append(args, args...)
	q := `SELECT document, source, device, percentage, ts, book_id, title, authors, pages FROM (
		SELECT e.document AS document, e.source AS source, e.device AS device, e.percentage AS percentage, e.timestamp AS ts, e.id AS ev,
			b.id AS book_id, b.title AS title, b.authors AS authors, CASE WHEN b.pages > 0 THEN b.pages ELSE d.page_count END AS pages
		FROM kosync_events e
		JOIN kosync_documents d ON d.user_id = e.user_id AND d.document = e.document
		JOIN books b ON b.id = d.book_id AND b.user_id = e.user_id
		WHERE e.user_id = ? AND d.ignored = 0 AND e.timestamp > 0` + kf + `
		UNION ALL
		SELECT 'library:' || b.id, p.source, p.device, p.percentage, p.timestamp, -p.id, b.id, b.title, b.authors, b.pages
		FROM progress_events p JOIN books b ON b.id = p.book_id AND b.user_id = p.user_id
		WHERE p.user_id = ? AND p.source = 'library' AND p.timestamp > 0` + lf + `
	) ORDER BY ts, ev`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReaderEvent
	for rows.Next() {
		var e ReaderEvent
		if err := rows.Scan(&e.Document, &e.Source, &e.Device, &e.Percentage, &e.Timestamp, &e.BookID, &e.BookTitle, &e.BookAuth, &e.BookPages); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
