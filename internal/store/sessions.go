package store

import "context"

// ReaderEvent is one progress update (or a device opening the book) for a
// document that is linked to a book on the shelf.
type ReaderEvent struct {
	Document   string
	Source     string // device | crosspoint | open
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
	q := `SELECT e.document, e.source, e.device, e.percentage, e.timestamp, b.id, b.title, b.authors,
			CASE WHEN b.pages > 0 THEN b.pages ELSE d.page_count END
		FROM kosync_events e
		JOIN kosync_documents d ON d.user_id = e.user_id AND d.document = e.document
		JOIN books b ON b.id = d.book_id AND b.user_id = e.user_id
		WHERE e.user_id = ? AND d.ignored = 0 AND e.timestamp > 0`
	args := []any{userID}
	if bookID > 0 {
		q += ` AND b.id = ?`
		args = append(args, bookID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY e.timestamp, e.id`, args...)
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
