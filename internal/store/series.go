package store

import (
	"context"
	"strings"
)

// SeriesSummary is one series on a user's shelf.
type SeriesSummary struct {
	Name    string `json:"name"`
	QID     string `json:"qid"`
	Authors string `json:"authors"`
	Books   int    `json:"books"`
	Read    int    `json:"read"`
	Reading int    `json:"reading"`
	Want    int    `json:"want"`
	Covers  []Book `json:"covers"`  // up to four, in series order (id, title, cover only)
	LastNo  string `json:"last_no"` // highest number read
}

// ListSeries returns the series on the user's shelf, most read first.
func (s *Store) ListSeries(ctx context.Context, userID int64) ([]SeriesSummary, error) {
	books, err := s.ListBooks(ctx, BookFilter{UserID: userID, Sort: "series"})
	if err != nil {
		return nil, err
	}
	var out []SeriesSummary
	idx := map[string]int{}
	readNos := map[string]map[string]bool{} // a book split in volumes counts once
	for _, b := range books {
		if strings.TrimSpace(b.Series) == "" {
			continue
		}
		k := strings.ToLower(b.Series)
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, SeriesSummary{Name: b.Series, Covers: []Book{}})
		}
		ss := &out[i]
		ss.Books++
		if ss.QID == "" {
			ss.QID = b.SeriesQID
		}
		if ss.Authors == "" {
			ss.Authors = strings.Split(b.Authors, ",")[0]
		}
		switch b.Status {
		case StatusRead:
			if readNos[k] == nil {
				readNos[k] = map[string]bool{}
			}
			if b.SeriesNo == "" || !readNos[k][b.SeriesNo] {
				ss.Read++
			}
			if b.SeriesNo != "" {
				readNos[k][b.SeriesNo] = true
				ss.LastNo = b.SeriesNo
			}
		case StatusReading:
			ss.Reading++
		case StatusWant:
			ss.Want++
		}
		if len(ss.Covers) < 4 {
			ss.Covers = append(ss.Covers, Book{ID: b.ID, Title: b.Title, Authors: b.Authors, Cover: b.Cover, SeriesNo: b.SeriesNo, Status: b.Status})
		}
	}
	// most read first, then by name
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Read > out[j-1].Read || out[j].Read == out[j-1].Read && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// RenameSeries renames a series on every book that carries it.
func (s *Store) RenameSeries(ctx context.Context, userID int64, from, to string) (int, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE books SET series = ?, updated_at = ? WHERE user_id = ? AND LOWER(series) = LOWER(?)`, to, Now(), userID, from)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetSeries records detected series data on a book (only when the book has none).
func (s *Store) SetSeries(ctx context.Context, bookID int64, name, no, qid string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE books SET series = ?, series_no = ?, series_qid = ?, series_checked = 1 WHERE id = ? AND series = ''`, name, no, qid, bookID)
	return err
}

// UncheckedSeriesBooks are books series detection has not looked at yet,
// grouped by the Open Library key of their first author.
func (s *Store) UncheckedSeriesBooks(ctx context.Context) (map[string][]Book, []int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.user_id, b.title, b.source, b.source_id, COALESCE(a.ol_key, '')
		FROM books b LEFT JOIN book_authors ba ON ba.book_id = b.id AND ba.position = 0 LEFT JOIN authors a ON a.id = ba.author_id
		WHERE b.series_checked = 0`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	byAuthor := map[string][]Book{}
	var noKey []int64
	for rows.Next() {
		var b Book
		var key string
		if err := rows.Scan(&b.ID, &b.UserID, &b.Title, &b.Source, &b.SourceID, &key); err != nil {
			return nil, nil, err
		}
		if key == "" {
			noKey = append(noKey, b.ID)
			continue
		}
		byAuthor[key] = append(byAuthor[key], b)
	}
	return byAuthor, noKey, rows.Err()
}

// MarkSeriesChecked records that detection looked at these books.
func (s *Store) MarkSeriesChecked(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := s.DB.ExecContext(ctx, `UPDATE books SET series_checked = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// ResetSeriesCheck queues all of a user's books without a series for detection.
func (s *Store) ResetSeriesCheck(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE books SET series_checked = 0 WHERE user_id = ? AND series = ''`, userID)
	return err
}
