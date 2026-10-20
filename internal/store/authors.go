package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
)

// Author is a writer on someone's shelf.
type Author struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Name       string `json:"name"`
	OLKey      string `json:"ol_key"`
	Bio        string `json:"bio"`
	BioFull    string `json:"bio_full"` // longer biography for "Read more"; "" = only Bio
	BioSource  string `json:"bio_source"`
	BirthDate  string `json:"birth_date"`
	DeathDate  string `json:"death_date"`
	Photo      string `json:"photo"`
	Wikipedia  string `json:"wikipedia"`
	Followed   bool   `json:"followed"`
	Edited     bool   `json:"edited"`
	EnrichedAt string `json:"enriched_at"`
	CreatedAt  string `json:"created_at"`

	// Derived from the shelf, read-only.
	Books     int     `json:"books"`
	Read      int     `json:"read"`
	Reading   int     `json:"reading"`
	Want      int     `json:"want"`
	AvgRating float64 `json:"avg_rating"`
	Pages     int     `json:"pages"`
	LastRead  string  `json:"last_read"`
}

// SplitAuthors splits a books.authors string into names.
func SplitAuthors(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range strings.Split(s, ",") {
		n = strings.TrimSpace(n)
		k := fold.Name(n)
		if n == "" || k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	return out
}

const authorCols = `a.id, a.user_id, a.name, a.ol_key, a.bio, a.bio_full, a.bio_source, a.birth_date, a.death_date, a.photo, a.wikipedia,
	a.followed, a.edited, a.enriched_at, a.created_at,
	COUNT(b.id), COUNT(CASE WHEN b.status = 'read' THEN 1 END), COUNT(CASE WHEN b.status = 'reading' THEN 1 END),
	COUNT(CASE WHEN b.status = 'want' THEN 1 END), COALESCE(ROUND(AVG(NULLIF(b.rating, 0)), 2), 0),
	COALESCE(SUM(CASE WHEN b.status = 'read' THEN b.pages END), 0),
	COALESCE((SELECT MAX(r.finished_on) FROM reads r JOIN book_authors x ON x.book_id = r.book_id WHERE x.author_id = a.id AND r.abandoned = 0), '')`

const authorFrom = ` FROM authors a LEFT JOIN book_authors ba ON ba.author_id = a.id LEFT JOIN books b ON b.id = ba.book_id`

func scanAuthor(row interface{ Scan(...any) error }) (*Author, error) {
	var a Author
	var followed, edited int
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.OLKey, &a.Bio, &a.BioFull, &a.BioSource, &a.BirthDate, &a.DeathDate, &a.Photo, &a.Wikipedia,
		&followed, &edited, &a.EnrichedAt, &a.CreatedAt, &a.Books, &a.Read, &a.Reading, &a.Want, &a.AvgRating, &a.Pages, &a.LastRead)
	if err != nil {
		return nil, notFound(err)
	}
	a.Followed, a.Edited = followed == 1, edited == 1
	return &a, nil
}

