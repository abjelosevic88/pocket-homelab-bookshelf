package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// Ebooks from Pocket Library, through Pocket Core (link reading:read): a book
// a person finishes there — in its web reader, KOReader or a CrossPoint — is a
// read here on the day they finished it; a book they are reading goes on
// "Reading" with its page; their highlights and notes become quotes. A book
// is matched to one on the shelves by ISBN, else by title and author, or added
// as an ebook with its cover. Each finish, start and highlight is taken in
// once, and a book the person removes is never added again. Each person can
// switch it off (setting library_sync = "off").
//
// Links and events share the audiobook tables, keyed "library:<book id>" and
// "library-note:<annotation id>", which never clash with Audiobookshelf ids.

var libraryUse = core.Use{App: "library", Scopes: []string{"reading:read"}, Reason: "Ebooks you finish (or are reading) in Pocket Library, as reads on your shelves, and your highlights there as quotes"}

const libraryEvery = 10 * time.Minute

var (
	libMu     sync.Mutex
	libStates = map[int64]libState{}
	libBusy   = map[int64]bool{}
)

// libState is how the last sync went for one person (shown in Settings).
type libState struct {
	At       time.Time `json:"at"`
	Added    int       `json:"added"`
	Finished int       `json:"finished"`
	Started  int       `json:"started"`
	Quotes   int       `json:"quotes"`
	Error    string    `json:"error,omitempty"`
	Off      bool      `json:"off,omitempty"`
}

// libraryWorker syncs everyone every ten minutes (and soon after a start).
func (s *Server) libraryWorker(ctx context.Context) {
	first := time.NewTimer(2 * time.Minute)
	defer first.Stop()
	t := time.NewTicker(libraryEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		case <-t.C:
		}
		if !s.core.Configured() {
			continue
		}
		users, err := s.store.ListUsers(ctx)
		if err != nil {
			continue
		}
		for _, u := range users {
			if s.store.UserKey(ctx, u.ID, "library_sync") == "off" {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			st := s.syncLibrary(cctx, u.ID, u.Email)
			cancel()
			if st.Error != "" && !st.Off {
				s.log.Warn("library sync", "user", u.ID, "err", st.Error)
			}
		}
	}
}

// libClient reaches the person's Pocket Library with a fresh link token.
type libClient struct {
	base, public, token string
}

func (c *libClient) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	res, err := abHTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("Pocket Library answered HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *libClient) cover(ctx context.Context, coverURL string) ([]byte, string, error) {
	if coverURL == "" {
		return nil, "", errors.New("no cover")
	}
	sep := "?"
	if strings.Contains(coverURL, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+coverURL+sep+"w=600", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := abHTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	return b, strings.Split(res.Header.Get("Content-Type"), ";")[0], err
}

func (s *Server) libFor(ctx context.Context, email string) (*libClient, *core.LinkError, error) {
	lt, err := s.core.ActAs(ctx, email, "library", "reading:read")
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			return nil, le, nil
		}
		return nil, nil, err
	}
	return &libClient{base: strings.TrimRight(lt.URL, "/"), public: strings.TrimRight(lt.PublicURL, "/"), token: lt.AccessToken}, nil, nil
}

// libLinkReason says why Bookshelf can't reach Pocket Library for the person.
func libLinkReason(le *core.LinkError) string {
	switch le.Reason {
	case "target_missing":
		return "Pocket Library is not on this box."
	case "off_person":
		return "Switched off for you: turn on “Pocket Bookshelf → Pocket Library” in Pocket Core, Settings → Apps acting for me."
	case "off_admin":
		return "Switched off for this box: an admin turns on “Pocket Bookshelf → Pocket Library” in Pocket Core, Links."
	case "not_in_audience":
		return "You have no Pocket Library: an admin can give you access in Pocket Core."
	case "not_offered", "not_declared":
		return "This Pocket Library or Bookshelf is too old to link the two; update both."
	case "unknown_user":
		return "Your account is not in Pocket Core."
	}
	return "Pocket Library: " + le.Message
}

// libBookInfo is a book as Pocket Library's reading feed describes it.
type libBookInfo struct {
	BookID      int64             `json:"book_id"`
	Title       string            `json:"title"`
	Subtitle    string            `json:"subtitle"`
	Authors     []string          `json:"authors"`
	ISBN        string            `json:"isbn"`
	Identifiers map[string]string `json:"identifiers"`
	Series      string            `json:"series"`
	SeriesIndex float64           `json:"series_index"`
	Pages       int               `json:"pages"`
	Language    string            `json:"language"`
	Published   string            `json:"published"`
	Cover       string            `json:"cover"`
	Kind        string            `json:"kind"`
	Status      string            `json:"status"`
	Percentage  float64           `json:"percentage"`
	StartedAt   string            `json:"started_at"`
	FinishedAt  string            `json:"finished_at"`
}

type libRead struct {
	libBookInfo
	ID        int64  `json:"id"`
	ReadState string `json:"status"`
	Day       string `json:"day"`
}

