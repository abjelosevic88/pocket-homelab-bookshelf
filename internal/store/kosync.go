package store

import (
	"context"
	"database/sql"
)

// KosyncAccount is the username and key a person's e-readers sync with.
type KosyncAccount struct {
	UserID    int64  `json:"-"`
	Username  string `json:"username"`
	KeyHash   string `json:"-"` // bcrypt of the MD5 hex of the sync password
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen"`
}

// KosyncDocument is one file read on an e-reader: its latest position and the
// book on the shelf it belongs to.
type KosyncDocument struct {
	Document   string  `json:"document"`
	Progress   string  `json:"progress"`
	Percentage float64 `json:"percentage"`
	Device     string  `json:"device"`
	DeviceID   string  `json:"device_id"`
	Timestamp  int64   `json:"timestamp"`
	Title      string  `json:"title"`
	Authors    string  `json:"authors"`
	Filename   string  `json:"filename"`
	BookID     *int64  `json:"book_id"`
	Ignored    bool    `json:"ignored"`

	// Joined, read-only.
	BookTitle   string `json:"book_title"`
	BookAuthors string `json:"book_authors"`
}

const kosyncAccountCols = `user_id, username, key_hash, created_at, last_seen`

func scanKosyncAccount(row interface{ Scan(...any) error }) (*KosyncAccount, error) {
	var a KosyncAccount
	if err := row.Scan(&a.UserID, &a.Username, &a.KeyHash, &a.CreatedAt, &a.LastSeen); err != nil {
		return nil, notFound(err)
	}
	return &a, nil
}

// GetKosyncAccount returns the person's sync account.
func (s *Store) GetKosyncAccount(ctx context.Context, userID int64) (*KosyncAccount, error) {
	return scanKosyncAccount(s.DB.QueryRowContext(ctx, `SELECT `+kosyncAccountCols+` FROM kosync_accounts WHERE user_id = ?`, userID))
}

// KosyncAccountByUsername finds a sync account by its (case-insensitive) username.
func (s *Store) KosyncAccountByUsername(ctx context.Context, username string) (*KosyncAccount, error) {
	return scanKosyncAccount(s.DB.QueryRowContext(ctx, `SELECT `+kosyncAccountCols+` FROM kosync_accounts WHERE username = ?`, username))
}

// SaveKosyncAccount sets the person's sync username, and the key when keyHash
// is not empty. A username someone else uses is ErrConflict.
func (s *Store) SaveKosyncAccount(ctx context.Context, userID int64, username, keyHash string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kosync_accounts (user_id, username, key_hash, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET username = excluded.username,
			key_hash = CASE WHEN excluded.key_hash = '' THEN kosync_accounts.key_hash ELSE excluded.key_hash END`,
		userID, username, keyHash, Now())
	return conflict(err)
}

// DeleteKosyncAccount turns sync off for the person; their documents stay.
func (s *Store) DeleteKosyncAccount(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kosync_accounts WHERE user_id = ?`, userID)
	return err
}

// TouchKosyncAccount records that a device just synced.
func (s *Store) TouchKosyncAccount(ctx context.Context, userID int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE kosync_accounts SET last_seen = ? WHERE user_id = ?`, Now(), userID)
}

const kosyncDocCols = `d.document, d.progress, d.percentage, d.device, d.device_id, d.timestamp, d.title, d.authors, d.filename,
	d.book_id, d.ignored, COALESCE(b.title, ''), COALESCE(b.authors, '')`

func scanKosyncDocument(row interface{ Scan(...any) error }) (*KosyncDocument, error) {
	var d KosyncDocument
	var book sql.NullInt64
	var ignored int
	if err := row.Scan(&d.Document, &d.Progress, &d.Percentage, &d.Device, &d.DeviceID, &d.Timestamp, &d.Title, &d.Authors, &d.Filename,
		&book, &ignored, &d.BookTitle, &d.BookAuthors); err != nil {
		return nil, notFound(err)
	}
	if book.Valid {
		d.BookID = &book.Int64
	}
	d.Ignored = ignored == 1
	return &d, nil
}

// GetKosyncDocument returns one of the person's documents.
func (s *Store) GetKosyncDocument(ctx context.Context, userID int64, document string) (*KosyncDocument, error) {
	return scanKosyncDocument(s.DB.QueryRowContext(ctx, `SELECT `+kosyncDocCols+` FROM kosync_documents d
		LEFT JOIN books b ON b.id = d.book_id WHERE d.user_id = ? AND d.document = ?`, userID, document))
}

// ListKosyncDocuments returns the person's documents, most recently read first.
func (s *Store) ListKosyncDocuments(ctx context.Context, userID int64) ([]KosyncDocument, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+kosyncDocCols+` FROM kosync_documents d
		LEFT JOIN books b ON b.id = d.book_id WHERE d.user_id = ? ORDER BY d.timestamp DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KosyncDocument{}
	for rows.Next() {
		d, err := scanKosyncDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// SaveKosyncProgress stores a device's position for a document. Title,
// authors and filename the device leaves out keep their earlier values.
func (s *Store) SaveKosyncProgress(ctx context.Context, userID int64, d *KosyncDocument) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kosync_documents
		(user_id, document, progress, percentage, device, device_id, timestamp, title, authors, filename)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, document) DO UPDATE SET progress = excluded.progress, percentage = excluded.percentage,
			device = excluded.device, device_id = excluded.device_id, timestamp = excluded.timestamp,
			title = CASE WHEN excluded.title = '' THEN kosync_documents.title ELSE excluded.title END,
			authors = CASE WHEN excluded.authors = '' THEN kosync_documents.authors ELSE excluded.authors END,
			filename = CASE WHEN excluded.filename = '' THEN kosync_documents.filename ELSE excluded.filename END`,
		userID, d.Document, d.Progress, d.Percentage, d.Device, d.DeviceID, d.Timestamp, d.Title, d.Authors, d.Filename)
	return err
}

// LinkKosyncDocument ties a document to a book on the person's shelf (nil
// unlinks it) and sets whether it is ignored. A book that is not theirs is ErrNotFound.
func (s *Store) LinkKosyncDocument(ctx context.Context, userID int64, document string, bookID *int64, ignored bool) error {
	if bookID != nil {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM books WHERE id = ? AND user_id = ?`, *bookID, userID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE kosync_documents SET book_id = ?, ignored = ? WHERE user_id = ? AND document = ?`,
		bookID, boolInt(ignored), userID, document)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteKosyncDocument forgets a document; the device sends it again on its next sync.
func (s *Store) DeleteKosyncDocument(ctx context.Context, userID int64, document string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kosync_documents WHERE user_id = ? AND document = ?`, userID, document)
	return err
}
