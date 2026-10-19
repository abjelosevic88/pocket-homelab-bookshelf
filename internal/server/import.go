package server

import (
	"context"
	"encoding/csv"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// handleImportGoodreads reads a Goodreads library export, a StoryGraph export
// or this app's own export.csv (detected by the header row) and adds the books
// that are not on the shelf yet. Covers are fetched afterwards in the background.
func (s *Server) handleImportGoodreads(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var src io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "malformed upload")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "form field 'file' is required")
			return
		}
		defer f.Close()
		src = f
	}
	cr := csv.NewReader(src)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "empty or invalid csv")
		return
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	if _, ok := col["title"]; !ok {
		writeErr(w, http.StatusBadRequest, "csv needs a Title column")
		return
	}
	// get returns the first non-empty value among the given column names.
	get := func(rec []string, names ...string) string {
		for _, n := range names {
			if i, ok := col[n]; ok && i < len(rec) {
				// Goodreads wraps ISBNs as ="0441478123" so spreadsheets keep the zeros.
				v := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(rec[i]), "=\""), "\""))
				if v != "" {
					return v
				}
			}
		}
		return ""
	}
	created, skipped := 0, 0
	var newIDs []int64
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "csv parse error: "+err.Error())
			return
		}
		b, reads := bookFromCSV(rec, get)
		b.UserID = me(r)
		if msg := clean(&b); msg != "" || s.findDuplicate(r, &b) != nil {
			skipped++
			continue
		}
		if err := s.store.ImportBook(r.Context(), &b, reads, nil); err != nil {
			s.fail(w, err, "import")
			return
		}
		created++
		newIDs = append(newIDs, b.ID)
	}
	s.fetchMissingCovers(me(r), newIDs)
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "skipped": skipped})
}

var exclusiveShelves = map[string]bool{"read": true, "currently-reading": true, "to-read": true, "did-not-finish": true}

func bookFromCSV(rec []string, get func([]string, ...string) string) (store.Book, []store.Read) {
	b := store.Book{Source: "import"}
	b.Title = get(rec, "title")
	b.Subtitle = get(rec, "subtitle")
	authors := get(rec, "authors", "author")
	if more := get(rec, "additional authors"); more != "" {
		authors += ", " + more
	}
	b.Authors = authors
	b.ISBN13 = get(rec, "isbn13", "isbn/uid")
	b.ISBN10 = get(rec, "isbn10", "isbn")
	b.Pages, _ = strconv.Atoi(get(rec, "pages", "number of pages"))
	b.Published = get(rec, "published", "original publication year", "year published")
	b.Publisher = get(rec, "publisher")
	b.Language = get(rec, "language")
	b.Review = metadata.PlainText(get(rec, "review", "my review"))
	b.PlanNote = get(rec, "plan_note", "private notes")
	b.PlannedFor = get(rec, "planned_for")
	b.Favorite = strings.EqualFold(get(rec, "favorite"), "yes")
	if v, err := strconv.ParseFloat(strings.ReplaceAll(get(rec, "rating", "my rating", "star rating"), ",", "."), 64); err == nil && v > 0 {
		b.Rating = math.Max(0.5, math.Min(5, math.Round(v*2)/2))
	}
	b.Format = formatFrom(get(rec, "format", "binding"))
	// Tags: our own column, StoryGraph's, or Goodreads shelves minus the status shelf.
	if t := get(rec, "tags"); t != "" {
		b.Tags = t
	} else {
		var tags []string
		for _, sh := range strings.Split(get(rec, "bookshelves"), ",") {
			if sh = strings.TrimSpace(sh); sh != "" && !exclusiveShelves[sh] {
				tags = append(tags, sh)
			}
		}
		b.Tags = strings.Join(tags, ", ")
	}
	switch strings.ToLower(get(rec, "status", "exclusive shelf", "read status")) {
	case "read":
		b.Status = store.StatusRead
	case "currently-reading", "reading":
		b.Status = store.StatusReading
	case "did-not-finish", "abandoned", "dnf":
		b.Status = store.StatusAbandoned
	default:
		b.Status = store.StatusWant
	}
	started := csvDate(get(rec, "started_on"))
	finished := csvDate(get(rec, "finished_on", "date read", "last date read"))
	var reads []store.Read
	switch b.Status {
	case store.StatusReading:
		reads = append(reads, store.Read{StartedOn: started})
	case store.StatusRead, store.StatusAbandoned:
		if finished != "" {
			reads = append(reads, store.Read{StartedOn: started, FinishedOn: finished, Abandoned: b.Status == store.StatusAbandoned})
		}
	}
	if added := csvDate(get(rec, "added", "date added")); added != "" {
		b.CreatedAt = added + "T12:00:00Z"
	}
	return b, reads
}

