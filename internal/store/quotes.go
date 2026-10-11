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

func squashText(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// ExistingQuoteTexts returns the normalised texts of a book's quotes, for
// telling which incoming highlights are already there.
func (s *Store) ExistingQuoteTexts(ctx context.Context, bookID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT text FROM quotes WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, squashText(t))
	}
	return out, rows.Err()
}

// QuoteIsKnown reports whether text is already among existing (same text, or
// contained in a longer saved one).
func QuoteIsKnown(existing []string, text string) bool {
	t := squashText(text)
	for _, e := range existing {
		if e == t || strings.Contains(e, t) {
			return true
		}
	}
	return false
}

// ImportQuotes adds quotes to one of the user's books, keeping their
// created_at. A quote already saved (same text, or part of a longer saved one)
// is skipped; a longer version of a saved quote replaces its text, keeping a
// note written in the app. Safe to run again with the same input.
func (s *Store) ImportQuotes(ctx context.Context, userID, bookID int64, in []Quote) (added, updated, skipped int, err error) {
	if _, err := s.GetBook(ctx, userID, bookID); err != nil {
		return 0, 0, 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	type saved struct {
		id         int64
		text, note string
	}
	var have []saved
	rows, err := tx.QueryContext(ctx, `SELECT id, text, note FROM quotes WHERE book_id = ?`, bookID)
	if err != nil {
		return 0, 0, 0, err
	}
	for rows.Next() {
		var q saved
		if err := rows.Scan(&q.id, &q.text, &q.note); err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		have = append(have, q)
	}
	rows.Close()
next:
	for _, q := range in {
		t := squashText(q.Text)
		if t == "" {
			continue
		}
		for i, h := range have {
			ht := squashText(h.text)
			switch {
			case ht == t || strings.Contains(ht, t):
				if h.note == "" && q.Note != "" {
					if _, err := tx.ExecContext(ctx, `UPDATE quotes SET note = ? WHERE id = ?`, q.Note, h.id); err != nil {
						return 0, 0, 0, err
					}
					have[i].note = q.Note
					updated++
				} else {
					skipped++
				}
				continue next
			case strings.Contains(t, ht):
				note := h.note
				if note == "" {
					note = q.Note
				}
				if _, err := tx.ExecContext(ctx, `UPDATE quotes SET text = ?, page = ?, note = ? WHERE id = ?`, q.Text, q.Page, note, h.id); err != nil {
					return 0, 0, 0, err
				}
				have[i].text, have[i].note = q.Text, note
				updated++
				continue next
			}
		}
		if q.CreatedAt == "" {
			q.CreatedAt = Now()
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO quotes (book_id, text, page, note, created_at) VALUES (?, ?, ?, ?, ?)`, bookID, q.Text, q.Page, q.Note, q.CreatedAt)
		if err != nil {
			return 0, 0, 0, err
		}
		id, _ := res.LastInsertId()
		have = append(have, saved{id: id, text: q.Text, note: q.Note})
		added++
	}
	if added+updated > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE books SET updated_at = ? WHERE id = ?`, Now(), bookID); err != nil {
			return 0, 0, 0, err
		}
	}
	return added, updated, skipped, tx.Commit()
}
