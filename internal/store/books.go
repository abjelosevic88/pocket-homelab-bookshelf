package store

import (
	"context"
	"database/sql"
	"strings"
)

const bookCols = `b.id, b.user_id, b.title, b.subtitle, b.authors, b.isbn13, b.isbn10, b.pages, b.published, b.publisher,
	b.language, b.description, b.format, b.tags, b.cover, b.source, b.source_id, b.source_url, b.status, b.rating,
	b.review, b.favorite, b.current_page, b.queue_pos, b.planned_for, b.plan_note, b.series, b.series_no, b.series_qid, b.created_at, b.updated_at,
	COALESCE((SELECT r.started_on FROM reads r WHERE r.book_id = b.id ORDER BY (r.finished_on = '') DESC, r.id DESC LIMIT 1), ''),
	COALESCE((SELECT MAX(r.finished_on) FROM reads r WHERE r.book_id = b.id AND r.abandoned = 0 AND r.finished_on <> ''), ''),
	(SELECT COUNT(1) FROM reads r WHERE r.book_id = b.id AND r.abandoned = 0 AND r.finished_on <> ''),
	(SELECT COUNT(1) FROM quotes q WHERE q.book_id = b.id)`

func scanBook(row interface{ Scan(...any) error }) (*Book, error) {
	var b Book
	var fav int
	err := row.Scan(&b.ID, &b.UserID, &b.Title, &b.Subtitle, &b.Authors, &b.ISBN13, &b.ISBN10, &b.Pages, &b.Published, &b.Publisher,
		&b.Language, &b.Description, &b.Format, &b.Tags, &b.Cover, &b.Source, &b.SourceID, &b.SourceURL, &b.Status, &b.Rating,
		&b.Review, &fav, &b.CurrentPage, &b.QueuePos, &b.PlannedFor, &b.PlanNote, &b.Series, &b.SeriesNo, &b.SeriesQID, &b.CreatedAt, &b.UpdatedAt,
		&b.StartedOn, &b.FinishedOn, &b.ReadCount, &b.QuoteCount)
	if err != nil {
		return nil, notFound(err)
	}
	b.Favorite = fav == 1
	return &b, nil
}

// BookFilter narrows ListBooks. UserID is required.
type BookFilter struct {
	UserID   int64
	Status   string // one status, or "" for all
	Query    string
	Tag      string
	Author   string
	Year     int  // finished (not abandoned) in this year
	Favorite bool // favourites only
	ISBN     string
	Series   string
	Sort     string // recent | added | title | author | rating | finished | queue | series
	Limit    int
}

// tagMatch is a SQL fragment that matches one tag in the comma-separated list.
const tagMatch = `(',' || REPLACE(LOWER(b.tags), ', ', ',') || ',') LIKE ('%,' || LOWER(?) || ',%')`

// ListBooks returns the books on a user's shelf.
func (s *Store) ListBooks(ctx context.Context, f BookFilter) ([]Book, error) {
	where := []string{"b.user_id = ?"}
	args := []any{f.UserID}
	if f.Status != "" {
		where = append(where, "b.status = ?")
		args = append(args, f.Status)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, `(LOWER(b.title) LIKE ? OR LOWER(b.subtitle) LIKE ? OR LOWER(b.authors) LIKE ? OR LOWER(b.tags) LIKE ?
			OR LOWER(b.review) LIKE ? OR b.isbn13 LIKE ? OR b.isbn10 LIKE ?)`)
		args = append(args, like, like, like, like, like, like, like)
	}
	if f.Tag != "" {
		where = append(where, tagMatch)
		args = append(args, f.Tag)
	}
	if f.Author != "" {
		where = append(where, "LOWER(b.authors) LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Author)+"%")
	}
	if f.Year > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM reads r WHERE r.book_id = b.id AND r.abandoned = 0 AND substr(r.finished_on, 1, 4) = ?)`)
		args = append(args, itoa4(f.Year))
	}
	if f.Favorite {
		where = append(where, "b.favorite = 1")
	}
	if f.Series != "" {
		where = append(where, "LOWER(b.series) = LOWER(?)")
		args = append(args, f.Series)
	}
	if f.ISBN != "" {
		where = append(where, "(b.isbn13 = ? OR b.isbn10 = ?)")
		args = append(args, f.ISBN, f.ISBN)
	}
	order := "b.updated_at DESC, b.id DESC"
	switch f.Sort {
	case "added":
		order = "b.created_at DESC, b.id DESC"
	case "title":
		order = "LOWER(b.title), b.id"
	case "author":
		order = "LOWER(b.authors), LOWER(b.title)"
	case "rating":
		order = "b.rating DESC, LOWER(b.title)"
	case "finished":
		order = "33 DESC, b.updated_at DESC" // 33 = the derived finished_on column
	case "series":
		order = "LOWER(b.series), CAST(NULLIF(b.series_no, '') AS REAL), LOWER(b.title)"
	case "queue":
		order = "(b.planned_for = '') DESC, b.planned_for, b.queue_pos, b.id"
	}
	q := `SELECT ` + bookCols + ` FROM books b WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + order
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Book{}
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func itoa4(y int) string {
	b := []byte{'0', '0', '0', '0'}
	for i := 3; i >= 0 && y > 0; i-- {
		b[i] = byte('0' + y%10)
		y /= 10
	}
	return string(b)
}

