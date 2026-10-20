package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// A linked app (Pocket Day) acting for Bob through Pocket Core reads only
// Bob's books, only with books:read, and nothing else in the app.
func TestCoreLinkScopes(t *testing.T) {
	h := newShelfHarness(t)
	anaBook, _, _, _ := h.anaShelf()
	var created struct{ Book store.Book }
	if code := h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Bob's Novel", "authors": "Bob Author",
		"status": "read", "finished_on": "2026-10-01", "pages": 300}, &created); code != 201 {
		t.Fatalf("create Bob's book: %d", code)
	}
	bobBook := created.Book.ID
	link, other, anaLink := "pcx_bob_books", "pcx_bob_other", "pcx_ana_books"

	// books:read — the list (with the query Day sends), one book; only Bob's
	for _, path := range []string{"/books", "/books?year=2026&limit=500", "/books?status=read&limit=100", fmt.Sprintf("/books/%d", bobBook)} {
		code, body := h.do(link, "GET", path, nil)
		if code != 200 || !strings.Contains(string(body), "Bob's Novel") || strings.Contains(string(body), "Secret Diary") {
			t.Errorf("linked GET %s = %d %s", path, code, body)
		}
	}
	if code, _ := h.do(link, "GET", fmt.Sprintf("/books/%d", anaBook), nil); code != 404 {
		t.Errorf("Ana's book through Bob's link = %d, want 404", code)
	}
	// auth/me answers any linked app, as the person it acts for
	for _, tok := range []string{link, other} {
		var u store.User
		if code := h.json(tok, "GET", "/auth/me", nil, &u); code != 200 || u.Email != "bob@example.com" {
			t.Errorf("%s auth/me = %d %+v", tok, code, u)
		}
	}

	// a token without books:read
	for _, path := range []string{"/books", fmt.Sprintf("/books/%d", bobBook)} {
		if code, _ := h.do(other, "GET", path, nil); code != 403 {
			t.Errorf("link without books:read: GET %s = %d, want 403", path, code)
		}
	}

	// everything else is closed, even when the link acts for the admin
	for _, tok := range []string{link, anaLink} {
		for _, c := range []struct{ m, p string }{
			{"POST", "/books"}, {"PUT", fmt.Sprintf("/books/%d", bobBook)}, {"DELETE", fmt.Sprintf("/books/%d", bobBook)},
			{"POST", fmt.Sprintf("/books/%d/status", bobBook)}, {"POST", fmt.Sprintf("/books/%d/progress", bobBook)},
			{"GET", fmt.Sprintf("/books/%d/cover", bobBook)}, {"GET", fmt.Sprintf("/books/%d/characters", bobBook)},
			{"POST", fmt.Sprintf("/books/%d/reads", bobBook)}, {"PUT", "/auth/me"}, {"POST", "/auth/logout"},
			{"GET", "/dashboard"}, {"GET", "/stats"}, {"GET", "/search?q=x"}, {"GET", "/quotes"}, {"GET", "/daily"},
			{"GET", "/authors"}, {"GET", "/series"}, {"GET", "/tags"},
			{"GET", "/users"}, {"POST", "/users"}, {"GET", "/settings"}, {"PUT", "/settings"}, {"GET", "/settings/system"},
			{"GET", "/settings/ai"}, {"PUT", "/settings/ai"}, {"GET", "/tokens"}, {"POST", "/tokens"},
			{"GET", "/backup"}, {"GET", "/export.json"}, {"GET", "/export.csv"}, {"POST", "/import"},
			{"POST", "/import/goodreads"}, {"POST", "/characters/run"}, {"POST", "/covers/fetch-missing"},
		} {
			if code, body := h.do(tok, c.m, c.p, map[string]any{}); code != 403 || !strings.Contains(string(body), "link does not cover") {
				t.Errorf("%s %s %s = %d %s, want 403", tok, c.m, c.p, code, body)
			}
		}
	}
	// a link token Core does not vouch for is just unauthorized
	if code, _ := h.do("pcx_revoked", "GET", "/books", nil); code != 401 {
		t.Errorf("unknown link token = %d, want 401", code)
	}
	// nothing changed through the links
	if b, err := h.st.GetBook(t.Context(), h.bob.ID, bobBook); err != nil || b.Title != "Bob's Novel" {
		t.Errorf("Bob's book: %v %+v", err, b)
	}
}
