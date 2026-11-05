package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

type linkView struct {
	Linked   bool `json:"linked"`
	Unlinked bool `json:"unlinked"`
	Book     struct {
		ID     int64   `json:"id"`
		Title  string  `json:"title"`
		Status string  `json:"status"`
		Rating float64 `json:"rating"`
		URL    string  `json:"url"`
	} `json:"book"`
}

// Pocket Library links its books to the shelves: setting a status or a rating
// there adds or updates the book here (one read, however often it's said,
// and the reading sync doesn't add a second), a book can be linked and
// unlinked by hand, and an unlinked book is never matched or added again.
func TestLibraryLinks(t *testing.T) {
	h := newShelfHarness(t)
	today := time.Now().Format("2006-01-02")
	feedReads := []map[string]any{}
	lib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/reading/feed":
			_ = json.NewEncoder(w).Encode(map[string]any{"now": time.Now().UTC().Format(time.RFC3339Nano), "books": []any{}, "reads": feedReads})
		case "/api/v1/annotations":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/books":
			_, _ = w.Write([]byte(`{"items":[{"id":7,"title":"Solaris","authors":[{"id":1,"name":"Stanisław Lem"}],"cover":"/api/v1/books/7/cover?v=a"}],"total":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lib.Close)
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_lib_bob", "url": lib.URL, "public_url": "https://library.example.com"})
	}
	wr, ro := "pcx_bob_libwrite", "pcx_bob_libread"

	var v linkView
	if code := h.json(ro, "GET", "/library/links/7", nil, &v); code != 200 || v.Linked || v.Unlinked {
		t.Fatalf("before: %d %+v", code, v)
	}
	// read-only links can look but not change anything
	for _, c := range []struct{ m, p string }{{"PUT", "/library/links/7"}, {"DELETE", "/library/links/7"}, {"POST", "/library/links/7/apply"}} {
		if code, _ := h.do(ro, c.m, c.p, map[string]any{"book_id": 1}); code != 403 {
			t.Errorf("books:read %s %s = %d, want 403", c.m, c.p, code)
		}
	}

	apply := map[string]any{"book": map[string]any{"title": "Solaris", "authors": []string{"Stanisław Lem"}, "pages": 200},
		"status": "read", "day": today, "rating": 4.5}
	for i := 0; i < 2; i++ {
		if code := h.json(wr, "POST", "/library/links/7/apply", apply, &v); code != 200 || !v.Linked || v.Book.Status != "read" || v.Book.Rating != 4.5 || v.Book.Title != "Solaris" {
			t.Fatalf("apply #%d: %d %+v", i+1, code, v)
		}
	}
	solaris := v.Book.ID
	if c, _ := h.do(wr, "POST", "/library/links/7/apply", map[string]any{"rating": 4.2}); c != 400 {
		t.Fatalf("rating 4.2 = %d", c)
	}
	readsOf := func(id int64) int {
		reads, err := h.st.ListReads(t.Context(), h.bob.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		return len(reads)
	}
	if n := readsOf(solaris); n != 1 {
		t.Fatalf("reads after applying twice: %d", n)
	}
	// the feed later reports the same finish: no second read
	feedReads = []map[string]any{{"id": 1, "book_id": 7, "status": "read", "day": today, "title": "Solaris", "authors": []string{"Stanisław Lem"}}}
	var st libState
	h.json(h.bobTok, "POST", "/library/sync", nil, &st)
	if st.Error != "" || st.Added != 0 || st.Finished != 0 {
		t.Fatalf("sync after apply: %+v", st)
	}
	if n := readsOf(solaris); n != 1 {
		t.Fatalf("reads after the sync: %d", n)
	}

	// the book knows its Library twin
	var got struct{ Book store.Book }
	h.json(h.bobTok, "GET", fmt.Sprintf("/books/%d", solaris), nil, &got)
	if got.Book.LibraryBookID != 7 || got.Book.LibraryURL != "https://library.example.com/book/7" || got.Book.Format != "ebook" {
		t.Fatalf("book: %+v", got.Book)
	}

	// linking by hand: only to one's own books
	anaBook, _, _, _ := h.anaShelf()
	if code, _ := h.do(wr, "PUT", "/library/links/8", map[string]any{"book_id": anaBook}); code != 404 {
		t.Fatalf("linking to Ana's book = %d", code)
	}
	var dune struct{ Book store.Book }
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "want"}, &dune)
	if code := h.json(wr, "PUT", "/library/links/8", map[string]any{"book_id": dune.Book.ID}, &v); code != 200 || !v.Linked || v.Book.ID != dune.Book.ID {
		t.Fatalf("link by hand: %d %+v", code, v)
	}
	var shelf []store.Book
	h.json(h.bobTok, "GET", "/books", nil, &shelf)
	links := map[string]int64{}
	for _, b := range shelf {
		links[b.Title] = b.LibraryBookID
	}
	if links["Dune"] != 8 || links["Solaris"] != 7 {
		t.Fatalf("links in the list: %v", links)
	}

	// search in Library, for the picker
	var found []map[string]any
	if code := h.json(h.bobTok, "GET", "/library/search?q=solaris", nil, &found); code != 200 || len(found) != 1 ||
		found[0]["url"] != "https://library.example.com/book/7" || found[0]["cover_url"] != "https://library.example.com/api/v1/books/7/cover?v=a" || found[0]["authors"] != "Stanisław Lem" {
		t.Fatalf("search: %d %v", code, found)
	}
	if code, _ := h.do(wr, "GET", "/library/search?q=solaris", nil); code != 403 {
		t.Fatalf("search through a link = %d, want 403", code)
	}

	// unlinking leaves a tombstone: not linked, not updated, never added again
	if code := h.json(wr, "DELETE", "/library/links/7", nil, &v); code != 200 || v.Linked || !v.Unlinked {
		t.Fatalf("unlink: %d %+v", code, v)
	}
	h.json(ro, "GET", "/library/links/7", nil, &v)
	if v.Linked || !v.Unlinked {
		t.Fatalf("after unlink: %+v", v)
	}
	if code, _ := h.do(wr, "POST", "/library/links/7/apply", apply); code != 409 {
		t.Fatalf("apply after unlink = %d, want 409", code)
	}
	h.json(h.bobTok, "GET", fmt.Sprintf("/books/%d", solaris), nil, &got)
	if got.Book.LibraryBookID != 0 {
		t.Fatalf("still linked: %+v", got.Book)
	}
	feedReads = []map[string]any{{"id": 2, "book_id": 7, "status": "read", "day": "2030-01-02", "title": "Solaris", "authors": []string{"Stanisław Lem"}}}
	h.json(h.bobTok, "POST", "/library/sync", nil, &st)
	h.json(h.bobTok, "GET", "/books", nil, &shelf)
	if st.Added != 0 || st.Finished != 0 || len(shelf) != 2 || readsOf(solaris) != 1 {
		t.Fatalf("sync after unlink: %+v, %d books", st, len(shelf))
	}
	// linking again clears the tombstone
	if code := h.json(wr, "PUT", "/library/links/7", map[string]any{"book_id": solaris}, &v); code != 200 || !v.Linked {
		t.Fatalf("relink: %d %+v", code, v)
	}
}

func TestLibraryLinkRoutes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	cases := []struct {
		scopes []string
		r      *http.Request
		want   bool
	}{
		{[]string{"books:read"}, req("GET", "/api/v1/library/links/7"), true},
		{[]string{"books:read"}, req("PUT", "/api/v1/library/links/7"), false},
		{[]string{"books:write"}, req("PUT", "/api/v1/library/links/7"), true},
		{[]string{"books:write"}, req("DELETE", "/api/v1/library/links/7"), true},
		{[]string{"books:write"}, req("POST", "/api/v1/library/links/7/apply"), true},
		{[]string{"books:read"}, req("POST", "/api/v1/library/links/7/apply"), false},
		{[]string{"books:write"}, req("GET", "/api/v1/library/links/x"), false},
		{[]string{"books:write", "books:read"}, req("GET", "/api/v1/library/search"), false},
		{[]string{"books:write"}, req("PUT", "/api/v1/books/7"), false},
	}
	for _, c := range cases {
		if got := linkAllows(c.scopes, c.r); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}
