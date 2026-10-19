package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

func me(r *http.Request) int64 { return actorID(r.Context()) }

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.BookFilter{
		UserID:   me(r),
		Status:   q.Get("status"),
		Query:    q.Get("q"),
		Tag:      q.Get("tag"),
		Author:   q.Get("author"),
		Year:     int(queryInt(r, "year")),
		Favorite: queryBool(r, "favorite"),
		ISBN:     metadata.NormalizeISBN(q.Get("isbn")),
		Sort:     q.Get("sort"),
		Limit:    int(queryInt(r, "limit")),
	}
	if f.Status != "" && !store.ValidStatus(f.Status) {
		writeErr(w, http.StatusBadRequest, "status must be want, reading, read or abandoned")
		return
	}
	books, err := s.store.ListBooks(r.Context(), f)
	if err != nil {
		s.fail(w, err, "list books")
		return
	}
	writeJSON(w, http.StatusOK, books)
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	reads, err := s.store.ListReads(r.Context(), b.UserID, b.ID)
	if err != nil {
		s.fail(w, err, "list reads")
		return
	}
	quotes, err := s.store.ListQuotes(r.Context(), store.QuoteFilter{UserID: b.UserID, BookID: b.ID})
	if err != nil {
		s.fail(w, err, "list quotes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"book": b, "reads": reads, "quotes": quotes})
}

// bookExtras are request fields that are not columns of a book.
type bookExtras struct {
	CoverURL       string `json:"cover_url"`   // download this image as the cover
	StartedOn      string `json:"started_on"`  // create: start of the read
	FinishedOn     string `json:"finished_on"` // create: end of the read
	AllowDuplicate bool   `json:"allow_duplicate"`
}

// clean trims and validates a book; it returns a message for the user, or "".
func clean(b *store.Book) string {
	b.Title = strings.TrimSpace(b.Title)
	b.Subtitle = strings.TrimSpace(b.Subtitle)
	b.Authors = strings.Join(splitList(b.Authors), ", ")
	b.ISBN13 = metadata.NormalizeISBN(b.ISBN13)
	b.ISBN10 = metadata.NormalizeISBN(b.ISBN10)
	if len(b.ISBN13) == 10 { // someone typed an ISBN-10 into the ISBN-13 field
		b.ISBN10, b.ISBN13 = b.ISBN13, metadata.ISBN10to13(b.ISBN13)
	}
	if b.ISBN13 == "" && len(b.ISBN10) == 10 {
		b.ISBN13 = metadata.ISBN10to13(b.ISBN10)
	}
	b.Format = strings.ToLower(strings.TrimSpace(b.Format))
	b.PlannedFor = strings.TrimSpace(b.PlannedFor)
	switch {
	case b.Title == "":
		return "title is required"
	case b.Pages < 0 || b.CurrentPage < 0:
		return "pages cannot be negative"
	case b.Rating < 0 || b.Rating > 5:
		return "rating must be between 0.5 and 5 (0 = not rated)"
	case b.Format != "" && b.Format != "paper" && b.Format != "ebook" && b.Format != "audio":
		return "format must be paper, ebook or audio"
	case b.PlannedFor != "" && !validMonth(b.PlannedFor):
		return "planned_for must be YYYY-MM"
	}
	b.Rating = math.Round(b.Rating*2) / 2 // half stars
	if b.Pages > 0 && b.CurrentPage > b.Pages {
		b.CurrentPage = b.Pages
	}
	return ""
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// findDuplicate returns a book already on the shelf with the same ISBN or the
// same title and authors.
func (s *Server) findDuplicate(r *http.Request, b *store.Book) *store.Book {
	if b.ISBN13 != "" {
		if found, _ := s.store.ListBooks(r.Context(), store.BookFilter{UserID: b.UserID, ISBN: b.ISBN13, Limit: 1}); len(found) > 0 {
			return &found[0]
		}
	}
	found, _ := s.store.ListBooks(r.Context(), store.BookFilter{UserID: b.UserID, Query: b.Title})
	for i := range found {
		if strings.EqualFold(found[i].Title, b.Title) && strings.EqualFold(found[i].Authors, b.Authors) {
			return &found[i]
		}
	}
	return nil
}

func (s *Server) handleCreateBook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "request too large")
		return
	}
	var b store.Book
	var ex bookExtras
	if json.Unmarshal(body, &b) != nil || json.Unmarshal(body, &ex) != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b.ID, b.UserID, b.Cover = 0, me(r), ""
	if b.Status == "" {
		b.Status = store.StatusWant
	}
	if !store.ValidStatus(b.Status) {
		writeErr(w, http.StatusBadRequest, "status must be want, reading, read or abandoned")
		return
	}
	if msg := clean(&b); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	for _, d := range []string{ex.StartedOn, ex.FinishedOn} {
		if d != "" && !validDate(d) {
			writeErr(w, http.StatusBadRequest, "dates must be YYYY-MM-DD")
			return
		}
	}
	if b.Source == "" {
		b.Source = "manual"
	}
	if !ex.AllowDuplicate {
		if dup := s.findDuplicate(r, &b); dup != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "this book is already on your shelf", "book_id": dup.ID})
			return
		}
	}
	if (b.Status == store.StatusRead || b.Status == store.StatusAbandoned) && ex.FinishedOn == "" && ex.StartedOn == "" {
		ex.FinishedOn = store.Today()
	}
	if err := s.store.CreateBook(r.Context(), &b, ex.StartedOn, ex.FinishedOn); err != nil {
		s.fail(w, err, "create book")
		return
	}
	resp := map[string]any{}
	if ex.CoverURL != "" {
		if err := s.saveCoverFromURL(r, &b, ex.CoverURL); err != nil {
			s.log.Warn("cover download failed", "book", b.ID, "url", ex.CoverURL, "err", err)
			resp["warning"] = "Saved, but the cover could not be downloaded: " + err.Error()
		}
	}
	saved, err := s.store.GetBook(r.Context(), b.UserID, b.ID)
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	resp["book"] = saved
	writeJSON(w, http.StatusCreated, resp)
}

