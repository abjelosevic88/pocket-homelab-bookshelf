package store

import (
	"context"
	"database/sql"
	"errors"
)

// AudiobookLink is the book on the person's shelves an audiobook was taken in
// as; found is false when it never was. A book id that no longer exists means
// the person removed the book.
func (s *Store) AudiobookLink(ctx context.Context, userID int64, itemID string) (bookID int64, found bool) {
	err := s.DB.QueryRowContext(ctx, `SELECT book_id FROM audiobook_links WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&bookID)
	return bookID, err == nil
}

// SetAudiobookLink remembers which book an audiobook is.
func (s *Store) SetAudiobookLink(ctx context.Context, userID int64, itemID string, bookID int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO audiobook_links (user_id, item_id, book_id) VALUES (?, ?, ?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET book_id = excluded.book_id`, userID, itemID, bookID)
	return err
}

// AudiobookEventSeen reports whether a finish or start was already taken in.
func (s *Store) AudiobookEventSeen(ctx context.Context, userID int64, itemID, kind, day string) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM audiobook_events WHERE user_id = ? AND item_id = ? AND kind = ? AND day = ?`, userID, itemID, kind, day).Scan(&n)
	return n > 0
}

// MarkAudiobookEvent records a finish or start as taken in.
func (s *Store) MarkAudiobookEvent(ctx context.Context, userID int64, itemID, kind, day string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO audiobook_events (user_id, item_id, kind, day) VALUES (?, ?, ?, ?)`, userID, itemID, kind, day)
	return err
}

// FinishFromAudiobook records a finished listen as a read: it closes the open
// read when there is one (keeping its start), else adds a read unless that
// day's finish is already there, and puts the book on "read". Changed is false
// when nothing had to change.
func (s *Store) FinishFromAudiobook(ctx context.Context, userID, bookID int64, startedOn, finishedOn string) (changed bool, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM books WHERE id = ? AND user_id = ?`, bookID, userID).Scan(&status); err != nil {
		return false, notFound(err)
	}
	var openID int64
	var openStart string
	err = tx.QueryRowContext(ctx, `SELECT id, started_on FROM reads WHERE book_id = ? AND finished_on = '' AND abandoned = 0 ORDER BY id DESC LIMIT 1`, bookID).Scan(&openID, &openStart)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if openID != 0 {
		if openStart == "" {
			openStart = startedOn
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reads SET started_on = ?, finished_on = ? WHERE id = ?`, openStart, finishedOn, openID); err != nil {
			return false, err
		}
		changed = true
	} else {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM reads WHERE book_id = ? AND finished_on = ?`, bookID, finishedOn).Scan(&n); err != nil {
			return false, err
		}
		if n == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO reads (book_id, started_on, finished_on) VALUES (?, ?, ?)`, bookID, startedOn, finishedOn); err != nil {
				return false, err
			}
			changed = true
		}
	}
	if status != StatusRead {
		if _, err := tx.ExecContext(ctx, `UPDATE books SET status = ?, current_page = CASE WHEN pages > 0 THEN pages ELSE current_page END WHERE id = ?`, StatusRead, bookID); err != nil {
			return false, err
		}
		changed = true
	}
	if changed {
		if _, err := tx.ExecContext(ctx, `UPDATE books SET updated_at = ? WHERE id = ?`, Now(), bookID); err != nil {
			return false, err
		}
	}
	return changed, tx.Commit()
}

// StartFromAudiobook puts a book the person is listening to on "reading"
// (with a read from the day they started) when it was only wanted. A book
// already reading, read or abandoned is left alone.
func (s *Store) StartFromAudiobook(ctx context.Context, userID, bookID int64, startedOn string) (changed bool, err error) {
	b, err := s.GetBook(ctx, userID, bookID)
	if err != nil {
		return false, err
	}
	if b.Status != StatusWant {
		return false, nil
	}
	_, err = s.SetStatus(ctx, userID, bookID, StatusReading, startedOn)
	return err == nil, err
}

// UserKey reads one of a person's settings ("" when unset).
func (s *Store) UserKey(ctx context.Context, userID int64, key string) string {
	var v string
	_ = s.DB.QueryRowContext(ctx, `SELECT value FROM user_settings WHERE user_id = ? AND key = ?`, userID, key).Scan(&v)
	return v
}

// PutUserKey saves one of a person's settings.
func (s *Store) PutUserKey(ctx context.Context, userID int64, key, value string) error {
	return s.putUserSettings(ctx, userID, map[string]string{key: value})
}