// csvDate accepts 2024/05/14, 2024-05-14 and 2024-05-14T… and returns YYYY-MM-DD.
func csvDate(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "/", "-")
	if len(s) >= 10 && validDate(s[:10]) {
		return s[:10]
	}
	return ""
}

func formatFrom(s string) string {
	s = strings.ToLower(s)
	switch {
	case s == "":
		return ""
	case strings.Contains(s, "audio"):
		return "audio"
	case strings.Contains(s, "kindle") || strings.Contains(s, "ebook") || strings.Contains(s, "digital") || strings.Contains(s, "epub"):
		return "ebook"
	case strings.Contains(s, "paper") || strings.Contains(s, "hardcover") || strings.Contains(s, "mass market") || strings.Contains(s, "print"):
		return "paper"
	}
	return ""
}

// ---- background cover fetching ----

type coverJob struct {
	mu      sync.Mutex
	running bool
	user    int64 // whose covers the (last) run fetched
	done    int
	found   int
	total   int
}

// fetchMissingCovers looks up covers by ISBN on Open Library for the given
// books (all cover-less books of the user when ids is nil), one request every
// few seconds to stay inside Open Library's rate limit for ISBN lookups.
func (s *Server) fetchMissingCovers(userID int64, ids []int64) bool {
	if ids != nil && len(ids) == 0 {
		return false
	}
	s.covers.mu.Lock()
	if s.covers.running {
		s.covers.mu.Unlock()
		return false
	}
	s.covers.running, s.covers.user, s.covers.done, s.covers.found, s.covers.total = true, userID, 0, 0, 0
	s.covers.mu.Unlock()
	go func() {
		defer func() { s.covers.mu.Lock(); s.covers.running = false; s.covers.mu.Unlock() }()
		ctx := context.Background()
		books, err := s.store.ListBooks(ctx, store.BookFilter{UserID: userID, Sort: "added"})
		if err != nil {
			return
		}
		want := map[int64]bool{}
		for _, id := range ids {
			want[id] = true
		}
		var todo []store.Book
		for _, b := range books {
			if b.Cover == "" && b.ISBN13 != "" && (ids == nil || want[b.ID]) {
				todo = append(todo, b)
			}
		}
		s.covers.mu.Lock()
		s.covers.total = len(todo)
		s.covers.mu.Unlock()
		for i, b := range todo {
			if i > 0 {
				time.Sleep(3 * time.Second)
			}
			url := "https://covers.openlibrary.org/b/isbn/" + b.ISBN13 + "-L.jpg?default=false"
			data, ct, err := s.meta.FetchCover(ctx, url, s.cfg.MaxCoverBytes)
			ok := err == nil && s.writeCover(ctx, &b, data, ct) == nil
			s.covers.mu.Lock()
			s.covers.done++
			if ok {
				s.covers.found++
			}
			s.covers.mu.Unlock()
		}
		s.covers.mu.Lock()
		found := s.covers.found
		s.covers.mu.Unlock()
		s.log.Info("cover lookup finished", "user", userID, "books", len(todo), "found", found)
	}()
	return true
}

func (s *Server) handleFetchCovers(w http.ResponseWriter, r *http.Request) {
	s.fetchMissingCovers(me(r), nil)
	s.handleCoverStatus(w, r)
}

// handleCoverStatus reports the caller's own cover run; while someone else's
// runs, it only says the job is busy.
func (s *Server) handleCoverStatus(w http.ResponseWriter, r *http.Request) {
	s.covers.mu.Lock()
	defer s.covers.mu.Unlock()
	if s.covers.user != me(r) {
		writeJSON(w, http.StatusOK, map[string]any{"running": false, "busy": s.covers.running, "done": 0, "found": 0, "total": 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": s.covers.running, "done": s.covers.done, "found": s.covers.found, "total": s.covers.total})
}