// handleUpdateBook merges the fields present in the body onto the book, so a
// client can send only {"rating": 4.5} or {"review": "…"}.
func (s *Server) handleUpdateBook(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "request too large")
		return
	}
	b := *cur
	var ex bookExtras
	if json.Unmarshal(body, &b) != nil || json.Unmarshal(body, &ex) != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Not editable here: identity, cover (own endpoint), status (own endpoint), queue order.
	newStatus := b.Status
	b.ID, b.UserID, b.Cover, b.Status, b.QueuePos, b.CreatedAt = cur.ID, cur.UserID, cur.Cover, cur.Status, cur.QueuePos, cur.CreatedAt
	if msg := clean(&b); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateBook(r.Context(), &b); err != nil {
		s.fail(w, err, "update book")
		return
	}
	if newStatus != cur.Status && store.ValidStatus(newStatus) {
		nb, err := s.store.SetStatus(r.Context(), b.UserID, b.ID, newStatus, "")
		if err != nil {
			s.fail(w, err, "set status")
			return
		}
		s.publishStatus(nb, "")
	}
	resp := map[string]any{}
	if ex.CoverURL != "" {
		if err := s.saveCoverFromURL(r, &b, ex.CoverURL); err != nil {
			resp["warning"] = "Saved, but the cover could not be downloaded: " + err.Error()
		}
	}
	saved, err := s.store.GetBook(r.Context(), b.UserID, b.ID)
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	if saved.Rating != cur.Rating {
		s.publishRated(saved)
	}
	resp["book"] = saved
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	if err := s.store.DeleteBook(r.Context(), b.UserID, b.ID); err != nil {
		s.fail(w, err, "delete book")
		return
	}
	s.removeCoverFile(b.Cover)
	w.WriteHeader(http.StatusNoContent)
}

