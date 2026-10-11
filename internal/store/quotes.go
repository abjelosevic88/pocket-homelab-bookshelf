package store

import (
	"context"
	"strings"
)

// QuoteFilter narrows ListQuotes. UserID is required.
type QuoteFilter struct {
	UserID int64
	BookID int64
	Query  string
	Random bool // one random quote
	Limit  int
}

// ListQuotes returns saved quotes, newest first (or one at random).
func (s *Store) ListQuotes(ctx context.Context, f QuoteFilter) ([]Quote, error) {
	where := []string{"b.user_id = ?"}
	args := []any{f.UserID}
	if f.BookID > 0 {
		where = append(where, "q.book_id = ?")
		args = append(args, f.BookID)
	}
	if t := strings.TrimSpace(f.Query); t != "" {
		like := "%" + strings.ToLower(t) + "%"
		where = append(where, "(LOWER(q.text) LIKE ? OR LOWER(q.note) LIKE ? OR LOWER(b.title) LIKE ? OR LOWER(b.authors) LIKE ?)")
		args = append(args, like, like, like, like)
	}
	order := "q.created_at DESC, q.id DESC"
	if f.BookID > 0 {
		order = "q.id"
	}
	if f.Random {
		order, f.Limit = "RANDOM()", 1
	}
	query := `SELECT q.id, q.book_id, q.text, q.page, q.note, q.created_at, b.title, b.authors, b.cover
		FROM quotes q JOIN books b ON b.id = q.book_id WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + order
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Quote{}
	for rows.Next() {
		var q Quote
		if err := rows.Scan(&q.ID, &q.BookID, &q.Text, &q.Page, &q.Note, &q.CreatedAt, &q.BookTitle, &q.BookAuthors, &q.BookCover); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// CreateQuote saves a quote on one of the user's books.
func (s *Store) CreateQuote(ctx context.Context, userID int64, q *Quote) error {
	if _, err := s.GetBook(ctx, userID, q.BookID); err != nil {
		return err
	}
	q.CreatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO quotes (book_id, text, page, note, created_at) VALUES (?, ?, ?, ?, ?)`, q.BookID, q.Text, q.Page, q.Note, q.CreatedAt)
	if err != nil {
		return err
	}
	q.ID, _ = res.LastInsertId()
	return nil
}

// UpdateQuote edits a quote.
func (s *Store) UpdateQuote(ctx context.Context, userID int64, q *Quote) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE quotes SET text = ?, page = ?, note = ? WHERE id = ? AND book_id IN (SELECT id FROM books WHERE user_id = ?)`,
		q.Text, q.Page, q.Note, q.ID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteQuote removes a quote.
func (s *Store) DeleteQuote(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM quotes WHERE id = ? AND book_id IN (SELECT id FROM books WHERE user_id = ?)`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
