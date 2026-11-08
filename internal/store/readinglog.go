package store

import (
	"context"
	"strings"
)

// ListeningSession is one listening session from Pocket Audiobooks; BookID
// and the book's details are filled when the audiobook is on the shelves.
type ListeningSession struct {
	SessionID string
	ItemID    string
	Day       string
	StartedAt string // RFC 3339
	UpdatedAt string
	Seconds   int
	StartPos  int
	EndPos    int
	Duration  int
	Device    string

	BookID    int64
	BookTitle string
	BookAuth  string
}

// SaveListeningSessions keeps the sessions Pocket Audiobooks reported since
// day from. With prune, the person's sessions since then that it no longer
// reports (forgotten there) are dropped.
func (s *Store) SaveListeningSessions(ctx context.Context, userID int64, from string, ss []ListeningSession, prune bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	keep := make([]any, 0, len(ss)+2)
	keep = append(keep, userID, from)
	for _, x := range ss {
		if _, err := tx.ExecContext(ctx, `INSERT INTO listening_sessions (user_id, session_id, item_id, day, started_at, updated_at, seconds, start_pos, end_pos, duration, device)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, session_id) DO UPDATE SET item_id = excluded.item_id, day = excluded.day, started_at = excluded.started_at,
				updated_at = excluded.updated_at, seconds = excluded.seconds, start_pos = excluded.start_pos, end_pos = excluded.end_pos,
				duration = excluded.duration, device = excluded.device`,
			userID, x.SessionID, x.ItemID, x.Day, x.StartedAt, x.UpdatedAt, x.Seconds, x.StartPos, x.EndPos, x.Duration, x.Device); err != nil {
			return err
		}
		keep = append(keep, x.SessionID)
	}
	if prune {
		q := `DELETE FROM listening_sessions WHERE user_id = ? AND day >= ?`
		if len(ss) > 0 {
			q += ` AND session_id NOT IN (?` + strings.Repeat(",?", len(ss)-1) + `)`
		}
		if _, err := tx.ExecContext(ctx, q, keep...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListeningSessions lists the person's listening sessions (half a minute or
// more) of audiobooks on their shelves, oldest first; bookID 0 means every book.
func (s *Store) ListeningSessions(ctx context.Context, userID, bookID int64) ([]ListeningSession, error) {
	q := `SELECT l.session_id, l.item_id, l.day, l.started_at, l.updated_at, l.seconds, l.start_pos, l.end_pos, l.duration, l.device, b.id, b.title, b.authors
		FROM listening_sessions l
		JOIN audiobook_links a ON a.user_id = l.user_id AND a.item_id = l.item_id
		JOIN books b ON b.id = a.book_id AND b.user_id = l.user_id
		WHERE l.user_id = ? AND l.seconds >= 30` // a tap and stop is not a session
	args := []any{userID}
	if bookID > 0 {
		q += ` AND b.id = ?`
		args = append(args, bookID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY l.started_at, l.session_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ListeningSession
	for rows.Next() {
		var x ListeningSession
		if err := rows.Scan(&x.SessionID, &x.ItemID, &x.Day, &x.StartedAt, &x.UpdatedAt, &x.Seconds, &x.StartPos, &x.EndPos, &x.Duration, &x.Device,
			&x.BookID, &x.BookTitle, &x.BookAuth); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// PageLog is a page the person set by hand (in Bookshelf, the app, the API).
type PageLog struct {
	BookID    int64
	BookTitle string
	BookAuth  string
	FromPage  int
	Page      int
	Pages     int
	At        int64 // unix seconds
}

// AddPageLog records a page set by hand, moving on from fromPage.
func (s *Store) AddPageLog(ctx context.Context, userID, bookID int64, fromPage, page, pages int, at int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO page_logs (user_id, book_id, from_page, page, pages, at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, bookID, fromPage, page, pages, at)
	return err
}

// PageLogs lists the pages the person set by hand, oldest first; bookID 0 means every book.
func (s *Store) PageLogs(ctx context.Context, userID, bookID int64) ([]PageLog, error) {
	q := `SELECT p.book_id, b.title, b.authors, p.from_page, p.page, p.pages, p.at
		FROM page_logs p JOIN books b ON b.id = p.book_id AND b.user_id = p.user_id WHERE p.user_id = ?`
	args := []any{userID}
	if bookID > 0 {
		q += ` AND p.book_id = ?`
		args = append(args, bookID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY p.at, p.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PageLog
	for rows.Next() {
		var p PageLog
		if err := rows.Scan(&p.BookID, &p.BookTitle, &p.BookAuth, &p.FromPage, &p.Page, &p.Pages, &p.At); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
