package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// ---- Pocket Requests on author pages and on "+ Want" ----

// rqTitle is one of an author's books as Requests lists it.
type rqTitle struct {
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
		Asked []rqAsk `json:"asked"`
	} `json:"book"`
}

// authorGet is an author page's "Get from Pocket Requests" layer.
type authorGet struct {
	Available bool         `json:"available"`
	Reason    string       `json:"reason,omitempty"`
	Items     []authorBook `json:"items"`
	Languages []string     `json:"languages"`
	AppURL    string       `json:"app_url,omitempty"`
}

type authorBook struct {
	BookKey   string  `json:"book_key"`
	Title     string  `json:"title"`
	Year      int     `json:"year"`
	Poster    string  `json:"poster"`
	Ebook     *rqHave `json:"ebook"`     // here: open it
	Audiobook *rqHave `json:"audiobook"` // here: open it
	Asked     []rqAsk `json:"asked"`
}

// GET /api/v1/authors/{id}/requests — the author's books in Pocket Requests:
// which are here as ebooks or audiobooks and which you asked for.
func (s *Server) handleAuthorRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, err := s.store.GetAuthor(ctx, me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	u, err := s.store.GetUser(ctx, me(r))
	if err != nil {
		s.fail(w, err, "get user")
		return
	}
	out := authorGet{Items: []authorBook{}, Languages: []string{}}
	if !s.core.Configured() {
		out.Reason = "Needs Pocket Core: books are asked for in Pocket Requests, through Pocket Core."
		writeJSON(w, http.StatusOK, out)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c, le, err := s.rqFor(cctx, u.Email)
	if le != nil {
		out.Reason = rqLinkReason(le)
		writeJSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		out.Reason = "Pocket Core: " + err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	var res struct {
		Items     []rqTitle `json:"items"`
		Languages []string  `json:"languages_wanted"`
	}
	if err := c.do(cctx, http.MethodGet, "/api/v1/books/author", url.Values{"name": {a.Name}}, nil, &res); err != nil {
		var re *rqError
		if errors.As(err, &re) && re.status == http.StatusNotFound {
			out.Reason = "This Pocket Requests can't list an author's books yet; update it."
		} else {
			out.Reason = err.Error()
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Available, out.AppURL = true, c.public
	if res.Languages != nil {
		out.Languages = res.Languages
	}
	for _, t := range res.Items {
		b := authorBook{BookKey: t.BookKey, Title: t.Title, Year: t.Year, Poster: t.Poster, Ebook: t.Book.Have.Ebook, Audiobook: t.Book.Have.Audiobook, Asked: []rqAsk{}}
		for _, x := range t.Book.Asked {
			x.URL = c.requestURL(x.ID)
			b.Asked = append(b.Asked, x)
		}
		out.Items = append(out.Items, b)
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/v1/authors/{id}/requests {"kind","book_key","title","language"} — ask
// for one of the author's books that isn't on the shelf.
func (s *Server) handleCreateAuthorRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Kind     string `json:"kind"`
		BookKey  string `json:"book_key"`
		Title    string `json:"title"`
		Language string `json:"language"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Kind != "ebook" && in.Kind != "audiobook" {
		writeErr(w, http.StatusBadRequest, "kind is ebook or audiobook")
		return
	}
	if strings.TrimSpace(in.BookKey) == "" && strings.TrimSpace(in.Title) == "" {
		writeErr(w, http.StatusBadRequest, "say which book (book_key or title)")
		return
	}
	a, err := s.store.GetAuthor(ctx, me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	u, err := s.store.GetUser(ctx, me(r))
	if err != nil {
		s.fail(w, err, "get user")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	rq, err := s.askRequests(cctx, u, rqAskInput{Kind: in.Kind, Language: in.Language, BookKey: in.BookKey, Title: in.Title, Author: a.Name})
	writeAskResult(w, rq, err)
}

// ---- "+ Want": also get it ----

// wantGetModes are the choices for "When I add a book to Want to read".
var wantGetModes = map[string][]string{"": nil, "ebook": {"ebook"}, "audiobook": {"audiobook"}, "both": {"ebook", "audiobook"}}

// wantGetResult is what happened to one format after a book was added.
type wantGetResult struct {
	Kind string `json:"kind"`
	OK   bool   `json:"ok"`
	Msg  string `json:"message"`
	URL  string `json:"url,omitempty"`
}

// afterWant runs right after a book lands on Want to read: with the
// person's setting it asks Requests for the ebook / audiobook; without it, it
// says whether "Also get it?" can be offered. nil = nothing to say.
func (s *Server) afterWant(ctx context.Context, u *store.User, b *store.Book) map[string]any {
	if !s.core.Configured() {
		return nil
	}
	mode := s.store.UserKey(ctx, u.ID, "want_get")
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if _, le, err := s.rqFor(cctx, u.Email); le != nil || err != nil {
		return nil // the link is off: never ask on the person's behalf, offer nothing
	}
	kinds := wantGetModes[mode]
	if len(kinds) == 0 {
		return map[string]any{"offer": true}
	}
	results := []wantGetResult{}
	for _, kind := range kinds {
		rq, err := s.askRequests(cctx, u, rqAskInput{Kind: kind, Book: b})
		res := wantGetResult{Kind: kind}
		var re *rqError
		switch {
		case err == nil:
			res.OK, res.Msg, res.URL = true, "asked for", rq.URL
		case errors.As(err, &re) && re.status == http.StatusConflict:
			res.OK, res.Msg, res.URL = true, re.msg, re.url // already asked or already here: fine
		default:
			res.Msg = err.Error()
		}
		results = append(results, res)
	}
	return map[string]any{"asked": results}
}

// GET/PUT /api/v1/requests/settings — the person's "+ Want" choice and whether Requests can be reached.
func (s *Server) handleRequestsSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, err := s.store.GetUser(ctx, me(r))
	if err != nil {
		s.fail(w, err, "get user")
		return
	}
	if r.Method == http.MethodPut {
		var in struct {
			WantGet string `json:"want_get"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if _, ok := wantGetModes[in.WantGet]; !ok {
			writeErr(w, http.StatusBadRequest, "want_get is empty, ebook, audiobook or both")
			return
		}
		if err := s.store.PutUserKey(ctx, u.ID, "want_get", in.WantGet); err != nil {
			s.fail(w, err, "save setting")
			return
		}
	}
	out := map[string]any{"core": s.core.Configured(), "want_get": s.store.UserKey(ctx, u.ID, "want_get"), "available": false}
	if s.core.Configured() {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		c, le, err := s.rqFor(cctx, u.Email)
		switch {
		case le != nil:
			out["reason"] = rqLinkReason(le)
		case err != nil:
			out["reason"] = "Pocket Core: " + err.Error()
		default:
			out["available"], out["app_url"] = true, c.public
		}
	} else {
		out["reason"] = "Needs Pocket Core: books are asked for in Pocket Requests, through Pocket Core."
	}
	writeJSON(w, http.StatusOK, out)
}