// ListAuthors returns the user's authors with shelf statistics.
func (s *Store) ListAuthors(ctx context.Context, userID int64, query, sort string) ([]Author, error) {
	q := `SELECT ` + authorCols + authorFrom + ` WHERE a.user_id = ?`
	args := []any{userID}
	if t := strings.TrimSpace(query); t != "" {
		q += ` AND (a.name_key LIKE ? OR LOWER(a.name) LIKE ?)`
		args = append(args, "%"+fold.Name(t)+"%", "%"+strings.ToLower(t)+"%")
	}
	// Ordinals refer to authorCols: 16 books, 17 read, 20 avg rating, 22 last finished.
	order := `17 DESC, 16 DESC, a.name_key` // most read
	switch sort {
	case "name":
		order = `a.name_key`
	case "added":
		order = `a.created_at DESC`
	case "rating":
		order = `20 DESC, 17 DESC, a.name_key`
	case "recent":
		order = `22 DESC, a.name_key`
	}
	q += ` GROUP BY a.id ORDER BY ` + order
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Author{}
	for rows.Next() {
		a, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetAuthor returns one of the user's authors.
func (s *Store) GetAuthor(ctx context.Context, userID, id int64) (*Author, error) {
	return scanAuthor(s.DB.QueryRowContext(ctx, `SELECT `+authorCols+authorFrom+` WHERE a.id = ? AND a.user_id = ? GROUP BY a.id`, id, userID))
}

// FindAuthorByName looks an author up by (folded) name.
func (s *Store) FindAuthorByName(ctx context.Context, userID int64, name string) (*Author, error) {
	return scanAuthor(s.DB.QueryRowContext(ctx, `SELECT `+authorCols+authorFrom+` WHERE a.user_id = ? AND a.name_key = ? GROUP BY a.id`, userID, fold.Name(name)))
}

// AuthorBooks lists the user's books by an author.
func (s *Store) AuthorBooks(ctx context.Context, userID, authorID int64) ([]Book, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+bookCols+` FROM books b JOIN book_authors ba ON ba.book_id = b.id
		WHERE ba.author_id = ? AND b.user_id = ? ORDER BY (b.status = 'read') DESC, 33 DESC, b.created_at DESC`, authorID, userID)
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

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ensureAuthor returns the id of the author with that name, creating it.
func ensureAuthor(ctx context.Context, q execer, userID int64, name string) (int64, error) {
	key := fold.Name(name)
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM authors WHERE user_id = ? AND name_key = ?`, userID, key).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := q.ExecContext(ctx, `INSERT INTO authors (user_id, name, name_key) VALUES (?, ?, ?)`, userID, name, key)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateAuthor adds an author by hand (followed, so kept without books).
func (s *Store) CreateAuthor(ctx context.Context, a *Author) error {
	id, err := ensureAuthor(ctx, s.DB, a.UserID, a.Name)
	if err != nil {
		return err
	}
	a.ID = id
	_, err = s.DB.ExecContext(ctx, `UPDATE authors SET followed = 1, ol_key = CASE WHEN ol_key = '' THEN ? ELSE ol_key END, enriched_at = CASE WHEN ? <> '' AND ol_key = '' THEN '' ELSE enriched_at END, updated_at = ? WHERE id = ?`,
		a.OLKey, a.OLKey, Now(), id)
	return err
}

// syncBookAuthors links a book to the authors named in its authors string
// and removes authors left without books (unless added by hand).
func (s *Store) syncBookAuthors(ctx context.Context, userID, bookID int64, authors string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := syncBookAuthorsTx(ctx, tx, userID, bookID, authors); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.authorsChanged()
	return nil
}
func syncBookAuthorsTx(ctx context.Context, tx *sql.Tx, userID, bookID int64, authors string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_authors WHERE book_id = ?`, bookID); err != nil {
		return err
	}
	for i, name := range SplitAuthors(authors) {
		id, err := ensureAuthor(ctx, tx, userID, name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO book_authors (book_id, author_id, position) VALUES (?, ?, ?)`, bookID, id, i); err != nil {
			return err
		}
	}
	if err := pruneAuthors(ctx, tx, userID); err != nil {
		return err
	}
	return nil
}

func pruneAuthors(ctx context.Context, q execer, userID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM authors WHERE user_id = ? AND followed = 0
		AND NOT EXISTS (SELECT 1 FROM book_authors ba WHERE ba.author_id = authors.id)`, userID)
	return err
}

func (s *Store) authorsChanged() {
	if s.OnAuthorsChanged != nil {
		s.OnAuthorsChanged()
	}
}

// SyncAllAuthors links every book to its authors (first start after the
// upgrade, and a cheap consistency pass on every start).
func (s *Store) SyncAllAuthors(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.user_id, b.authors FROM books b
		WHERE b.authors <> '' AND NOT EXISTS (SELECT 1 FROM book_authors ba WHERE ba.book_id = b.id)`)
	if err != nil {
		return err
	}
	type row struct {
		id, user int64
		authors  string
	}
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.user, &r.authors); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, r)
	}
	rows.Close()
	for _, r := range todo {
		if err := s.syncBookAuthors(ctx, r.user, r.id, r.authors); err != nil {
			return err
		}
	}
	return nil
}

// UpdateAuthor saves the editable fields. A new name is written into every
// linked book's authors string; renaming onto an existing author merges them.
func (s *Store) UpdateAuthor(ctx context.Context, a *Author) error {
	cur, err := s.GetAuthor(ctx, a.UserID, a.ID)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	newKey := fold.Name(a.Name)
	if newKey != fold.Name(cur.Name) || a.Name != cur.Name {
		// Rewrite the name in the books that carry it.
		rows, err := tx.QueryContext(ctx, `SELECT b.id, b.authors FROM books b JOIN book_authors ba ON ba.book_id = b.id WHERE ba.author_id = ?`, a.ID)
		if err != nil {
			return err
		}
		type bk struct {
			id      int64
			authors string
		}
		var books []bk
		for rows.Next() {
			var x bk
			if err := rows.Scan(&x.id, &x.authors); err != nil {
				rows.Close()
				return err
			}
			books = append(books, x)
		}
		rows.Close()
		for _, x := range books {
			names := SplitAuthors(x.authors)
			for i, n := range names {
				if fold.Name(n) == fold.Name(cur.Name) {
					names[i] = a.Name
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE books SET authors = ?, updated_at = ? WHERE id = ?`, strings.Join(names, ", "), Now(), x.id); err != nil {
				return err
			}
		}
		var other int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM authors WHERE user_id = ? AND name_key = ? AND id <> ?`, a.UserID, newKey, a.ID).Scan(&other); err == nil {
			// Merge into the existing author: move the links, drop this one.
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO book_authors (book_id, author_id, position) SELECT book_id, ?, position FROM book_authors WHERE author_id = ?`, other, a.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM authors WHERE id = ?`, a.ID); err != nil {
				return err
			}
			a.ID = other
			return tx.Commit()
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE authors SET name = ?, name_key = ?, bio = ?, bio_full = ?, bio_source = ?, birth_date = ?, death_date = ?, wikipedia = ?,
		ol_key = ?, followed = ?, edited = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		a.Name, newKey, a.Bio, a.BioFull, a.BioSource, a.BirthDate, a.DeathDate, a.Wikipedia, a.OLKey, boolInt(a.Followed), boolInt(a.Edited), Now(), a.ID, a.UserID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SaveEnrichment stores what the catalogues know about an author.
func (s *Store) SaveEnrichment(ctx context.Context, a *Author) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE authors SET ol_key = ?, bio = ?, bio_full = ?, bio_source = ?, birth_date = ?, death_date = ?, wikipedia = ?,
		enriched_at = ?, updated_at = ? WHERE id = ?`, a.OLKey, a.Bio, a.BioFull, a.BioSource, a.BirthDate, a.DeathDate, a.Wikipedia, Now(), Now(), a.ID)
	return err
}