// GetBook returns one of the user's books.
func (s *Store) GetBook(ctx context.Context, userID, id int64) (*Book, error) {
	return scanBook(s.DB.QueryRowContext(ctx, `SELECT `+bookCols+` FROM books b WHERE b.id = ? AND b.user_id = ?`, id, userID))
}

// NormalizeTags trims, de-duplicates and joins a comma-separated tag list.
func NormalizeTags(s string) string {
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return strings.Join(out, ", ")
}

// CreateBook inserts a book and opens/records a read to match its status.
// startedOn / finishedOn are optional dates for that read (YYYY-MM-DD).
func (s *Store) CreateBook(ctx context.Context, b *Book, startedOn, finishedOn string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if !ValidStatus(b.Status) {
		b.Status = StatusWant
	}
	b.Tags = NormalizeTags(b.Tags)
	if b.Status == StatusWant && b.QueuePos == 0 {
		b.QueuePos = nextQueuePos(ctx, tx, b.UserID)
	}
	now := Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO books (user_id, title, subtitle, authors, isbn13, isbn10, pages, published, publisher, language,
		description, format, tags, cover, source, source_id, source_url, status, rating, review, favorite, current_page, queue_pos,
		planned_for, plan_note, series, series_no, series_qid, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.UserID, b.Title, b.Subtitle, b.Authors, b.ISBN13, b.ISBN10, b.Pages, b.Published, b.Publisher, b.Language,
		b.Description, b.Format, b.Tags, b.Cover, b.Source, b.SourceID, b.SourceURL, b.Status, b.Rating, b.Review, boolInt(b.Favorite),
		b.CurrentPage, b.QueuePos, b.PlannedFor, b.PlanNote, b.Series, b.SeriesNo, b.SeriesQID, now, now)
	if err != nil {
		return err
	}
	b.ID, _ = res.LastInsertId()
	b.CreatedAt, b.UpdatedAt = now, now
	switch b.Status {
	case StatusReading:
		if startedOn == "" {
			startedOn = Today()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO reads (book_id, started_on) VALUES (?, ?)`, b.ID, startedOn)
	case StatusRead, StatusAbandoned:
		// A finished book with no known finish date gets no read row: it is
		// "read", just not counted in any particular year.
		if finishedOn != "" {
			_, err = tx.ExecContext(ctx, `INSERT INTO reads (book_id, started_on, finished_on, abandoned) VALUES (?, ?, ?, ?)`,
				b.ID, startedOn, finishedOn, boolInt(b.Status == StatusAbandoned))
		}
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.syncBookAuthors(ctx, b.UserID, b.ID, b.Authors)
}

func nextQueuePos(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, userID int64) int {
	var n int
	_ = q.QueryRowContext(ctx, `SELECT COALESCE(MAX(queue_pos), 0) + 1 FROM books WHERE user_id = ? AND status = 'want'`, userID).Scan(&n)
	return n
}

// UpdateBook saves the editable fields. Status changes go through SetStatus.
func (s *Store) UpdateBook(ctx context.Context, b *Book) error {
	b.Tags = NormalizeTags(b.Tags)
	b.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE books SET title = ?, subtitle = ?, authors = ?, isbn13 = ?, isbn10 = ?, pages = ?, published = ?,
		publisher = ?, language = ?, description = ?, format = ?, tags = ?, source = ?, source_id = ?, source_url = ?, rating = ?, review = ?,
		favorite = ?, current_page = ?, planned_for = ?, plan_note = ?, series = ?, series_no = ?, series_qid = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		b.Title, b.Subtitle, b.Authors, b.ISBN13, b.ISBN10, b.Pages, b.Published, b.Publisher, b.Language, b.Description, b.Format, b.Tags,
		b.Source, b.SourceID, b.SourceURL, b.Rating, b.Review, boolInt(b.Favorite), b.CurrentPage, b.PlannedFor, b.PlanNote, b.Series, b.SeriesNo, b.SeriesQID, b.UpdatedAt, b.ID, b.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.syncBookAuthors(ctx, b.UserID, b.ID, b.Authors)
}

// SetCover records the cover file name of a book.
func (s *Store) SetCover(ctx context.Context, userID, id int64, name string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE books SET cover = ?, updated_at = ? WHERE id = ? AND user_id = ?`, name, Now(), id, userID)
	return err
}

// DeleteBook removes a book with its reads and quotes.
func (s *Store) DeleteBook(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM books WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return pruneAuthors(ctx, s.DB, userID)
}

// SetStatus moves a book to another status and keeps the reads in step:
//
//	→ reading    opens a read (started today/date) unless one is open
//	→ read       closes the open read, or records a read finished on date
//	→ abandoned  same, marked abandoned
//	→ want       drops an open read and puts the book at the end of the queue
func (s *Store) SetStatus(ctx context.Context, userID, id int64, to, date string) (*Book, error) {
	if !ValidStatus(to) {
		return nil, ErrNotFound
	}
	if date == "" {
		date = Today()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var from string
	var pages int
	if err := tx.QueryRowContext(ctx, `SELECT status, pages FROM books WHERE id = ? AND user_id = ?`, id, userID).Scan(&from, &pages); err != nil {
		return nil, notFound(err)
	}
	var openID int64
	_ = tx.QueryRowContext(ctx, `SELECT id FROM reads WHERE book_id = ? AND finished_on = '' AND abandoned = 0 ORDER BY id DESC LIMIT 1`, id).Scan(&openID)
	now := Now()
	exec := func(q string, args ...any) {
		if err == nil {
			_, err = tx.ExecContext(ctx, q, args...)
		}
	}
	switch to {
	case StatusReading:
		if openID == 0 {
			exec(`INSERT INTO reads (book_id, started_on) VALUES (?, ?)`, id, date)
		}
		if from == StatusRead || from == StatusAbandoned {
			exec(`UPDATE books SET current_page = 0 WHERE id = ?`, id)
		}
	case StatusRead, StatusAbandoned:
		ab := boolInt(to == StatusAbandoned)
		if openID != 0 {
			exec(`UPDATE reads SET finished_on = ?, abandoned = ? WHERE id = ?`, date, ab, openID)
		} else if from != to {
			exec(`INSERT INTO reads (book_id, finished_on, abandoned) VALUES (?, ?, ?)`, id, date, ab)
		}
		if to == StatusRead && pages > 0 {
			exec(`UPDATE books SET current_page = ? WHERE id = ?`, pages, id)
		}
	case StatusWant:
		if openID != 0 {
			exec(`DELETE FROM reads WHERE id = ?`, openID)
		}
		if from != StatusWant {
			exec(`UPDATE books SET queue_pos = ? WHERE id = ?`, nextQueuePos(ctx, tx, userID), id)
		}
	}
	exec(`UPDATE books SET status = ?, updated_at = ? WHERE id = ?`, to, now, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBook(ctx, userID, id)
}

// ReorderQueue sets the plan order of want-to-read books to the given order.
func (s *Store) ReorderQueue(ctx context.Context, userID int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE books SET queue_pos = ? WHERE id = ? AND user_id = ?`, i+1, id, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TagCount is a tag with the number of books carrying it.
type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ListTags returns every tag on the user's shelf, most used first.
func (s *Store) ListTags(ctx context.Context, userID int64) ([]TagCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tags FROM books WHERE user_id = ? AND tags <> ''`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	names := map[string]string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		for _, tag := range strings.Split(t, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			k := strings.ToLower(tag)
			counts[k]++
			if _, ok := names[k]; !ok {
				names[k] = tag
			}
		}
	}
	return sortedCounts(counts, names, 0), rows.Err()
}

// ImportBook inserts a book exactly as given, with its reads and quotes and
// without the automatic read bookkeeping of CreateBook.
func (s *Store) ImportBook(ctx context.Context, b *Book, reads []Read, quotes []Quote) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if !ValidStatus(b.Status) {
		b.Status = StatusWant
	}
	b.Tags = NormalizeTags(b.Tags)
	if b.Status == StatusWant && b.QueuePos == 0 {
		b.QueuePos = nextQueuePos(ctx, tx, b.UserID)
	}
	now := Now()
	if b.CreatedAt == "" {
		b.CreatedAt = now
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO books (user_id, title, subtitle, authors, isbn13, isbn10, pages, published, publisher, language,
		description, format, tags, cover, source, source_id, source_url, status, rating, review, favorite, current_page, queue_pos,
		planned_for, plan_note, series, series_no, series_qid, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.UserID, b.Title, b.Subtitle, b.Authors, b.ISBN13, b.ISBN10, b.Pages, b.Published, b.Publisher, b.Language,
		b.Description, b.Format, b.Tags, "", b.Source, b.SourceID, b.SourceURL, b.Status, b.Rating, b.Review, boolInt(b.Favorite),
		b.CurrentPage, b.QueuePos, b.PlannedFor, b.PlanNote, b.Series, b.SeriesNo, b.SeriesQID, b.CreatedAt, now)
	if err != nil {
		return err
	}
	b.ID, _ = res.LastInsertId()
	for _, r := range reads {
		if _, err := tx.ExecContext(ctx, `INSERT INTO reads (book_id, started_on, finished_on, abandoned) VALUES (?, ?, ?, ?)`, b.ID, r.StartedOn, r.FinishedOn, boolInt(r.Abandoned)); err != nil {
			return err
		}
	}
	for _, q := range quotes {
		if q.CreatedAt == "" {
			q.CreatedAt = now
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO quotes (book_id, text, page, note, created_at) VALUES (?, ?, ?, ?, ?)`, b.ID, q.Text, q.Page, q.Note, q.CreatedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.syncBookAuthors(ctx, b.UserID, b.ID, b.Authors)
}
