package store

import "context"

// ListReads returns the reads of one of the user's books, newest first.
func (s *Store) ListReads(ctx context.Context, userID, bookID int64) ([]Read, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id, r.book_id, r.started_on, r.finished_on, r.abandoned, r.created_at FROM reads r
		JOIN books b ON b.id = r.book_id WHERE r.book_id = ? AND b.user_id = ?
		ORDER BY (r.finished_on = '') DESC, COALESCE(NULLIF(r.finished_on, ''), r.started_on) DESC, r.id DESC`, bookID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Read{}
	for rows.Next() {
		var r Read
		var ab int
		if err := rows.Scan(&r.ID, &r.BookID, &r.StartedOn, &r.FinishedOn, &ab, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Abandoned = ab == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// readOwned checks that a read belongs to one of the user's books.
func (s *Store) readOwned(ctx context.Context, userID, readID int64) (int64, error) {
	var bookID int64
	err := s.DB.QueryRowContext(ctx, `SELECT r.book_id FROM reads r JOIN books b ON b.id = r.book_id WHERE r.id = ? AND b.user_id = ?`, readID, userID).Scan(&bookID)
	return bookID, notFound(err)
}

// AddRead records a past read of a book (a re-read, or dates filled in later).
func (s *Store) AddRead(ctx context.Context, userID int64, r *Read) error {
	if _, err := s.GetBook(ctx, userID, r.BookID); err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO reads (book_id, started_on, finished_on, abandoned) VALUES (?, ?, ?, ?)`, r.BookID, r.StartedOn, r.FinishedOn, boolInt(r.Abandoned))
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return s.touchBook(ctx, r.BookID)
}

// UpdateRead changes the dates of a read.
func (s *Store) UpdateRead(ctx context.Context, userID int64, r *Read) error {
	bookID, err := s.readOwned(ctx, userID, r.ID)
	if err != nil {
		return err
	}
	r.BookID = bookID
	if _, err := s.DB.ExecContext(ctx, `UPDATE reads SET started_on = ?, finished_on = ?, abandoned = ? WHERE id = ?`, r.StartedOn, r.FinishedOn, boolInt(r.Abandoned), r.ID); err != nil {
		return err
	}
	return s.touchBook(ctx, bookID)
}

// DeleteRead removes a read.
func (s *Store) DeleteRead(ctx context.Context, userID, readID int64) error {
	bookID, err := s.readOwned(ctx, userID, readID)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM reads WHERE id = ?`, readID); err != nil {
		return err
	}
	return s.touchBook(ctx, bookID)
}

func (s *Store) touchBook(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE books SET updated_at = ? WHERE id = ?`, Now(), id)
	return err
}
