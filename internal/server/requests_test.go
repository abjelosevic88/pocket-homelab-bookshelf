package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// "Get it": the book page asks Pocket Requests (through Core, as the person)
// whether the book is here or on its way, and asks for an ebook or audiobook.
func TestGetItThroughRequests(t *testing.T) {
	h := newShelfHarness(t)
	var mu sync.Mutex
	var posted []map[string]any
	asked := map[string]bool{}
	rq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pcx_rq_ana" {
			http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/books/lookup":
			q := r.URL.Query()
			if q.Get("isbn") != "9780593098233" || q.Get("title") != "Dune Messiah" || q.Get("author") != "Frank Herbert" {
				_ = json.NewEncoder(w).Encode(map[string]any{"title": nil})
				return
			}
			var ebook any
			if asked["ebook"] {
				ebook = map[string]any{"id": 41, "status": "approved", "state": "downloading", "progress": 0.4}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"title": map[string]any{"kind": "book", "book_key": "ol:OL893415W", "title": "Dune Messiah",
				"subtitle": "Frank Herbert", "year": 1969, "poster": "",
				"book": map[string]any{
					"have":     map[string]any{"ebook": nil, "audiobook": map[string]any{"url": "https://audio.example.com/book/abc"}},
					"requests": map[string]any{"ebook": ebook, "audiobook": nil}}}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/requests":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			posted = append(posted, in)
			if asked[in["kind"].(string)] {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "You already asked for Dune Messiah.", "request_id": 41})
				return
			}
			asked[in["kind"].(string)] = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 41, "kind": in["kind"], "title": "Dune Messiah", "status": "approved", "state": "sent"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rq.Close)
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			User   string   `json:"user"`
			App    string   `json:"app"`
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.App != "requests" || strings.Join(in.Scopes, " ") != "requests:read requests:write" || in.User != "ana@example.com" {
			http.Error(w, `{"error":"switched off","reason":"off_person"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_rq_ana", "url": rq.URL, "public_url": "https://get.example.com"})
	}

	var anaB, bobB struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune Messiah", "authors": "Frank Herbert, Someone Else", "isbn13": "9780593098233", "status": "want"}, &anaB)
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Solaris", "authors": "Stanisław Lem", "status": "want"}, &bobB)
	ana, bob := anaB.Book, bobB.Book

	var g getIt
	h.json(h.anaTok, "GET", "/books/"+strconv.FormatInt(ana.ID, 10)+"/requests", nil, &g)
	if !g.Available || !g.Found || g.BookKey != "ol:OL893415W" || g.Audiobook.Have == nil || g.Ebook.Have != nil || g.Ebook.Request != nil {
		t.Fatalf("first look: %+v", g)
	}

	var made rqRequest
	h.json(h.anaTok, "POST", "/books/"+strconv.FormatInt(ana.ID, 10)+"/requests", map[string]any{"kind": "ebook"}, &made)
	if made.ID != 41 || made.URL != "https://get.example.com/requests/41" {
		t.Fatalf("made: %+v", made)
	}
	if p := posted[0]; p["book_key"] != "ol:OL893415W" || p["isbn"] != "9780593098233" || p["author"] != "Frank Herbert" || p["title"] != "Dune Messiah" {
		t.Fatalf("posted: %v", p)
	}
	// the cache was dropped: the ebook now shows as on its way
	h.json(h.anaTok, "GET", "/books/"+strconv.FormatInt(ana.ID, 10)+"/requests", nil, &g)
	if g.Ebook.Request == nil || g.Ebook.Request.State != "downloading" || g.Ebook.Request.URL != "https://get.example.com/requests/41" {
		t.Fatalf("after asking: %+v", g.Ebook)
	}

	// asking twice: Requests' 409 comes through with a link to the request
	code, body := h.do(h.anaTok, "POST", "/books/"+strconv.FormatInt(ana.ID, 10)+"/requests", map[string]any{"kind": "ebook"})
	if code != http.StatusConflict || !strings.Contains(string(body), "/requests/41") {
		t.Fatalf("twice: %d %s", code, body)
	}
	if code, _ := h.do(h.anaTok, "POST", "/books/"+strconv.FormatInt(ana.ID, 10)+"/requests", map[string]any{"kind": "film"}); code != http.StatusBadRequest {
		t.Fatalf("bad kind: %d", code)
	}

	// Bob's link is off: greyed out with the reason, not an error
	h.json(h.bobTok, "GET", "/books/"+strconv.FormatInt(bob.ID, 10)+"/requests", nil, &g)
	if g.Available || !strings.Contains(g.Reason, "Apps acting for me") {
		t.Fatalf("bob: %+v", g)
	}
	// and nobody reaches another person's book
	if code, _ := h.do(h.bobTok, "GET", "/books/"+strconv.FormatInt(ana.ID, 10)+"/requests", nil); code != http.StatusNotFound {
		t.Fatalf("bob on ana's book: %d", code)
	}
}

func TestBookKey(t *testing.T) {
	cases := map[[2]string]string{
		{"openlibrary", "/works/OL45804W"}: "ol:OL45804W",
		{"openlibrary", "/books/OL1M"}:     "",
		{"google", "zyTCAlFPjgYC"}:         "gb:zyTCAlFPjgYC",
		{"", ""}:                           "",
	}
	for in, want := range cases {
		b := &store.Book{Source: in[0], SourceID: in[1]}
		if got := bookKey(b); got != want {
			t.Errorf("%v: %q, want %q", in, got, want)
		}
	}
}