// handleSetStatus moves a book along: want → reading → read / abandoned.
// Finishing can carry the verdict in the same call.
func (s *Server) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string   `json:"status"`
		Date   string   `json:"date"`
		Rating *float64 `json:"rating"`
		Review *string  `json:"review"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !store.ValidStatus(in.Status) {
		writeErr(w, http.StatusBadRequest, "status must be want, reading, read or abandoned")
		return
	}
	if in.Date != "" && !validDate(in.Date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	b, err := s.store.SetStatus(r.Context(), me(r), idParam(r, "id"), in.Status, in.Date)
	if err != nil {
		s.fail(w, err, "set status")
		return
	}
	s.publishStatus(b, in.Date)
	if in.Rating != nil || in.Review != nil {
		if in.Rating != nil {
			b.Rating = *in.Rating
		}
		if in.Review != nil {
			b.Review = *in.Review
		}
		if msg := clean(b); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		if err := s.store.UpdateBook(r.Context(), b); err != nil {
			s.fail(w, err, "update book")
			return
		}
		b, _ = s.store.GetBook(r.Context(), b.UserID, b.ID)
		if in.Rating != nil && b != nil {
			s.publishRated(b)
		}
	}
	writeJSON(w, http.StatusOK, b)
}

// publishStatus tells Pocket Core a book was started, finished or abandoned
// (a move back to "want" is not an event).
func (s *Server) publishStatus(b *store.Book, date string) {
	if b == nil {
		return
	}
	var topic string
	switch b.Status {
	case store.StatusReading:
		topic = "bookshelf.started"
		if date == "" {
			date = b.StartedOn
		}
	case store.StatusRead:
		topic = "bookshelf.finished"
		if date == "" {
			date = b.FinishedOn
		}
	case store.StatusAbandoned:
		topic = "bookshelf.abandoned"
	default:
		return
	}
	s.publish(b.UserID, topic, fmt.Sprintf("book:%d", b.ID), map[string]any{
		"book_id": b.ID, "title": b.Title, "authors": b.Authors, "status": b.Status, "date": date,
		"pages": b.Pages, "rating": b.Rating, "read_count": b.ReadCount, "series": b.Series,
	})
}

// publishRated tells Pocket Core a book got a (new) rating.
func (s *Server) publishRated(b *store.Book) {
	s.publish(b.UserID, "bookshelf.rated", fmt.Sprintf("book:%d", b.ID), map[string]any{
		"book_id": b.ID, "title": b.Title, "authors": b.Authors, "rating": b.Rating, "status": b.Status, "favorite": b.Favorite,
	})
}

// handleProgress records the current page ({"page": 120}) or percent
// ({"percent": 40}, needs the page count). Reaching the last page does not
// finish the book; that stays a deliberate step.
func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Page    *int `json:"page"`
		Percent *int `json:"percent"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	switch {
	case in.Page != nil:
		b.CurrentPage = *in.Page
	case in.Percent != nil && b.Pages > 0:
		b.CurrentPage = int(math.Round(float64(*in.Percent) / 100 * float64(b.Pages)))
	default:
		writeErr(w, http.StatusBadRequest, "send page, or percent for a book with a page count")
		return
	}
	if msg := clean(b); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateBook(r.Context(), b); err != nil {
		s.fail(w, err, "update book")
		return
	}
	if b.Status == store.StatusWant {
		b, _ = s.store.SetStatus(r.Context(), b.UserID, b.ID, store.StatusReading, "")
		s.publishStatus(b, "")
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleReorderPlanner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderQueue(r.Context(), me(r), in.IDs); err != nil {
		s.fail(w, err, "reorder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "list tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	st, _ := s.store.GetSettings(r.Context(), me(r))
	d, err := s.store.GetDashboard(r.Context(), me(r), st.YearlyGoal)
	if err != nil {
		s.fail(w, err, "dashboard")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, _ := s.store.GetSettings(r.Context(), me(r))
	year := int(queryInt(r, "year"))
	goal := 0
	if year == 0 || year == time.Now().Year() {
		goal = st.YearlyGoal
	}
	out, err := s.store.GetStats(r.Context(), me(r), year, goal)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- reads ----

func (s *Server) readInput(w http.ResponseWriter, r *http.Request) (*store.Read, bool) {
	var rd store.Read
	if err := decode(r, &rd); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return nil, false
	}
	if (rd.StartedOn != "" && !validDate(rd.StartedOn)) || (rd.FinishedOn != "" && !validDate(rd.FinishedOn)) {
		writeErr(w, http.StatusBadRequest, "dates must be YYYY-MM-DD")
		return nil, false
	}
	if rd.StartedOn != "" && rd.FinishedOn != "" && rd.FinishedOn < rd.StartedOn {
		writeErr(w, http.StatusBadRequest, "finished before it started")
		return nil, false
	}
	return &rd, true
}

func (s *Server) handleAddRead(w http.ResponseWriter, r *http.Request) {
	rd, ok := s.readInput(w, r)
	if !ok {
		return
	}
	if rd.FinishedOn == "" {
		writeErr(w, http.StatusBadRequest, "a past read needs a finish date (use status \"reading\" to start one)")
		return
	}
	rd.BookID = idParam(r, "id")
	if err := s.store.AddRead(r.Context(), me(r), rd); err != nil {
		s.fail(w, err, "add read")
		return
	}
	writeJSON(w, http.StatusCreated, rd)
}

func (s *Server) handleUpdateRead(w http.ResponseWriter, r *http.Request) {
	rd, ok := s.readInput(w, r)
	if !ok {
		return
	}
	rd.ID = idParam(r, "id")
	if err := s.store.UpdateRead(r.Context(), me(r), rd); err != nil {
		s.fail(w, err, "update read")
		return
	}
	writeJSON(w, http.StatusOK, rd)
}

func (s *Server) handleDeleteRead(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteRead(r.Context(), me(r), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete read")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- quotes ----

func (s *Server) handleListQuotes(w http.ResponseWriter, r *http.Request) {
	hidden := 0
	switch r.URL.Query().Get("hidden") {
	case "1":
		hidden = 1
	case "0":
		hidden = -1
	}
	q, err := s.store.ListQuotes(r.Context(), store.QuoteFilter{UserID: me(r), BookID: queryInt(r, "book"), Query: r.URL.Query().Get("q"),
		Random: queryBool(r, "random"), Favorite: queryBool(r, "favorite"), Hidden: hidden, Limit: int(queryInt(r, "limit"))})
	if err != nil {
		s.fail(w, err, "list quotes")
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func quoteInput(w http.ResponseWriter, r *http.Request) (*store.Quote, bool) {
	var q store.Quote
	if err := decode(r, &q); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return nil, false
	}
	q.Text, q.Page, q.Note = strings.TrimSpace(q.Text), strings.TrimSpace(q.Page), strings.TrimSpace(q.Note)
	if q.Text == "" {
		writeErr(w, http.StatusBadRequest, "quote text is required")
		return nil, false
	}
	return &q, true
}

func (s *Server) handleCreateQuote(w http.ResponseWriter, r *http.Request) {
	q, ok := quoteInput(w, r)
	if !ok {
		return
	}
	q.BookID = idParam(r, "id")
	if err := s.store.CreateQuote(r.Context(), me(r), q); err != nil {
		s.fail(w, err, "create quote")
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (s *Server) handleUpdateQuote(w http.ResponseWriter, r *http.Request) {
	q, ok := quoteInput(w, r)
	if !ok {
		return
	}
	q.ID = idParam(r, "id")
	if err := s.store.UpdateQuote(r.Context(), me(r), q); err != nil {
		s.fail(w, err, "update quote")
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) handleDeleteQuote(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteQuote(r.Context(), me(r), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete quote")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- metadata search ----

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	source := q.Get("source")
	if source == "" {
		st, _ := s.store.GetSettings(r.Context(), me(r))
		source = st.SearchSource
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	meta := s.metaFor(r.Context(), me(r))
	results, warnings, err := meta.Search(r.Context(), q.Get("q"), source, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "warnings": warnings})
		return
	}
	// Mark results that are already on the shelf.
	mine, _ := s.store.ListBooks(r.Context(), store.BookFilter{UserID: me(r)})
	byISBN, byTitle := map[string]int64{}, map[string]int64{}
	for _, b := range mine {
		if b.ISBN13 != "" {
			byISBN[b.ISBN13] = b.ID
		}
		byTitle[strings.ToLower(b.Title+"\x00"+b.Authors)] = b.ID
	}
	type result struct {
		metadata.Candidate
		OnShelf int64 `json:"on_shelf,omitempty"`
	}
	out := make([]result, 0, len(results))
	for _, c := range results {
		res := result{Candidate: c}
		if id, ok := byISBN[c.ISBN13]; ok && c.ISBN13 != "" {
			res.OnShelf = id
		} else if id, ok := byTitle[strings.ToLower(c.Title+"\x00"+strings.Join(c.Authors, ", "))]; ok {
			res.OnShelf = id
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out, "warnings": warnings, "sources": meta.Sources()})
}

func (s *Server) handleSearchDetails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := s.metaFor(r.Context(), me(r)).Details(r.Context(), q.Get("source"), q.Get("id"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ---- covers ----

func (s *Server) writeCover(ctx context.Context, b *store.Book, data []byte, contentType string) error {
	name := strconv.FormatInt(b.ID, 10) + "-" + store.RandomToken(4) + metadata.CoverExt[contentType]
	if err := os.WriteFile(filepath.Join(s.cfg.CoverDir, name), data, 0o644); err != nil {
		return err
	}
	if err := s.store.SetCover(ctx, b.UserID, b.ID, name); err != nil {
		_ = os.Remove(filepath.Join(s.cfg.CoverDir, name))
		return err
	}
	s.removeCoverFile(b.Cover)
	b.Cover = name
	return nil
}

func (s *Server) saveCoverFromURL(r *http.Request, b *store.Book, url string) error {
	data, ct, err := s.meta.FetchCover(r.Context(), url, s.cfg.MaxCoverBytes)
	if err != nil {
		return err
	}
	return s.writeCover(r.Context(), b, data, ct)
}

func (s *Server) removeCoverFile(name string) {
	if name != "" {
		_ = os.Remove(filepath.Join(s.cfg.CoverDir, filepath.Base(name)))
	}
}

// handleSetCover accepts a multipart upload (field "cover") or {"url": "…"}.
func (s *Server) handleSetCover(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		var in struct {
			URL string `json:"url"`
		}
		if err := decode(r, &in); err != nil || in.URL == "" {
			writeErr(w, http.StatusBadRequest, "send a multipart 'cover' file or {\"url\": \"…\"}")
			return
		}
		if err := s.saveCoverFromURL(r, b, in.URL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, b)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxCoverBytes+1<<20)
	if err := r.ParseMultipartForm(s.cfg.MaxCoverBytes); err != nil {
		writeErr(w, http.StatusBadRequest, "cover too large or malformed upload")
		return
	}
	f, _, err := r.FormFile("cover")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "form field 'cover' is required")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxCoverBytes+1))
	if err != nil || int64(len(data)) > s.cfg.MaxCoverBytes {
		writeErr(w, http.StatusBadRequest, "cover too large")
		return
	}
	ct := http.DetectContentType(data)
	if _, ok := metadata.CoverExt[ct]; !ok {
		writeErr(w, http.StatusBadRequest, "cover must be a JPEG, PNG, WebP or GIF image")
		return
	}
	if err := s.writeCover(r.Context(), b, data, ct); err != nil {
		s.fail(w, err, "save cover")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// handleCoverSuggestions lists covers to pick from: the other editions of the
// book on Open Library and, with a key, Google Books volumes.
func (s *Server) handleCoverSuggestions(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	work := ""
	if b.Source == "openlibrary" {
		work = b.SourceID
	}
	covers := s.metaFor(r.Context(), me(r)).CoverSuggestions(r.Context(), work, b.Title, b.Authors)
	if covers == nil {
		covers = []metadata.CoverOption{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"covers": covers})
}

func (s *Server) handleGetCover(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil || b.Cover == "" {
		writeErr(w, http.StatusNotFound, "no cover")
		return
	}
	// The file name changes with every new cover, so it can be cached for good.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeFile(w, r, filepath.Join(s.cfg.CoverDir, filepath.Base(b.Cover)))
}

func (s *Server) handleDeleteCover(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	s.removeCoverFile(b.Cover)
	_ = s.store.SetCover(r.Context(), b.UserID, b.ID, "")
	w.WriteHeader(http.StatusNoContent)
}
