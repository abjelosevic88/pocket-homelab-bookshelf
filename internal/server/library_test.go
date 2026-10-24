package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Ebooks read in Pocket Library become reads: a finished one matched by ISBN
// or added as an ebook with its cover, one being read goes on "reading" with
// its page, highlights become quotes. A second sync adds nothing.
func TestLibrarySync(t *testing.T) {
	h := newShelfHarness(t)
	if err := os.MkdirAll(h.cfg.CoverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lib := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pcx_lib_ana" {
			http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/reading/feed":
			if r.URL.Query().Get("since") == "" {
				http.Error(w, "since", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"now": now,
				"books": []map[string]any{
					{"book_id": 7, "title": "Na Drini ćuprija", "authors": []string{"Ivo Andrić"}, "pages": 300, "status": "reading", "percentage": 0.5, "started_at": today + "T08:00:00Z", "cover": "/api/v1/books/7/cover?v=a"},
				},
				"reads": []map[string]any{
					{"id": 1, "book_id": 9, "status": "read", "day": today, "started_at": today + "T07:00:00Z", "finished_at": today + "T20:00:00Z", "title": "Dune Messiah", "authors": []string{"Frank Herbert"}, "isbn": "9780593098233", "cover": "/api/v1/books/9/cover?v=b"},
					{"id": 2, "book_id": 11, "status": "read", "day": today, "finished_at": today + "T21:00:00Z", "title": "Solaris", "authors": []string{"Stanisław Lem"}, "cover": "/api/v1/books/11/cover?v=c"},
				}})
		case r.URL.Path == "/api/v1/annotations":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 50, "book_id": 11, "kind": "highlight", "text": "We don't want to conquer the cosmos.", "percentage": 0.42},
				{"id": 51, "book_id": 11, "kind": "bookmark", "text": ""},
				{"id": 52, "book_id": 99, "kind": "highlight", "text": "a book not on the shelves"},
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/books/") && strings.HasSuffix(r.URL.Path, "/cover"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nfake"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lib.Close)
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			User   string   `json:"user"`
			App    string   `json:"app"`
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.App != "library" || len(in.Scopes) != 1 || in.Scopes[0] != "reading:read" || in.User != "ana@example.com" {
			http.Error(w, `{"error":"switched off","reason":"off_person"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_lib_ana", "url": lib.URL, "public_url": "https://library.example.com"})
	}
	// Dune Messiah is already on Ana's shelves (by ISBN, another title spelling): matched, not added
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune: Messiah", "authors": "Frank Herbert", "isbn13": "9780593098233", "status": "want"}, nil)

	var st libState
	h.json(h.anaTok, "POST", "/library/sync", nil, &st)
	if st.Error != "" || st.Added != 2 || st.Finished != 2 || st.Started != 1 || st.Quotes != 1 {
		t.Fatalf("first sync: %+v", st)
	}
	var shelf []struct {
		Title       string `json:"title"`
		Status      string `json:"status"`
		Format      string `json:"format"`
		CurrentPage int    `json:"current_page"`
		QuoteCount  int    `json:"quote_count"`
		SourceURL   string `json:"source_url"`
	}
	h.json(h.anaTok, "GET", "/books", nil, &shelf)
	got := map[string]string{}
	for _, b := range shelf {
		got[b.Title] = b.Status + "|" + b.Format
		switch b.Title {
		case "Na Drini ćuprija":
			if b.CurrentPage != 150 || b.SourceURL != "https://library.example.com/book/7" {
				t.Fatalf("reading book: %+v", b)
			}
		case "Solaris":
			if b.QuoteCount != 1 {
				t.Fatalf("quotes on Solaris: %d", b.QuoteCount)
			}
		}
	}
	if len(shelf) != 3 || got["Dune: Messiah"] != "read|" || got["Solaris"] != "read|ebook" || got["Na Drini ćuprija"] != "reading|ebook" {
		t.Fatalf("shelves: %v", got)
	}

	h.json(h.anaTok, "POST", "/library/sync", nil, &st)
	if st.Added != 0 || st.Finished != 0 || st.Started != 0 || st.Quotes != 0 {
		t.Fatalf("second sync: %+v", st)
	}

	// Bob's link is off: said, not an error
	h.json(h.bobTok, "POST", "/library/sync", nil, &st)
	if !st.Off || !strings.Contains(st.Error, "Apps acting for me") {
		t.Fatalf("bob: %+v", st)
	}
}

// Each person picks how often Bookshelf looks at Pocket Library: ten minutes
// unless they choose 2, 5, 30 or 60.
func TestLibraryInterval(t *testing.T) {
	h := newShelfHarness(t)
	var st struct {
		Every   int   `json:"every"`
		Choices []int `json:"choices"`
		Enabled bool  `json:"enabled"`
	}
	h.json(h.anaTok, "GET", "/library", nil, &st)
	if st.Every != 10 || len(st.Choices) != 5 || !st.Enabled {
		t.Fatalf("default: %+v", st)
	}
	h.json(h.anaTok, "PUT", "/library", map[string]any{"every": 5}, &st)
	if st.Every != 5 || !st.Enabled {
		t.Fatalf("after choosing 5: %+v", st)
	}
	if c, _ := h.do(h.anaTok, "PUT", "/library", map[string]any{"every": 7}); c != http.StatusBadRequest {
		t.Fatalf("7 minutes = %d", c)
	}
	h.json(h.bobTok, "GET", "/library", nil, &st)
	if st.Every != 10 {
		t.Fatalf("bob got ana's interval: %+v", st)
	}
}
