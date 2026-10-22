package store

import (
	"context"
	"database/sql"
)

// ActivityRow is how far the reader got in one document on one day.
type ActivityRow struct {
	Day      string  `json:"day"`
	Document string  `json:"document"`
	Pages    int     `json:"pages"`
	Syncs    int     `json:"syncs"`
	From     float64 `json:"from"`
	To       float64 `json:"to"`
}

// ReplaceKosyncActivity stores the person's reading activity, replacing what was there.
func (s *Store) ReplaceKosyncActivity(ctx context.Context, userID int64, rows []ActivityRow) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `DELETE FROM kosync_activity WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO kosync_activity (user_id, day, document, pages, syncs, from_pct, to_pct) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, day, document) DO UPDATE SET pages = excluded.pages, syncs = excluded.syncs, from_pct = excluded.from_pct, to_pct = excluded.to_pct`,
			userID, r.Day, r.Document, r.Pages, r.Syncs, r.From, r.To); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BookActivity is a day of reading one book: the documents linked to it
// together, with pages counted on the book's own page count when it has one.
type BookActivity struct {
	Day   string  `json:"day"`
	Pages int     `json:"pages"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
}

// ReadingDay is all the reading on one day.
type ReadingDay struct {
	Day   string `json:"day"`
	Pages int    `json:"pages"`
	Books int    `json:"books"`
}

// pagesOn counts a day's pages: the share of the book read times its page
// count, else CrossPoint Sync's own count.
const pagesOn = `CASE WHEN b.pages > 0 AND a.to_pct > a.from_pct THEN CAST(ROUND((a.to_pct - a.from_pct) * b.pages) AS INTEGER) ELSE a.pages END`

// ActivityForBook lists the days the person read a book on an e-reader, oldest first.
func (s *Store) ActivityForBook(ctx context.Context, userID, bookID int64) ([]BookActivity, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.day, SUM(`+pagesOn+`), MIN(a.from_pct), MAX(a.to_pct)
		FROM kosync_activity a JOIN kosync_documents d ON d.user_id = a.user_id AND d.document = a.document
		JOIN books b ON b.id = d.book_id
		WHERE a.user_id = ? AND b.id = ? AND b.user_id = ? GROUP BY a.day ORDER BY a.day`, userID, bookID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BookActivity{}
	for rows.Next() {
		var a BookActivity
		if err := rows.Scan(&a.Day, &a.Pages, &a.From, &a.To); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ReadingDays lists the days with reading on an e-reader since from (YYYY-MM-DD), oldest first.
func (s *Store) ReadingDays(ctx context.Context, userID int64, from string) ([]ReadingDay, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.day, SUM(`+pagesOn+`), COUNT(DISTINCT a.document)
		FROM kosync_activity a LEFT JOIN kosync_documents d ON d.user_id = a.user_id AND d.document = a.document
		LEFT JOIN books b ON b.id = d.book_id
		WHERE a.user_id = ? AND a.day >= ? AND (a.syncs > 0 OR a.pages > 0) GROUP BY a.day ORDER BY a.day`, userID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReadingDay{}
	for rows.Next() {
		var d ReadingDay
		var pages sql.NullInt64
		if err := rows.Scan(&d.Day, &pages, &d.Books); err != nil {
			return nil, err
		}
		d.Pages = int(pages.Int64)
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkKosyncFinishAsked records that the person was asked whether the document's book is finished.
func (s *Store) MarkKosyncFinishAsked(ctx context.Context, userID int64, document string) (first bool, err error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE kosync_documents SET finish_asked = 1 WHERE user_id = ? AND document = ? AND finish_asked = 0`, userID, document)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// KosyncDocumentsForBook lists the documents linked to a book.
func (s *Store) KosyncDocumentsForBook(ctx context.Context, userID, bookID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT document FROM kosync_documents WHERE user_id = ? AND book_id = ? AND ignored = 0`, userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkAIFacts records that the AI was asked about a book's series and original title.
func (s *Store) MarkAIFacts(ctx context.Context, bookID int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO book_ai_facts (book_id, asked_at) VALUES (?, ?)`, bookID, Now())
	return err
}

// BooksWithoutAIFacts lists the user's books with no series or original title
// that the AI was not asked about yet.
func (s *Store) BooksWithoutAIFacts(ctx context.Context, userID int64) ([]Book, error) {
	books, err := s.ListBooks(ctx, BookFilter{UserID: userID, Sort: "added"})
	if err != nil {
		return nil, err
	}
	asked := map[int64]bool{}
	rows, err := s.DB.QueryContext(ctx, `SELECT f.book_id FROM book_ai_facts f JOIN books b ON b.id = f.book_id WHERE b.user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		asked[id] = true
	}
	var out []Book
	for _, b := range books {
		if !asked[b.ID] && (b.Series == "" || b.OriginalTitle == "") {
			out = append(out, b)
		}
	}
	return out, rows.Err()
}