// MarkEnriched records a lookup that found nothing, so it is not repeated.
func (s *Store) MarkEnriched(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE authors SET enriched_at = ? WHERE id = ?`, Now(), id)
	return err
}

// ResetEnrichment queues an author for a fresh lookup (optionally with a new key).
func (s *Store) ResetEnrichment(ctx context.Context, userID, id int64, olKey string) error {
	q := `UPDATE authors SET enriched_at = '', edited = 0 WHERE id = ? AND user_id = ?`
	args := []any{id, userID}
	if olKey != "" {
		q = `UPDATE authors SET enriched_at = '', edited = 0, ol_key = ? WHERE id = ? AND user_id = ?`
		args = []any{olKey, id, userID}
	}
	_, err := s.DB.ExecContext(ctx, q, args...)
	return err
}

// SetAuthorPhoto records the photo file of an author.
func (s *Store) SetAuthorPhoto(ctx context.Context, id int64, name string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE authors SET photo = ?, updated_at = ? WHERE id = ?`, name, Now(), id)
	return err
}

// PendingAuthors returns authors not looked up yet (all users).
func (s *Store) PendingAuthors(ctx context.Context, limit int) ([]Author, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+authorCols+authorFrom+` WHERE a.enriched_at = '' GROUP BY a.id ORDER BY a.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Author
	for rows.Next() {
		a, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// AuthorWorkKeys returns the Open Library work keys and titles of the user's
// books by an author, for marking a bibliography.
func (s *Store) AuthorWorkKeys(ctx context.Context, userID, authorID int64) (map[string]int64, []Book, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.source, b.source_id, b.title FROM books b JOIN book_authors ba ON ba.book_id = b.id
		WHERE ba.author_id = ? AND b.user_id = ?`, authorID, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	keys := map[string]int64{}
	var books []Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Source, &b.SourceID, &b.Title); err != nil {
			return nil, nil, err
		}
		if b.Source == "openlibrary" && b.SourceID != "" {
			keys[b.SourceID] = b.ID
		}
		books = append(books, b)
	}
	return keys, books, rows.Err()
}

// DeleteAuthor removes an author that has no books on the shelf.
func (s *Store) DeleteAuthor(ctx context.Context, userID, id int64) error {
	a, err := s.GetAuthor(ctx, userID, id)
	if err != nil {
		return err
	}
	if a.Books > 0 {
		return ErrConflict
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM authors WHERE id = ? AND user_id = ?`, id, userID)
	return err
}