type libFeed struct {
	Now   string        `json:"now"`
	Books []libBookInfo `json:"books"`
	Reads []libRead     `json:"reads"`
}

type libNote struct {
	ID         int64   `json:"id"`
	BookID     int64   `json:"book_id"`
	Kind       string  `json:"kind"`
	Text       string  `json:"text"`
	Note       string  `json:"note"`
	Chapter    string  `json:"chapter"`
	Percentage float64 `json:"percentage"`
	Page       int     `json:"page"`
	CreatedAt  string  `json:"created_at"`
}

func libKey(id int64) string { return "library:" + strconv.FormatInt(id, 10) }

// syncLibrary takes in one person's reading from Pocket Library.
func (s *Server) syncLibrary(ctx context.Context, uid int64, email string) libState {
	libMu.Lock()
	if libBusy[uid] {
		st := libStates[uid]
		libMu.Unlock()
		return st
	}
	libBusy[uid] = true
	libMu.Unlock()
	st := libState{At: time.Now()}
	defer func() {
		libMu.Lock()
		libStates[uid] = st
		delete(libBusy, uid)
		libMu.Unlock()
	}()
	c, le, err := s.libFor(ctx, email)
	if err != nil {
		st.Error = "Pocket Core did not answer: " + err.Error()
		return st
	}
	if le != nil {
		st.Off, st.Error = true, libLinkReason(le)
		return st
	}
	since := s.store.UserKey(ctx, uid, "library_since")
	if since == "" {
		since = "2000-01-01T00:00:00Z" // the first time: everything
	}
	var feed libFeed
	if err := c.get(ctx, "/api/v1/reading/feed", url.Values{"since": {since}}, &feed); err != nil {
		st.Error = err.Error()
		return st
	}
	shelf, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid})
	if err != nil {
		st.Error = err.Error()
		return st
	}
	books := map[int64]libBookInfo{}
	for _, b := range feed.Books {
		books[b.BookID] = b
	}
	finished := map[int64]bool{}
	for _, rd := range feed.Reads {
		if rd.ReadState != "read" || rd.Day == "" {
			continue
		}
		finished[rd.BookID] = true
		key := libKey(rd.BookID)
		if s.store.AudiobookEventSeen(ctx, uid, key, "finished", rd.Day) {
			continue
		}
		info := rd.libBookInfo
		bookID, added, ok := s.libBook(ctx, c, uid, info, store.StatusRead, "", &shelf)
		_ = s.store.MarkAudiobookEvent(ctx, uid, key, "finished", rd.Day)
		if !ok {
			continue
		}
		start := ""
		if len(rd.StartedAt) >= 10 && rd.StartedAt[:10] <= rd.Day {
			start = rd.StartedAt[:10]
		}
		changed, err := s.store.FinishFromAudiobook(ctx, uid, bookID, start, rd.Day)
		if err != nil {
			st.Error = err.Error()
			return st
		}
		if added {
			st.Added++
		}
		if changed {
			st.Finished++
		}
	}
	for id, b := range books {
		if b.Status != "reading" || finished[id] || b.Percentage <= 0 {
			continue
		}
		start := ""
		if len(b.StartedAt) >= 10 {
			start = b.StartedAt[:10]
		}
		key := libKey(id)
		seen := start == "" || s.store.AudiobookEventSeen(ctx, uid, key, "started", start)
		bookID, added, ok := s.libBook(ctx, c, uid, b, store.StatusReading, start, &shelf)
		if !ok {
			continue
		}
		if !seen {
			_ = s.store.MarkAudiobookEvent(ctx, uid, key, "started", start)
			if added {
				st.Added++
				st.Started++
			} else if changed, err := s.store.StartFromAudiobook(ctx, uid, bookID, start); err == nil && changed {
				st.Started++
			}
		}
		// the page follows the position, for books still being read here
		if bk, err := s.store.GetBook(ctx, uid, bookID); err == nil && bk.Status == store.StatusReading {
			pages := bk.Pages
			if pages == 0 && b.Pages > 0 {
				bk.Pages, pages = b.Pages, b.Pages
			}
			if pages > 0 {
				if page := int(math.Round(b.Percentage * float64(pages))); page != bk.CurrentPage && page <= pages {
					bk.CurrentPage = page
					_ = s.store.UpdateBook(ctx, bk)
				}
			}
		}
	}
	st.Quotes = s.libQuotes(ctx, c, uid, &shelf)
	if feed.Now != "" {
		_ = s.store.PutUserKey(ctx, uid, "library_since", feed.Now)
	}
	return st
}

