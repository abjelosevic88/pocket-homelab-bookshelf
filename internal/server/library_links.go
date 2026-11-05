package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Linking a book on the shelves to the same book in Pocket Library, by hand
// or from Library itself (link scopes books:read / books:write). The link is
// the row "library:<library book id>" in audiobook_links that the reading
// sync uses; unlinking leaves a tombstone (book id 0), so the sync never
// matches or adds that Library book again.

// libPublic is Pocket Library's public address, as Core last handed it out.
var libPublic atomic.Value // string

func libPublicURL() string {
	v, _ := libPublic.Load().(string)
	return v
}

// libraryURLFor is where a Library book opens, or "" when Library's address
// isn't known yet (books Library brought in carry it in their source URL).
func libraryURLFor(b *store.Book, libID int64) string {
	id := strconv.FormatInt(libID, 10)
	if base := libPublicURL(); base != "" {
		return base + "/book/" + id
	}
	if b != nil && b.Source == "library" && b.SourceURL != "" {
		if i := strings.LastIndex(b.SourceURL, "/book/"); i > 0 {
			return b.SourceURL[:i] + "/book/" + id
		}
	}
	return ""
}

// learnLibraryURL asks Core once for Library's address when nothing has told
// us yet, so a linked book's page can open it.
func (s *Server) learnLibraryURL(ctx context.Context, email string) {
	if libPublicURL() != "" || !s.core.Configured() || email == "" {
		return
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _, _ = s.libFor(c, email)
}

// withLibraryLinks fills in the Library link of books on one person's shelves.
func (s *Server) withLibraryLinks(ctx context.Context, uid int64, books []store.Book) {
	if len(books) == 0 {
		return
	}
	links, err := s.store.LibraryBooks(ctx, uid)
	if err != nil || len(links) == 0 {
		return
	}
	for i := range books {
		if id, ok := links[books[i].ID]; ok {
			books[i].LibraryBookID = id
			books[i].LibraryURL = libraryURLFor(&books[i], id)
		}
	}
}

// libraryLinkView is what GET /library/links/{id} answers.
func (s *Server) libraryLinkView(ctx context.Context, uid, libID int64) map[string]any {
	bookID, found := s.store.AudiobookLink(ctx, uid, libKey(libID))
	if !found {
		return map[string]any{"linked": false}
	}
	if bookID == 0 {
		return map[string]any{"linked": false, "unlinked": true}
	}
	b, err := s.store.GetBook(ctx, uid, bookID)
	if err != nil {
		return map[string]any{"linked": false} // the book was removed from the shelves
	}
	return map[string]any{"linked": true, "book": map[string]any{
		"id": b.ID, "title": b.Title, "authors": b.Authors, "status": b.Status, "rating": b.Rating,
		"url": strings.TrimRight(s.cfg.BaseURL, "/") + fmt.Sprintf("/books/%d", b.ID),
	}}
}

func libIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id := idParam(r, "libID")
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "not a Pocket Library book id")
		return 0, false
	}
	return id, true
}

// GET /api/v1/library/links/{libID}
func (s *Server) handleLibraryLinkGet(w http.ResponseWriter, r *http.Request) {
	libID, ok := libIDParam(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.libraryLinkView(r.Context(), me(r), libID))
}

// PUT /api/v1/library/links/{libID} {"book_id": n}
func (s *Server) handleLibraryLinkPut(w http.ResponseWriter, r *http.Request) {
	libID, ok := libIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		BookID int64 `json:"book_id"`
	}
	if err := decode(r, &in); err != nil || in.BookID <= 0 {
		writeErr(w, http.StatusBadRequest, "book_id is required")
		return
	}
	ctx, uid := r.Context(), me(r)
	if _, err := s.store.GetBook(ctx, uid, in.BookID); err != nil {
		s.fail(w, err, "link book")
		return
	}
	if err := s.store.SetAudiobookLink(ctx, uid, libKey(libID), in.BookID); err != nil {
		s.fail(w, err, "link book")
		return
	}
	writeJSON(w, http.StatusOK, s.libraryLinkView(ctx, uid, libID))
}

// DELETE /api/v1/library/links/{libID}
func (s *Server) handleLibraryLinkDelete(w http.ResponseWriter, r *http.Request) {
	libID, ok := libIDParam(w, r)
	if !ok {
		return
	}
	if err := s.store.SetAudiobookLink(r.Context(), me(r), libKey(libID), 0); err != nil {
		s.fail(w, err, "unlink book")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"linked": false, "unlinked": true})
}

