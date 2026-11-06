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

// "Ask about this book": the book page starts Pocket AI chats about a book
// (through Core, as the person) with the book and the reader's place in it as
// context, and lists the book's chats.
func TestAskAboutABook(t *testing.T) {
	h := newShelfHarness(t)
	h.cfg.BaseURL = "https://bookshelf.example.com"
	var mu sync.Mutex
	var started []map[string]any
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pcx_ai_ana" {
			http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/link/chats":
			out := []map[string]any{}
			for i, in := range started {
				if in["url"] == r.URL.Query().Get("url") {
					out = append(out, map[string]any{"id": "c" + strconv.Itoa(i), "title": in["title"], "status": "running", "url": "https://ai.example.com/t/c" + strconv.Itoa(i)})
				}
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == "POST" && r.URL.Path == "/api/v1/link/chats":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			started = append(started, in)
			id := "c" + strconv.Itoa(len(started)-1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "title": in["title"], "status": "running", "url": "https://ai.example.com/t/" + id})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ai.Close)
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			User   string   `json:"user"`
			App    string   `json:"app"`
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.App != "ai" || strings.Join(in.Scopes, " ") != "chats:write" || in.User != "ana@example.com" {
			http.Error(w, `{"error":"switched off","reason":"off_person"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_ai_ana", "url": ai.URL, "public_url": "https://ai.example.com"})
	}

	var created struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert", "pages": 600, "status": "reading",
		"review": "Paul is annoying so far.", "description": "Desert planet."}, &created)
	b := created.Book
	h.json(h.anaTok, "POST", "/books/"+strconv.FormatInt(b.ID, 10)+"/progress", map[string]any{"page": 150}, nil)
	h.json(h.anaTok, "POST", "/books/"+strconv.FormatInt(b.ID, 10)+"/quotes", map[string]any{"text": "Fear is the mind-killer.", "page": "8"}, nil)
	path := "/books/" + strconv.FormatInt(b.ID, 10) + "/chats"

	var box chatsBox
	if code := h.json(h.anaTok, "GET", path, nil, &box); code != 200 || !box.Available || len(box.Chats) != 0 || box.Model != "sonnet" {
		t.Fatalf("empty box: %d %+v", code, box)
	}
	// the starters suit a book being read, with the page filled in
	labels := []string{}
	for _, s := range box.Starters {
		labels = append(labels, s.Label)
		if s.Label == "Recap so far" && !strings.Contains(s.Text, "page 150") {
			t.Fatalf("recap not filled in: %q", s.Text)
		}
	}
	if j := strings.Join(labels, ","); !strings.Contains(j, "Recap so far") || strings.Contains(j, "Discuss the ending") || strings.Contains(j, "Is it for me?") {
		t.Fatalf("starters for a book being read: %s", j)
	}

	var chat bookChat
	if code := h.json(h.anaTok, "POST", path, map[string]any{"text": "Who is Stilgar? I'm on page {page}."}, &chat); code != 201 || chat.ID != "c0" {
		t.Fatalf("start: %d %+v", code, chat)
	}
	in := started[0]
	ctx, _ := in["context"].(string)
	if in["text"] != "Who is Stilgar? I'm on page 150." || in["title"] != "Dune" || in["model"] != "sonnet" ||
		in["url"] != "https://bookshelf.example.com/books/"+strconv.FormatInt(b.ID, 10) {
		t.Fatalf("sent to Pocket AI: %+v", in)
	}
	for _, want := range []string{"page 150 of 600 (25%)", "Do not reveal", "Paul is annoying", "Fear is the mind-killer.” (p. 8)", "Desert planet."} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("context misses %q:\n%s", want, ctx)
		}
	}
	if code := h.json(h.anaTok, "GET", path, nil, &box); code != 200 || len(box.Chats) != 1 || box.Chats[0].Status != "running" {
		t.Fatalf("listed: %d %+v", code, box)
	}
	if code := h.json(h.anaTok, "POST", path, map[string]any{"text": "  "}, nil); code != 400 {
		t.Fatalf("empty question: %d", code)
	}

	// Bob has no link to Pocket AI: the page says why, and starting fails with it
	var bobB struct{ Book store.Book }
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Solaris", "authors": "Stanisław Lem", "status": "read"}, &bobB)
	bobPath := "/books/" + strconv.FormatInt(bobB.Book.ID, 10) + "/chats"
	if code := h.json(h.bobTok, "GET", bobPath, nil, &box); code != 200 || box.Available || box.Missing || !strings.Contains(box.Reason, "Apps acting for me") {
		t.Fatalf("bob's box: %d %+v", code, box)
	}
	if code := h.json(h.bobTok, "POST", bobPath, map[string]any{"text": "Hi"}, nil); code != 409 {
		t.Fatalf("bob starts: %d", code)
	}
	// nor can he ask about Ana's book
	if code := h.json(h.bobTok, "GET", path, nil, nil); code != 404 {
		t.Fatalf("bob on ana's book: %d", code)
	}
}

func TestStartersFollowTheBook(t *testing.T) {
	b := &store.Book{Title: "Dune", Authors: "Frank Herbert", Status: "want", Pages: 600}
	got := startersFor(b, store.DefaultAskStarters)
	j := ""
	for _, s := range got {
		j += s.Label + "|" + s.Text + "\n"
	}
	if !strings.Contains(j, "Should I read Dune next?") || !strings.Contains(j, "Tell me about Frank Herbert") || strings.Contains(j, "Recap so far") {
		t.Fatalf("want-to-read starters:\n%s", j)
	}
	if s := fillStarter("on {page} of {pages} ({percent})", &store.Book{Pages: 200, CurrentPage: 50}); s != "on 50 of 200 (25%)" {
		t.Fatalf("filled: %q", s)
	}
	if s := fillStarter("up to page {page}", &store.Book{}); s != "up to page where I am" {
		t.Fatalf("no page yet: %q", s)
	}
}