// libQuotes takes the person's highlights and notes in as quotes on books
// that are on the shelves through this link (each one once).
func (s *Server) libQuotes(ctx context.Context, c *libClient, uid int64, shelf *[]store.Book) int {
	var notes []libNote
	if err := c.get(ctx, "/api/v1/annotations", url.Values{"limit": {"2000"}}, &notes); err != nil {
		return 0
	}
	n := 0
	for _, a := range notes {
		if a.Kind == "bookmark" || strings.TrimSpace(a.Text) == "" {
			continue
		}
		key := "library-note:" + strconv.FormatInt(a.ID, 10)
		if s.store.AudiobookEventSeen(ctx, uid, key, "quote", "") {
			continue
		}
		bookID, found := s.store.AudiobookLink(ctx, uid, libKey(a.BookID))
		if !found {
			continue // only books the link brought in (or matched)
		}
		page := ""
		switch {
		case a.Page > 0:
			page = strconv.Itoa(a.Page)
		case a.Percentage > 0:
			page = fmt.Sprintf("%d%%", int(math.Round(a.Percentage*100)))
		}
		q := &store.Quote{BookID: bookID, Text: strings.TrimSpace(a.Text), Page: page, Note: strings.TrimSpace(a.Note)}
		if err := s.store.CreateQuote(ctx, uid, q); err != nil {
			continue
		}
		_ = s.store.MarkAudiobookEvent(ctx, uid, key, "quote", "")
		n++
	}
	return n
}

// libBook finds the book on the person's shelves, or adds it as an ebook
// with the cover from Pocket Library. ok is false when the person removed
// the book it was linked to.
func (s *Server) libBook(ctx context.Context, c *libClient, uid int64, info libBookInfo, status, startedOn string, shelf *[]store.Book) (bookID int64, added, ok bool) {
	key := libKey(info.BookID)
	if id, found := s.store.AudiobookLink(ctx, uid, key); found {
		if _, err := s.store.GetBook(ctx, uid, id); err != nil {
			return 0, false, false
		}
		return id, false, true
	}
	authors := strings.Join(info.Authors, ", ")
	isbn := strings.ReplaceAll(info.ISBN, "-", "")
	want := abNorm(info.Title)
	for _, b := range *shelf {
		if isbn != "" && (b.ISBN13 == isbn || b.ISBN10 == isbn) || b.Source == "library" && b.SourceID == strconv.FormatInt(info.BookID, 10) ||
			want != "" && abNorm(b.Title) == want && abSameAuthor(b.Authors, authors) {
			_ = s.store.SetAudiobookLink(ctx, uid, key, b.ID)
			return b.ID, false, true
		}
	}
	if strings.TrimSpace(info.Title) == "" {
		return 0, false, false
	}
	b := &store.Book{UserID: uid, Title: strings.TrimSpace(info.Title), Subtitle: info.Subtitle, Authors: authors, Pages: info.Pages,
		Published: info.Published, Language: info.Language, Format: "ebook", Status: status, Source: "library", SourceID: strconv.FormatInt(info.BookID, 10),
		Series: info.Series}
	if len(isbn) == 13 {
		b.ISBN13 = isbn
	} else if len(isbn) == 10 {
		b.ISBN10 = isbn
	}
	if info.Series != "" && info.SeriesIndex > 0 {
		b.SeriesNo = strconv.FormatFloat(info.SeriesIndex, 'f', -1, 64)
	}
	if c.public != "" {
		b.SourceURL = c.public + "/book/" + strconv.FormatInt(info.BookID, 10)
	}
	if err := s.store.CreateBook(ctx, b, startedOn, ""); err != nil {
		s.log.Warn("library: add book", "title", info.Title, "err", err)
		return 0, false, false
	}
	_ = s.store.SetAudiobookLink(ctx, uid, key, b.ID)
	if data, ct, err := c.cover(ctx, info.Cover); err == nil && len(data) > 0 {
		if err := s.writeCover(ctx, b, data, ct); err != nil {
			s.log.Debug("library: cover", "title", info.Title, "err", err)
		}
	}
	*shelf = append(*shelf, *b)
	return b.ID, true, true
}

// ---- API ----

// GET /api/v1/library — whether ebooks come in, and how the last sync went.
func (s *Server) handleLibraryStatus(w http.ResponseWriter, r *http.Request) {
	uid := me(r)
	libMu.Lock()
	st, ran := libStates[uid]
	busy := libBusy[uid]
	libMu.Unlock()
	out := map[string]any{"core": s.core.Configured(), "enabled": s.store.UserKey(r.Context(), uid, "library_sync") != "off", "syncing": busy}
	if ran {
		out["last"] = st
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /api/v1/library {"enabled": bool}
func (s *Server) handleLibrarySet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	v := "on"
	if !in.Enabled {
		v = "off"
	}
	if err := s.store.PutUserKey(r.Context(), me(r), "library_sync", v); err != nil {
		s.fail(w, err, "library")
		return
	}
	s.handleLibraryStatus(w, r)
}

// POST /api/v1/library/sync — take in the caller's reading now.
func (s *Server) handleLibrarySync(w http.ResponseWriter, r *http.Request) {
	if !s.core.Configured() {
		writeErr(w, http.StatusConflict, "ebooks come from Pocket Library through Pocket Core, which this Bookshelf is not joined to")
		return
	}
	u, err := s.store.GetUser(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "library")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	writeJSON(w, http.StatusOK, s.syncLibrary(ctx, u.ID, u.Email))
}
