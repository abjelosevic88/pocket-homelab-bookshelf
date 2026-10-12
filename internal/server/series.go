package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Series are detected through Wikidata: an author's works that belong to a
// series carry the series and their number in it, and usually their Open
// Library work id, which is how they are matched to books on the shelf.

// bookKeys are the forms of a shelf title to look up: whole and without subtitle.
func bookKeys(s string) []string {
	out := []string{fold.String(s)}
	if st := shortTitle(s); st != out[0] {
		out = append(out, st)
	}
	return out
}

// detectSeries looks at books detection has not seen yet.
func (s *Server) detectSeries(ctx context.Context) {
	byAuthor, noKey, err := s.store.UncheckedSeriesBooks(ctx)
	if err != nil {
		return
	}
	_ = s.store.MarkSeriesChecked(ctx, noKey)
	for olKey, books := range byAuthor {
		if ctx.Err() != nil {
			return
		}
		ids := make([]int64, len(books))
		for i, b := range books {
			ids[i] = b.ID
		}
		qid, err := s.meta.AuthorWikidataID(ctx, olKey)
		if err != nil {
			continue // catalogue unreachable: try again on the next pass
		}
		var works []metadata.WDWork
		if qid != "" {
			if works, err = s.meta.SeriesWorksByAuthor(ctx, qid); err != nil {
				continue
			}
		}
		if len(works) > 0 {
			s.assignSeries(ctx, books, works)
		}
		_ = s.store.MarkSeriesChecked(ctx, ids)
	}
}

// assignSeries matches shelf books to an author's series works: by Open
// Library work id, else by the whole title in any language (Cyrillic read as
// Latin). A work in several series goes to the one most of the matched books share.
func (s *Server) assignSeries(ctx context.Context, books []store.Book, works []metadata.WDWork) {
	byOL := map[string]*metadata.WDWork{}
	byTitle := map[string]*metadata.WDWork{}
	var seriesIDs []string
	for i := range works {
		w := &works[i]
		for _, k := range w.OLWorks {
			byOL[k] = w
		}
		for _, l := range w.Labels {
			if k := fold.String(metadata.Latin(l)); k != "" {
				if _, taken := byTitle[k]; !taken {
					byTitle[k] = w
				}
			}
		}
		for _, r := range w.Series {
			seriesIDs = append(seriesIDs, r.QID)
		}
	}
	info, _ := s.meta.SeriesNames(ctx, uniq(seriesIDs))
	matched := map[int64]*metadata.WDWork{}
	for _, b := range books {
		var w *metadata.WDWork
		if b.Source == "openlibrary" {
			w = byOL[strings.TrimPrefix(b.SourceID, "/works/")]
		}
		for _, k := range bookKeys(b.Title) {
			if w == nil {
				w = byTitle[k]
			}
		}
		if w == nil && b.Source == "openlibrary" {
			if t := s.meta.OpenLibraryWorkTitle(ctx, b.SourceID); t != "" {
				for _, k := range bookKeys(metadata.Latin(t)) {
					if w == nil {
						w = byTitle[k]
					}
				}
			}
		}
		if w != nil {
			matched[b.ID] = w
		}
	}
	shared := map[string]int{}
	for _, w := range matched {
		for _, r := range w.Series {
			if info[r.QID].Valid {
				shared[r.QID]++
			}
		}
	}
	for id, w := range matched {
		var best *metadata.WDSeriesRef
		for i := range w.Series {
			r := &w.Series[i]
			if !info[r.QID].Valid {
				continue
			}
			if best == nil || shared[r.QID] > shared[best.QID] || shared[r.QID] == shared[best.QID] && best.Ordinal == "" && r.Ordinal != "" {
				best = r
			}
		}
		if best != nil {
			_ = s.store.SetSeries(ctx, id, info[best.QID].Name, best.Ordinal, best.QID)
		}
	}
}

