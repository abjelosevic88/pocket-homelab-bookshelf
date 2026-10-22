package store

import (
	"context"
	"strings"
	"time"
)

// Summary is a book's spoiler-free "what it is about" paragraph in one language.
type Summary struct {
	Status    string `json:"status"` // ok | unknown
	Text      string `json:"text"`
	Source    string `json:"source"`
	UpdatedAt string `json:"updated_at"`
}

// Summaries returns one of the user's books' summaries by language.
func (s *Store) Summaries(ctx context.Context, userID, bookID int64) (map[string]Summary, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT m.lang, m.status, m.text, m.source, m.updated_at FROM book_summaries m
		JOIN books b ON b.id = m.book_id WHERE m.book_id = ? AND b.user_id = ?`, bookID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Summary{}
	for rows.Next() {
		var lang string
		var m Summary
		if err := rows.Scan(&lang, &m.Status, &m.Text, &m.Source, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out[lang] = m
	}
	return out, rows.Err()
}

// PutSummary saves (replaces) a book's summary in one language.
func (s *Store) PutSummary(ctx context.Context, userID, bookID int64, lang string, m Summary) error {
	if err := s.bookOwned(ctx, userID, bookID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO book_summaries (book_id, lang, status, text, source, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id, lang) DO UPDATE SET status = excluded.status, text = excluded.text, source = excluded.source, updated_at = excluded.updated_at`,
		bookID, lang, m.Status, m.Text, m.Source, time.Now().UTC().Format(time.RFC3339))
	return err
}

// DeleteSummaries forgets a book's summaries, so they are written again.
func (s *Store) DeleteSummaries(ctx context.Context, userID, bookID int64) error {
	if err := s.bookOwned(ctx, userID, bookID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM book_summaries WHERE book_id = ?`, bookID)
	return err
}

// MissingSummaries lists the user's books lacking a summary in any of langs,
// with the languages they lack; books being read first, then the newest.
func (s *Store) MissingSummaries(ctx context.Context, userID int64, langs []string) ([]Book, [][]string, error) {
	books, err := s.ListBooks(ctx, BookFilter{UserID: userID, Sort: "added"})
	if err != nil {
		return nil, nil, err
	}
	have := map[int64]map[string]bool{}
	rows, err := s.DB.QueryContext(ctx, `SELECT m.book_id, m.lang FROM book_summaries m JOIN books b ON b.id = m.book_id WHERE b.user_id = ?`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var lang string
		if err := rows.Scan(&id, &lang); err != nil {
			return nil, nil, err
		}
		if have[id] == nil {
			have[id] = map[string]bool{}
		}
		have[id][lang] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var outB, later []Book
	var outL, laterL [][]string
	for _, b := range books {
		var miss []string
		for _, l := range langs {
			if l = strings.TrimSpace(l); l != "" && !have[b.ID][l] {
				miss = append(miss, l)
			}
		}
		if len(miss) == 0 {
			continue
		}
		if b.Status == StatusReading {
			outB, outL = append(outB, b), append(outL, miss)
		} else {
			later, laterL = append(later, b), append(laterL, miss)
		}
	}
	return append(outB, later...), append(outL, laterL...), nil
}
