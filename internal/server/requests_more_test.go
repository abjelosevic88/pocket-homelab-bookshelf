package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// fakeRequests is just enough Pocket Requests for author pages, languages and "+ Want".
func fakeRequests(t *testing.T, h *shelfHarness) (*sync.Mutex, *[]map[string]any) {
	var mu sync.Mutex
	posted := []map[string]any{}
	asked := map[string]bool{}
	rq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		j := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/books/author":
			if r.URL.Query().Get("name") != "Frank Herbert" {
				j(map[string]any{"items": []any{}, "languages_wanted": []string{"en", "sr"}})
				return
			}
			j(map[string]any{"languages_wanted": []string{"en", "sr"}, "items": []any{
				map[string]any{"book_key": "ol:OL893415W", "title": "Dune Messiah", "year": 1969, "book": map[string]any{"have": map[string]any{"ebook": map[string]any{"url": "https://library.example.com/book/7"}}, "asked": []any{}}},
				map[string]any{"book_key": "ol:OL893416W", "title": "Children of Dune", "year": 1976, "book": map[string]any{"have": map[string]any{}, "asked": []any{
					map[string]any{"id": 52, "kind": "audiobook", "language": "", "status": "approved", "state": "downloading", "progress": 0.5}}}},
			}})
		case r.Method == "GET" && r.URL.Path == "/api/v1/books/lookup":
			j(map[string]any{"title": map[string]any{"kind": "book", "book_key": "ol:OL893415W", "title": "Dune Messiah", "book": map[string]any{
				"have": map[string]any{}, "requests": map[string]any{}, "asked": []any{map[string]any{"id": 9, "kind": "ebook", "language": "sr", "status": "approved", "state": "searching"}}}},
				"languages_wanted": []string{"en", "sr"}})
		case r.Method == "POST" && r.URL.Path == "/api/v1/requests":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			posted = append(posted, in)
			k := in["kind"].(string) + "|" + anyStr(in["language"]) + "|" + anyStr(in["title"])
			if asked[k] {
				w.WriteHeader(http.StatusConflict)
				j(map[string]any{"error": "You already asked for it.", "request_id": 60})
				return
			}
			asked[k] = true
			w.WriteHeader(http.StatusCreated)
			j(map[string]any{"id": 60 + len(posted), "kind": in["kind"], "status": "approved", "state": "sent"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rq.Close)
	t.Cleanup(func() { getItMu.Lock(); getItCache = map[string]getItCached{}; getItMu.Unlock() })
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			User string `json:"user"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.User != "ana@example.com" {
			http.Error(w, `{"error":"switched off","reason":"off_person"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_rq_ana", "url": rq.URL, "public_url": "https://get.example.com"})
	}
	return &mu, &posted
}

func anyStr(v any) string {
	s, _ := v.(string)
	return s
}

func TestAuthorPageAndLanguages(t *testing.T) {
	h := newShelfHarness(t)
	mu, posted := fakeRequests(t, h)
	var b struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "read"}, &b)
	var authors []store.Author
	h.json(h.anaTok, "GET", "/authors", nil, &authors)
	if len(authors) != 1 {
		t.Fatalf("authors %v", authors)
	}
	aid := strconv.FormatInt(authors[0].ID, 10)
	var ag authorGet
	h.json(h.anaTok, "GET", "/authors/"+aid+"/requests", nil, &ag)
	if !ag.Available || len(ag.Items) != 2 || ag.Items[0].Ebook == nil || len(ag.Items[1].Asked) != 1 || ag.Items[1].Asked[0].URL != "https://get.example.com/requests/52" {
		t.Fatalf("author: %+v", ag)
	}
	var made rqRequest
	if code := h.json(h.anaTok, "POST", "/authors/"+aid+"/requests", map[string]any{"kind": "ebook", "book_key": "ol:OL893416W", "title": "Children of Dune", "language": "sr"}, &made); code != http.StatusCreated {
		t.Fatalf("author ask %d", code)
	}
	mu.Lock()
	p := (*posted)[0]
	mu.Unlock()
	if p["book_key"] != "ol:OL893416W" || p["author"] != "Frank Herbert" || p["language"] != "sr" {
		t.Fatalf("posted %v", p)
	}
	// the book page shows per-language asks and the box's languages
	var g getIt
	h.json(h.anaTok, "GET", "/books/"+strconv.FormatInt(b.Book.ID, 10)+"/requests", nil, &g)
	if len(g.Languages) != 2 || len(g.Asked) != 1 || g.Asked[0].Language != "sr" || g.Asked[0].URL == "" {
		t.Fatalf("get it: %+v", g)
	}
	// Bob's link is off: the author layer says why
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Solaris", "authors": "Stanisław Lem"}, nil)
	var bobAuthors []store.Author
	h.json(h.bobTok, "GET", "/authors", nil, &bobAuthors)
	h.json(h.bobTok, "GET", "/authors/"+strconv.FormatInt(bobAuthors[0].ID, 10)+"/requests", nil, &ag)
	if ag.Available || ag.Reason == "" {
		t.Fatalf("bob: %+v", ag)
	}
}

func TestWantAlsoGetsIt(t *testing.T) {
	h := newShelfHarness(t)
	mu, posted := fakeRequests(t, h)
	var out struct {
		Book  store.Book
		GetIt map[string]any `json:"get_it"`
	}
	// no choice yet: "Also get it?" is offered, nothing is asked
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune Messiah", "authors": "Frank Herbert", "status": "want"}, &out)
	if out.GetIt["offer"] != true || len(*posted) != 0 {
		t.Fatalf("offer: %v %v", out.GetIt, *posted)
	}
	// "both": asked straight away; asking again (409) is quiet success
	if code := h.json(h.anaTok, "PUT", "/requests/settings", map[string]any{"want_get": "both"}, nil); code != 200 {
		t.Fatalf("settings %d", code)
	}
	out.GetIt = nil
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Children of Dune", "authors": "Frank Herbert", "status": "want"}, &out)
	asked, _ := out.GetIt["asked"].([]any)
	mu.Lock()
	n := len(*posted)
	mu.Unlock()
	if len(asked) != 2 || n != 2 || asked[0].(map[string]any)["ok"] != true {
		t.Fatalf("both: %v (%d posted)", out.GetIt, n)
	}
	out.GetIt = nil
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Children of Dune", "authors": "Frank Herbert", "status": "want", "allow_duplicate": true}, &out)
	asked, _ = out.GetIt["asked"].([]any)
	if len(asked) != 2 || asked[1].(map[string]any)["ok"] != true {
		t.Fatalf("again: %v", out.GetIt)
	}
	// a read book asks for nothing; a person whose link is off gets nothing extra
	out.GetIt = nil
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Heretics of Dune", "authors": "Frank Herbert", "status": "read"}, &out)
	if out.GetIt != nil {
		t.Fatalf("read: %v", out.GetIt)
	}
	h.json(h.bobTok, "PUT", "/requests/settings", map[string]any{"want_get": "ebook"}, nil)
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Solaris", "authors": "Stanisław Lem", "status": "want"}, &out)
	if out.GetIt != nil {
		t.Fatalf("bob: %v", out.GetIt)
	}
	if code := h.json(h.anaTok, "PUT", "/requests/settings", map[string]any{"want_get": "film"}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad mode %d", code)
	}
}