func uniq(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// seriesTotal is how many books a series has: its distinct main numbers
// (1, 2, 3 …), or all its entries when Wikidata numbers only a few.
func seriesTotal(members []metadata.WDWork) int {
	nums := map[int]bool{}
	for _, m := range members {
		if f, ok := metadata.OrdinalValue(m.Series[0].Ordinal); ok && f >= 1 && f == float64(int(f)) {
			nums[int(f)] = true
		}
	}
	if len(nums)*2 < len(members) {
		return len(members)
	}
	return len(nums)
}

func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSeries(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "series")
		return
	}
	type item struct {
		store.SeriesSummary
		Total int `json:"total"`
	}
	out := make([]item, 0, len(list))
	for _, ss := range list {
		it := item{SeriesSummary: ss}
		if ss.QID != "" {
			if m, err := s.meta.SeriesMembers(r.Context(), ss.QID); err == nil {
				it.Total = max(seriesTotal(m), ss.Read)
			}
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

type seriesEntry struct {
	No       string      `json:"no"`
	Title    string      `json:"title"`
	Year     string      `json:"year,omitempty"`
	OLWork   string      `json:"ol_work,omitempty"`
	CoverURL string      `json:"cover_url,omitempty"`
	ThumbURL string      `json:"thumb_url,omitempty"`
	Book     *store.Book `json:"book,omitempty"` // on your shelf
}

// addCovers looks up covers (and Open Library works) for entries you do not
// own, a few at a time.
func (s *Server) addCovers(ctx context.Context, entries []seriesEntry, author string) {
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i := range entries {
		e := &entries[i]
		if e.Book != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			key, cover := s.meta.WorkCover(ctx, e.OLWork, e.Title, author)
			if e.OLWork == "" {
				e.OLWork = key
			}
			if cover > 0 {
				e.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", cover)
				e.ThumbURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-M.jpg", cover)
			}
		}()
	}
	wg.Wait()
}

// handleSeriesDetail lists a series in order: your books, and the others
// from Wikidata that you do not have yet.
func (s *Server) handleSeriesDetail(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	books, err := s.store.ListBooks(r.Context(), store.BookFilter{UserID: me(r), Series: name, Sort: "series"})
	if err != nil {
		s.fail(w, err, "series")
		return
	}
	qid := ""
	for _, b := range books {
		if b.SeriesQID != "" {
			qid = b.SeriesQID
			break
		}
	}
	var entries []seriesEntry
	used := map[int64]bool{}
	total := 0
	if qid != "" {
		if members, err := s.meta.SeriesMembers(r.Context(), qid); err == nil {
			total = seriesTotal(members)
			for _, m := range members {
				e := seriesEntry{No: m.Series[0].Ordinal, Title: m.Title, Year: m.Year}
				if len(m.OLWorks) > 0 {
					e.OLWork = "/works/" + m.OLWorks[0]
				}
				for i := range books {
					b := &books[i]
					if used[b.ID] {
						continue
					}
					sameNo := b.SeriesNo != "" && b.SeriesNo == e.No
					sameWork := e.OLWork != "" && b.SourceID == e.OLWork
					if sameNo || sameWork {
						e.Book, used[b.ID] = b, true
						break
					}
				}
				entries = append(entries, e)
			}
		}
	}
	for i := range books { // books Wikidata does not know (or no Wikidata link)
		if !used[books[i].ID] {
			entries = append(entries, seriesEntry{No: books[i].SeriesNo, Title: books[i].Title, Book: &books[i]})
		}
	}
	read := 0
	author := ""
	for _, b := range books {
		if b.Status == store.StatusRead {
			read++
		}
		if author == "" {
			author = strings.Split(b.Authors, ",")[0]
		}
	}
	s.addCovers(r.Context(), entries, author)
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "author": author, "qid": qid, "total": total, "read": read, "books": len(books), "entries": entries})
}

// handleSeriesNext suggests what to read next in each series you are in:
// the first numbered book after the highest one you have read.
func (s *Server) handleSeriesNext(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSeries(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "series")
		return
	}
	type next struct {
		Series   string      `json:"series"`
		Author   string      `json:"author"`
		No       string      `json:"no"`
		Title    string      `json:"title"`
		Year     string      `json:"year,omitempty"`
		OLWork   string      `json:"ol_work,omitempty"`
		CoverURL string      `json:"cover_url,omitempty"`
		ThumbURL string      `json:"thumb_url,omitempty"`
		Book     *store.Book `json:"book,omitempty"` // already on your shelf
		Read     int         `json:"read"`
		Total    int         `json:"total"`
	}
	out := []next{}
	for _, ss := range list {
		if ss.Read == 0 || ss.QID == "" {
			continue
		}
		books, _ := s.store.ListBooks(r.Context(), store.BookFilter{UserID: me(r), Series: ss.Name, Sort: "series"})
		maxRead := -1.0
		reading := false
		for _, b := range books {
			if b.Status == store.StatusReading {
				reading = true
			}
			if f, ok := metadata.OrdinalValue(b.SeriesNo); ok && b.Status == store.StatusRead && f > maxRead {
				maxRead = f
			}
		}
		if reading { // you are in the middle of one already
			continue
		}
		members, err := s.meta.SeriesMembers(r.Context(), ss.QID)
		if err != nil {
			continue
		}
		for _, m := range members {
			f, ok := metadata.OrdinalValue(m.Series[0].Ordinal)
			if !ok || f <= maxRead || f != float64(int(f)) || f < 1 {
				continue
			}
			n := next{Series: ss.Name, No: m.Series[0].Ordinal, Title: m.Title, Year: m.Year, Read: ss.Read, Total: seriesTotal(members)}
			if len(m.OLWorks) > 0 {
				n.OLWork = "/works/" + m.OLWorks[0]
			}
			for i := range books {
				if books[i].SeriesNo == n.No || (n.OLWork != "" && books[i].SourceID == n.OLWork) {
					n.Book = &books[i]
				}
			}
			if n.Book == nil || n.Book.Status == store.StatusWant {
				n.Author = ss.Authors
				if n.Book == nil {
					key, cover := s.meta.WorkCover(r.Context(), n.OLWork, n.Title, ss.Authors)
					if n.OLWork == "" {
						n.OLWork = key
					}
					if cover > 0 {
						n.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", cover)
						n.ThumbURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-M.jpg", cover)
					}
				}
				out = append(out, n)
			}
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRenameSeries(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.From) == "" {
		writeErr(w, http.StatusBadRequest, "send {from, to}")
		return
	}
	n, err := s.store.RenameSeries(r.Context(), me(r), in.From, strings.TrimSpace(in.To))
	if err != nil {
		s.fail(w, err, "rename series")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"books": n})
}

// handleDetectSeries looks all books without a series up again.
func (s *Server) handleDetectSeries(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ResetSeriesCheck(r.Context(), me(r)); err != nil {
		s.fail(w, err, "series")
		return
	}
	s.kickAuthors()
	w.WriteHeader(http.StatusAccepted)
}
