package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// "Get it": a book on the shelves can be asked for as an ebook or an
// audiobook in Pocket Requests, through Pocket Core (link requests:read +
// requests:write). The book page shows, per format, whether it is already on
// the server (with a link to open it), on its way (the request's state, with a
// link to the request) or can be asked for. Bookshelf talks to Requests on the
// person's behalf; the browser never sees the link token.

var requestsUse = core.Use{App: "requests", Scopes: []string{"requests:read", "requests:write"},
	Reason: "Ask for an ebook or audiobook from your shelves (a Get it button) and show whether it is on its way"}

// rqClient reaches the person's Pocket Requests with a fresh link token.
type rqClient struct {
	base, public, token string
}

// rqError is Requests answering with an error.
type rqError struct {
	status    int
	msg       string
	requestID int64
	url       string // the request's page, when Requests named one
}

func (e *rqError) Error() string { return e.msg }

func (c *rqClient) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := abHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Pocket Requests didn't answer: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var e struct {
			Error     string `json:"error"`
			RequestID int64  `json:"request_id"`
		}
		_ = json.Unmarshal(b, &e)
		msg := strings.TrimSpace(e.Error)
		if msg == "" {
			msg = fmt.Sprintf("Pocket Requests answered HTTP %d", res.StatusCode)
		}
		return &rqError{status: res.StatusCode, msg: msg, requestID: e.RequestID}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

func (s *Server) rqFor(ctx context.Context, email string) (*rqClient, *core.LinkError, error) {
	lt, err := s.core.ActAs(ctx, email, "requests", "requests:read", "requests:write")
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			return nil, le, nil
		}
		return nil, nil, err
	}
	return &rqClient{base: strings.TrimRight(lt.URL, "/"), public: strings.TrimRight(lt.PublicURL, "/"), token: lt.AccessToken}, nil, nil
}

// rqLinkReason says why Bookshelf can't reach Pocket Requests for the person.
func rqLinkReason(le *core.LinkError) string {
	switch le.Reason {
	case "target_missing":
		return "Pocket Requests is not on this box."
	case "off_person":
		return "Switched off for you: turn on “Pocket Bookshelf → Pocket Requests” in Pocket Core, Settings → Apps acting for me."
	case "off_admin":
		return "Switched off for this box: an admin turns on “Pocket Bookshelf → Pocket Requests” in Pocket Core, Links."
	case "not_in_audience":
		return "You have no Pocket Requests: an admin can give you access in Pocket Core."
	case "not_offered", "not_declared":
		return "This Pocket Requests or Bookshelf is too old to link the two; update both."
	case "unknown_user":
		return "Your account is not in Pocket Core."
	}
	return "Pocket Requests: " + le.Message
}

// rqRequest is a request as Requests reports it.
type rqRequest struct {
	ID       int64   `json:"id"`
	Kind     string  `json:"kind"`
	Status   string  `json:"status"`
	State    string  `json:"state"`
	Detail   string  `json:"state_detail,omitempty"`
	Progress float64 `json:"progress"`
	URL      string  `json:"url,omitempty"` // filled in by Bookshelf: the request's page
}

type rqHave struct {
	URL string `json:"url"`
}