// POST /api/v1/library/links/{libID}/apply — Pocket Library sets a book's
// status and/or rating here: the linked book, else one matched or added the
// way the reading sync does it.
func (s *Server) handleLibraryLinkApply(w http.ResponseWriter, r *http.Request) {
	libID, ok := libIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Book   libBookInfo `json:"book"`
		Status string      `json:"status"`
		Day    string      `json:"day"`
		Rating *float64    `json:"rating"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Status != "" && !store.ValidStatus(in.Status) {
		writeErr(w, http.StatusBadRequest, "status must be want, reading, read or abandoned")
		return
	}
	if in.Day == "" {
		in.Day = store.Today()
	} else if !validDate(in.Day) {
		writeErr(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
		return
	}
	if in.Rating != nil {
		v := *in.Rating
		if v != 0 && (v < 0.5 || v > 5 || math.Mod(v*2, 1) != 0) {
			writeErr(w, http.StatusBadRequest, "rating is 0 (none) or 0.5 to 5 in halves")
			return
		}
	}
	ctx, uid, u := r.Context(), me(r), userFrom(r.Context())
	key := libKey(libID)
	if id, found := s.store.AudiobookLink(ctx, uid, key); found && id == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "unlinked"})
		return
	}
	in.Book.BookID = libID
	shelf, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid})
	if err != nil {
		s.fail(w, err, "apply")
		return
	}
	// the cover comes along when Library can be reached for the person
	var c *libClient
	if u != nil && s.core.Configured() {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		c, _, _ = s.libFor(cctx, u.Email)
		cancel()
	}
	status := in.Status
	if status == "" {
		status = store.StatusWant // a book added only to be rated
	}
	start := ""
	if status == store.StatusReading {
		start = in.Day
	}
	bookID, added, ok := s.libBook(ctx, c, uid, in.Book, status, start, &shelf)
	if !ok {
		if strings.TrimSpace(in.Book.Title) == "" {
			writeErr(w, http.StatusBadRequest, "book.title is required to add the book")
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{"error": "unlinked"})
		return
	}
	if added {
		s.kickAuthors()
	}
	if in.Status != "" {
		if err := s.applyLibraryStatus(ctx, u, uid, bookID, key, in.Status, in.Day, added); err != nil {
			s.fail(w, err, "apply status")
			return
		}
	}
	if in.Rating != nil {
		b, err := s.store.GetBook(ctx, uid, bookID)
		if err != nil {
			s.fail(w, err, "apply rating")
			return
		}
		if b.Rating != *in.Rating {
			b.Rating = *in.Rating
			if err := s.store.UpdateBook(ctx, b, s.bookRatedEvent(ctx, u, b, true)); err != nil {
				s.fail(w, err, "apply rating")
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, s.libraryLinkView(ctx, uid, libID))
}

// applyLibraryStatus moves a book to the status Library set, the way the
// reading sync would, and marks the day so the sync doesn't do it again.
func (s *Server) applyLibraryStatus(ctx context.Context, u *store.User, uid, bookID int64, key, status, day string, added bool) error {
	b, err := s.store.GetBook(ctx, uid, bookID)
	if err != nil {
		return err
	}
	switch status {
	case store.StatusRead:
		if added || b.Status != store.StatusRead { // a book added as read has no read yet
			if _, err := s.store.FinishFromAudiobook(ctx, uid, bookID, "", day); err != nil {
				return err
			}
		}
		return s.store.MarkAudiobookEvent(ctx, uid, key, "finished", day)
	case store.StatusReading:
		if !added && b.Status != store.StatusReading {
			if _, err := s.store.SetStatus(ctx, uid, bookID, store.StatusReading, day, s.bookStatusEvent(ctx, u, day)); err != nil {
				return err
			}
		}
		return s.store.MarkAudiobookEvent(ctx, uid, key, "started", day)
	default:
		if b.Status == status {
			return nil
		}
		_, err := s.store.SetStatus(ctx, uid, bookID, status, day, s.bookStatusEvent(ctx, u, day))
		return err
	}
}

// GET /api/v1/library/search?q= — books in the person's Pocket Library, to
// link one by hand (link library:read).
func (s *Server) handleLibrarySearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	u := userFrom(r.Context())
	if !s.core.Configured() || u == nil {
		writeErr(w, http.StatusConflict, "books are linked to Pocket Library through Pocket Core, which this Bookshelf is not joined to")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	c, le, err := s.libForScope(ctx, u.Email, "library:read")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Core did not answer: "+err.Error())
		return
	}
	if le != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": libLinkReason(le), "off": true})
		return
	}
	out := []map[string]any{}
	if q == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	var page struct {
		Items []struct {
			ID      int64  `json:"id"`
			Title   string `json:"title"`
			Cover   string `json:"cover"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Series      string  `json:"series"`
			SeriesIndex float64 `json:"series_index"`
			Language    string  `json:"language"`
		} `json:"items"`
	}
	if err := c.get(ctx, "/api/v1/books", url.Values{"q": {q}, "limit": {"20"}}, &page); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	pub := c.public
	if pub == "" {
		pub = c.base
	}
	for _, b := range page.Items {
		names := make([]string, 0, len(b.Authors))
		for _, a := range b.Authors {
			names = append(names, a.Name)
		}
		cover := ""
		if b.Cover != "" {
			cover = b.Cover
			if strings.HasPrefix(cover, "/") {
				cover = pub + cover
			}
		}
		out = append(out, map[string]any{"id": b.ID, "title": b.Title, "authors": strings.Join(names, ", "), "cover_url": cover,
			"series": b.Series, "series_index": b.SeriesIndex, "language": b.Language, "url": fmt.Sprintf("%s/book/%d", pub, b.ID)})
	}
	writeJSON(w, http.StatusOK, out)
}
