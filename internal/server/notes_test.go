package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// A book's page lists the person's notes about it from Pocket Notes, asked as
// that person through Core, and offers a link that starts one.
func TestBookNotes(t *testing.T) {
	h := newShelfHarness(t)
	h.cfg.BaseURL = "https://bookshelf.example.com"
	var asked []string
	notes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mentions" || r.Header.Get("Authorization") != "Bearer pcx_for_ana" {
			http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
			return
		}
		asked = append(asked, r.URL.Query().Get("url"))
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 401, "title": "Dune, thoughts", "category": "Books", "excerpt": "Spice", "modified": 1792500000}})
	}))
	t.Cleanup(notes.Close)
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			User   string   `json:"user"`
			App    string   `json:"app"`
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.App != "notes" || len(in.Scopes) != 1 || in.Scopes[0] != "notes:read" {
			http.Error(w, `{"error":"not declared","reason":"not_declared"}`, http.StatusForbidden)
			return
		}
		if in.User != "ana@example.com" {
			http.Error(w, `{"error":"switched off","reason":"off_person"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_for_ana", "url": notes.URL, "public_url": "https://notes.example.com"})
	}
	var made struct{ Book struct{ ID int64 } }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert"}, &made)
	id := made.Book.ID

	var box notesBox
	h.json(h.anaTok, "GET", "/books/"+strconv.FormatInt(id, 10)+"/notes", nil, &box)
	item := "https://bookshelf.example.com/books/" + strconv.FormatInt(id, 10)
	if !box.Available || len(box.Notes) != 1 || box.Notes[0].URL != "https://notes.example.com/note/401" || len(asked) != 1 || asked[0] != item {
		t.Fatalf("box %+v asked %v", box, asked)
	}
	nu, _ := url.Parse(box.NewURL)
	if nu.Host != "notes.example.com" || nu.Path != "/new" || nu.Query().Get("url") != item || nu.Query().Get("title") != "Dune" || !strings.Contains(nu.Query().Get("label"), "Frank Herbert") {
		t.Fatalf("new url %s", box.NewURL)
	}

	// Bob cannot ask about Ana's book, and his own link being off is said, not an error.
	if c, _ := h.do(h.bobTok, "GET", "/books/"+strconv.FormatInt(id, 10)+"/notes", nil); c != http.StatusNotFound {
		t.Fatalf("bob on ana's book: %d", c)
	}
	var bobs struct{ Book struct{ ID int64 } }
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Emma"}, &bobs)
	h.json(h.bobTok, "GET", "/books/"+strconv.FormatInt(bobs.Book.ID, 10)+"/notes", nil, &box)
	if box.Available || box.Missing || !strings.Contains(box.Reason, "Apps acting for me") || len(box.Notes) != 0 {
		t.Fatalf("bob's box: %+v", box)
	}

	// No Pocket Notes on the box: nothing to offer at all.
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"no such app","reason":"target_missing"}`, http.StatusNotFound)
	}
	box = notesBox{}
	h.json(h.anaTok, "GET", "/books/"+strconv.FormatInt(id, 10)+"/notes", nil, &box)
	if box.Available || !box.Missing {
		t.Fatalf("no notes app: %+v", box)
	}
}
