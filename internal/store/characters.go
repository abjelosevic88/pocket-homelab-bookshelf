package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Character is one person in a book.
type Character struct {
	Name        string `json:"name"`
	Role        string `json:"role"` // main | supporting
	Description string `json:"description"`
}

// CharacterSet is a book's characters in one language.
type CharacterSet struct {
	Status     string      `json:"status"` // ok | none | unknown
	Characters []Character `json:"characters"`
	Source     string      `json:"source"`
	UpdatedAt  string      `json:"updated_at"`
}

// ValidCharacterStatus reports whether s is a known status.
func ValidCharacterStatus(s string) bool { return s == "ok" || s == "none" || s == "unknown" }

// Characters returns a book's character sets by language.
func (s *Store) Characters(ctx context.Context, bookID int64) (map[string]CharacterSet, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT lang, status, characters, source, updated_at FROM book_characters WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CharacterSet{}
	for rows.Next() {
		var lang, raw string
		var cs CharacterSet
		if err := rows.Scan(&lang, &cs.Status, &raw, &cs.Source, &cs.UpdatedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &cs.Characters) != nil || cs.Characters == nil {
			cs.Characters = []Character{}
		}
		out[lang] = cs
	}
	return out, rows.Err()
}

// PutCharacters saves (replaces) a book's characters in one language.
func (s *Store) PutCharacters(ctx context.Context, bookID int64, lang string, cs CharacterSet) error {
	if cs.Characters == nil {
		cs.Characters = []Character{}
	}
	raw, err := json.Marshal(cs.Characters)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO book_characters (book_id, lang, status, characters, source, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id, lang) DO UPDATE SET status = excluded.status, characters = excluded.characters, source = excluded.source, updated_at = excluded.updated_at`,
		bookID, lang, cs.Status, string(raw), cs.Source, time.Now().UTC().Format(time.RFC3339))
	return err
}

// DeleteCharacters forgets a book's characters (in all languages), so they are written again.
func (s *Store) DeleteCharacters(ctx context.Context, bookID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM book_characters WHERE book_id = ?`, bookID)
	return err
}

// MissingCharacters lists books that lack characters in any of langs, with
// the languages they lack; userID 0 means every user's books.
func (s *Store) MissingCharacters(ctx context.Context, userID int64, langs []string, limit int) ([]Book, [][]string, error) {
	books, err := s.ListBooks(ctx, BookFilter{UserID: userID, AllUsers: userID == 0})
	if err != nil {
		return nil, nil, err
	}
	have := map[int64]map[string]bool{}
	rows, err := s.DB.QueryContext(ctx, `SELECT book_id, lang FROM book_characters`)
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
	var outB []Book
	var outL [][]string
	for _, b := range books {
		var miss []string
		for _, l := range langs {
			if l = strings.TrimSpace(l); l != "" && !have[b.ID][l] {
				miss = append(miss, l)
			}
		}
		if len(miss) > 0 {
			outB = append(outB, b)
			outL = append(outL, miss)
			if limit > 0 && len(outB) >= limit {
				break
			}
		}
	}
	return outB, outL, nil
}

// GetBookAnyUser returns a book whoever owns it.
func (s *Store) GetBookAnyUser(ctx context.Context, id int64) (*Book, error) {
	return scanBook(s.DB.QueryRowContext(ctx, `SELECT `+bookCols+` FROM books b WHERE b.id = ?`, id))
}