// rqLookup is GET /api/v1/books/lookup's answer.
type rqLookup struct {
	Title *struct {
		Kind     string `json:"kind"`
		BookKey  string `json:"book_key"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Year     int    `json:"year"`
		Poster   string `json:"poster"`
		Book     struct {
			Have struct {
				Ebook     *rqHave `json:"ebook"`
				Audiobook *rqHave `json:"audiobook"`
			} `json:"have"`
			Requests struct {
				Ebook     *rqRequest `json:"ebook"`
				Audiobook *rqRequest `json:"audiobook"`
			} `json:"requests"`
			Asked []rqAsk `json:"asked"`
		} `json:"book"`
	} `json:"title"`
	Languages []string `json:"languages_wanted"`
}

// rqAsk is one live request for a book (an ebook may be asked for in two languages).
type rqAsk struct {
	ID       int64   `json:"id"`
	Kind     string  `json:"kind"`
	Language string  `json:"language"`
	Status   string  `json:"status"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
	URL      string  `json:"url,omitempty"`
}

// getItFormat is one format on the book page.
type getItFormat struct {
	Have    *rqHave    `json:"have"`    // on the server: open it here
	Request *rqRequest `json:"request"` // asked for: how far along
}

// getIt is what the book page shows.
type getIt struct {
	Available bool        `json:"available"`        // Requests can be reached
	Reason    string      `json:"reason,omitempty"` // why not (greyed out with this)
	Found     bool        `json:"found"`            // Requests knows the book
	BookKey   string      `json:"book_key,omitempty"`
	Ebook     getItFormat `json:"ebook"`
	Audiobook getItFormat `json:"audiobook"`
	AppURL    string      `json:"app_url,omitempty"`
	// The languages the box reads (best first) and every live request, per language.
	Languages []string `json:"languages"`
	Asked     []rqAsk  `json:"asked"`
}

var (
	getItMu    sync.Mutex
	getItCache = map[string]getItCached{}
)

type getItCached struct {
	v  getIt
	at time.Time
}

const getItTTL = 45 * time.Second

func getItKey(uid, bookID int64) string {
	return strconv.FormatInt(uid, 10) + ":" + strconv.FormatInt(bookID, 10)
}

func forgetGetIt(uid, bookID int64) {
	getItMu.Lock()
	delete(getItCache, getItKey(uid, bookID))
	getItMu.Unlock()
}

// bookKey turns a book's metadata source into Requests' book key.
func bookKey(b *store.Book) string {
	id := strings.TrimSpace(b.SourceID)
	if id == "" {
		return ""
	}
	switch b.Source {
	case "openlibrary":
		id = strings.TrimPrefix(strings.TrimPrefix(id, "/works/"), "/books/")
		if strings.HasSuffix(id, "W") {
			return "ol:" + id
		}
	case "google", "googlebooks", "google_books":
		return "gb:" + id
	}
	return ""
}

func firstAuthor(authors string) string {
	for _, a := range strings.Split(authors, ",") {
		if a = strings.TrimSpace(a); a != "" {
			return a
		}
	}
	return ""
}

func (c *rqClient) requestURL(id int64) string {
	if c.public == "" || id == 0 {
		return ""
	}
	return c.public + "/requests/" + strconv.FormatInt(id, 10)
}

// lookupGetIt asks Requests about one of the person's books.
func (s *Server) lookupGetIt(ctx context.Context, u *store.User, b *store.Book, fresh bool) getIt {
	key := getItKey(u.ID, b.ID)
	if !fresh {
		getItMu.Lock()
		c, ok := getItCache[key]
		getItMu.Unlock()
		if ok && time.Since(c.at) < getItTTL {
			return c.v
		}
	}
	v := s.fetchGetIt(ctx, u, b)
	getItMu.Lock()
	getItCache[key] = getItCached{v, time.Now()}
	getItMu.Unlock()
	return v
}

func (s *Server) fetchGetIt(ctx context.Context, u *store.User, b *store.Book) getIt {
	if !s.core.Configured() {
		return getIt{Reason: "Needs Pocket Core: books are asked for in Pocket Requests, through Pocket Core."}
	}
	c, le, err := s.rqFor(ctx, u.Email)
	if le != nil {
		return getIt{Reason: rqLinkReason(le)}
	}
	if err != nil {
		return getIt{Reason: "Pocket Core: " + err.Error()}
	}
	out := getIt{Available: true, AppURL: c.public, Languages: []string{}, Asked: []rqAsk{}}
	q := url.Values{}
	if isbn := firstNonEmpty(b.ISBN13, b.ISBN10); isbn != "" {
		q.Set("isbn", isbn)
	}
	q.Set("title", b.Title)
	if a := firstAuthor(b.Authors); a != "" {
		q.Set("author", a)
	}
	var lk rqLookup
	if err := c.do(ctx, http.MethodGet, "/api/v1/books/lookup", q, nil, &lk); err != nil {
		var re *rqError
		if errors.As(err, &re) && re.status == http.StatusNotFound {
			return getIt{Reason: "This Pocket Requests can't find books yet; update it."}
		}
		return getIt{Reason: err.Error()}
	}
	if lk.Languages != nil {
		out.Languages = lk.Languages
	}
	if lk.Title == nil {
		return out
	}
	t := lk.Title
	out.Found, out.BookKey = true, t.BookKey
	out.Ebook.Have, out.Audiobook.Have = t.Book.Have.Ebook, t.Book.Have.Audiobook
	out.Ebook.Request, out.Audiobook.Request = t.Book.Requests.Ebook, t.Book.Requests.Audiobook
	for _, r := range []*rqRequest{out.Ebook.Request, out.Audiobook.Request} {
		if r != nil {
			r.URL = c.requestURL(r.ID)
		}
	}
	for _, a := range t.Book.Asked {
		a.URL = c.requestURL(a.ID)
		out.Asked = append(out.Asked, a)
	}
	return out
}

// ---- API ----

// GET /api/v1/books/{id}/requests — the book page's "Get it" box.
func (s *Server) handleBookRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, err := s.store.GetBook(ctx, me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	u, err := s.store.GetUser(ctx, b.UserID)
	if err != nil {
		s.fail(w, err, "get user")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.lookupGetIt(cctx, u, b, r.URL.Query().Get("fresh") == "1"))
}

// POST /api/v1/books/{id}/requests {"kind": "ebook"|"audiobook", "language", "note"} — ask for it.
func (s *Server) handleCreateBookRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Kind     string `json:"kind"`
		Language string `json:"language"`
		Note     string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Kind != "ebook" && in.Kind != "audiobook" {
		writeErr(w, http.StatusBadRequest, "kind is ebook or audiobook")
		return
	}
	b, err := s.store.GetBook(ctx, me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	u, err := s.store.GetUser(ctx, b.UserID)
	if err != nil {
		s.fail(w, err, "get user")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	rq, err := s.askRequests(cctx, u, rqAskInput{Kind: in.Kind, Language: in.Language, Note: in.Note, Book: b})
	writeAskResult(w, rq, err)
}

// rqAskInput is what Bookshelf asks Requests for: a book on the shelf, or one
// Requests already knows by its key (an author's other books).
type rqAskInput struct {
	Kind, Language, Note string
	Book                 *store.Book // a book on the shelf
	BookKey, Title       string      // else: a book by key
	Author               string
}

// askRequests asks Pocket Requests for an ebook or audiobook as the person.
func (s *Server) askRequests(ctx context.Context, u *store.User, in rqAskInput) (*rqRequest, error) {
	if !s.core.Configured() {
		return nil, &rqError{status: http.StatusConflict, msg: "Needs Pocket Core: books are asked for in Pocket Requests, through Pocket Core."}
	}
	c, le, err := s.rqFor(ctx, u.Email)
	if le != nil {
		return nil, &rqError{status: http.StatusConflict, msg: rqLinkReason(le)}
	}
	if err != nil {
		return nil, &rqError{status: http.StatusBadGateway, msg: "Pocket Core: " + err.Error()}
	}
	body := map[string]any{"kind": in.Kind}
	key, title, author := in.BookKey, in.Title, in.Author
	if b := in.Book; b != nil {
		title, author = b.Title, firstAuthor(b.Authors)
		// the key Requests found on the book page beats Bookshelf's own source id
		getItMu.Lock()
		if cached, ok := getItCache[getItKey(u.ID, b.ID)]; ok {
			key = cached.v.BookKey
		}
		getItMu.Unlock()
		key = firstNonEmpty(key, bookKey(b))
		if isbn := firstNonEmpty(b.ISBN13, b.ISBN10); isbn != "" {
			body["isbn"] = isbn
		}
		defer forgetGetIt(u.ID, b.ID)
	}
	if key != "" {
		body["book_key"] = key
	}
	if title != "" {
		body["title"] = title
	}
	if author != "" {
		body["author"] = author
	}
	if l := strings.ToLower(strings.TrimSpace(in.Language)); l != "" && l != "any" && len(l) <= 3 {
		body["language"] = l
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		if len(note) > 1000 {
			note = note[:1000]
		}
		body["note"] = note
	}
	var rq rqRequest
	if err := c.do(ctx, http.MethodPost, "/api/v1/requests", nil, body, &rq); err != nil {
		var re *rqError
		if errors.As(err, &re) && re.requestID > 0 {
			re.url = c.requestURL(re.requestID)
		}
		return nil, err
	}
	rq.URL = c.requestURL(rq.ID)
	return &rq, nil
}

// writeAskResult answers a "Get it" POST: 201 with the request, or Requests' error with its status.
func writeAskResult(w http.ResponseWriter, rq *rqRequest, err error) {
	if err == nil {
		writeJSON(w, http.StatusCreated, rq)
		return
	}
	var re *rqError
	if errors.As(err, &re) {
		status := re.status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		out := map[string]any{"error": re.msg}
		if re.requestID > 0 {
			out["request_id"] = re.requestID
			out["url"] = re.url
		}
		writeJSON(w, status, out)
		return
	}
	writeErr(w, http.StatusBadGateway, err.Error())
}
