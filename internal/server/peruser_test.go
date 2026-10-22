package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/config"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// shelfHarness runs the app with two people on it: Ana (the owner, admin)
// and Bob (a member). Each has an API token; Bob also has a Core device token
// that a fake Pocket Core vouches for.
type shelfHarness struct {
	t        *testing.T
	ts       *httptest.Server
	st       *store.Store
	s        *Server
	cfg      *config.Config
	ana, bob *store.User
	anaTok   string
	bobTok   string
	// linkToken, when set, answers the fake Core's POST /api/v1/links/token.
	linkToken http.HandlerFunc
}

func newShelfHarness(t *testing.T) *shelfHarness {
	t.Helper()
	h := &shelfHarness{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/links/status" && r.URL.Query().Get("app") == "ai" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
			return
		}

		if r.URL.Path == "/api/v1/links/token" && h.linkToken != nil {
			h.linkToken(w, r)
			return
		}
		if r.URL.Path != "/api/v1/auth/introspect" {
			http.NotFound(w, r)
			return
		}
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := map[string]any{"active": false}
		switch in.Token {
		case "pcd_bobphone":
			out = map[string]any{"active": true, "kind": "device", "user": map[string]any{"id": 7, "email": "bob@example.com", "name": "Bob", "role": "member"}}
		case "pcd_carolphone": // a Core admin who never used Bookshelf before
			out = map[string]any{"active": true, "kind": "device", "user": map[string]any{"id": 9, "email": "carol@example.com", "name": "Carol", "role": "admin"}}
		case "pcx_bob_books", "pcx_bob_other", "pcx_ana_books": // a linked app (Pocket Day) acting for Bob or Ana
			scope, user := "books:read", map[string]any{"id": 7, "email": "bob@example.com", "name": "Bob", "role": "member"}
			if in.Token == "pcx_bob_other" {
				scope = "something:else"
			}
			if in.Token == "pcx_ana_books" {
				user = map[string]any{"id": 1, "email": "ana@example.com", "name": "Ana", "role": "admin"}
			}
			out = map[string]any{"active": true, "kind": "delegated", "scope": scope, "audience": "bookshelf", "user": user,
				"app": map[string]any{"id": 4, "slug": "day", "name": "Pocket Day"}}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(core.Close)
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db", CoverDir: dir + "/covers",
		CoreURL: core.URL, MaxCoverBytes: 1 << 20, GoogleBooksKey: "env-google-key"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	ana := &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	bob := &store.User{Email: "bob@example.com", Name: "Bob", Role: "member"}
	for _, u := range []*store.User{ana, bob} {
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	anaTok, _, _ := st.CreateAPIToken(ctx, ana.ID, "ana")
	bobTok, _, _ := st.CreateAPIToken(ctx, bob.ID, "bob")
	s := New(cfg, st, log, nil)
	if err := s.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	*h = shelfHarness{t: t, ts: ts, st: st, s: s, cfg: cfg, ana: ana, bob: bob, anaTok: anaTok, bobTok: bobTok}
	return h
}

// do sends a request with a bearer token and returns status and body.
func (h *shelfHarness) do(tok, method, path string, body any) (int, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.ts.URL+"/api/v1"+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (h *shelfHarness) json(tok, method, path string, body any, v any) int {
	h.t.Helper()
	code, out := h.do(tok, method, path, body)
	if v != nil && len(out) > 0 {
		if err := json.Unmarshal(out, v); err != nil {
			h.t.Fatalf("%s %s: %v (%s)", method, path, err, out)
		}
	}
	return code
}

// anaShelf puts a book with everything hanging off it on Ana's shelf.
func (h *shelfHarness) anaShelf() (bookID, readID, quoteID, authorID int64) {
	h.t.Helper()
	ctx := context.Background()
	var created struct{ Book store.Book }
	if code := h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Secret Diary", "authors": "Ana Author", "tags": "private",
		"status": "read", "rating": 4.5, "review": "only mine", "finished_on": "2026-03-01", "pages": 200}, &created); code != 201 {
		h.t.Fatalf("create book: %d", code)
	}
	bookID = created.Book.ID
	reads, _ := h.st.ListReads(ctx, h.ana.ID, bookID)
	if len(reads) != 1 {
		h.t.Fatalf("reads: %+v", reads)
	}
	readID = reads[0].ID
	var q store.Quote
	if code := h.json(h.anaTok, "POST", fmt.Sprintf("/books/%d/quotes", bookID), map[string]any{"text": "a line long enough to be a daily highlight"}, &q); code != 201 {
		h.t.Fatalf("create quote: %d", code)
	}
	a, err := h.st.FindAuthorByName(ctx, h.ana.ID, "Ana Author")
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.st.PutCharacters(ctx, h.ana.ID, bookID, "en", store.CharacterSet{Status: "ok", Characters: []store.Character{{Name: "Hero", Role: "main"}}}); err != nil {
		h.t.Fatal(err)
	}
	return bookID, readID, q.ID, a.ID
}

func TestMembersSeeOnlyTheirOwnShelf(t *testing.T) {
	h := newShelfHarness(t)
	bookID, readID, quoteID, authorID := h.anaShelf()
	ctx := context.Background()

	// Settings Ana chose: goal, languages, Google key, OpenAI key, daily delivery.
	if code := h.json(h.anaTok, "PUT", "/settings", map[string]any{"title": "Ana's Books", "yearly_goal": 30, "languages": "sr,en", "google_books_key": "ana-google", "landing": "planner"}, nil); code != 200 {
		t.Fatalf("ana settings: %d", code)
	}
	if code := h.json(h.anaTok, "PUT", "/settings/ai", map[string]any{"key": "sk-ana-secret-key-123456", "model": "gpt-x"}, nil); code != 200 {
		t.Fatalf("ana ai: %d", code)
	}
	if code := h.json(h.anaTok, "PUT", "/daily/settings", map[string]any{"enabled": true, "count": 3, "time": "07:00", "channel": "ntfy", "url": "https://ntfy.example/ana"}, nil); code != 200 {
		t.Fatalf("ana daily: %d", code)
	}

	for _, tok := range []string{h.bobTok, "pcd_bobphone"} { // API token and Core device token
		name := map[string]string{h.bobTok: "api token", "pcd_bobphone": "core device token"}[tok]
		t.Run(name, func(t *testing.T) {
			var me store.User
			if code := h.json(tok, "GET", "/auth/me", nil, &me); code != 200 || me.ID != h.bob.ID {
				t.Fatalf("resolved to %+v (%d), want Bob", me, code)
			}
			// list / search
			var books []store.Book
			for _, q := range []string{"", "?q=Secret", "?tag=private", "?year=2026", "?status=read", "?author=Ana"} {
				if h.json(tok, "GET", "/books"+q, nil, &books); len(books) != 0 {
					t.Errorf("GET /books%s showed Ana's books: %+v", q, books)
				}
			}
			var quotes []store.Quote
			for _, q := range []string{"", "?q=highlight", fmt.Sprintf("?book=%d", bookID), "?random=1"} {
				if h.json(tok, "GET", "/quotes"+q, nil, &quotes); len(quotes) != 0 {
					t.Errorf("GET /quotes%s showed Ana's quotes", q)
				}
			}
			var authors []store.Author
			if h.json(tok, "GET", "/authors", nil, &authors); len(authors) != 0 {
				t.Errorf("authors leaked: %+v", authors)
			}
			for _, p := range []string{"/tags", "/series", "/series/next"} {
				if code, out := h.do(tok, "GET", p, nil); code != 200 || strings.Contains(string(out), "private") || strings.Contains(string(out), "Secret") {
					t.Errorf("GET %s: %d %s", p, code, out)
				}
			}
			// read / change / delete someone else's records: always 404
			for _, c := range []struct{ method, path string }{
				{"GET", fmt.Sprintf("/books/%d", bookID)},
				{"PUT", fmt.Sprintf("/books/%d", bookID)},
				{"DELETE", fmt.Sprintf("/books/%d", bookID)},
				{"POST", fmt.Sprintf("/books/%d/status", bookID)},
				{"POST", fmt.Sprintf("/books/%d/progress", bookID)},
				{"GET", fmt.Sprintf("/books/%d/cover", bookID)},
				{"GET", fmt.Sprintf("/books/%d/covers", bookID)},
				{"DELETE", fmt.Sprintf("/books/%d/cover", bookID)},
				{"GET", fmt.Sprintf("/books/%d/characters", bookID)},
				{"PUT", fmt.Sprintf("/books/%d/characters", bookID)},
				{"DELETE", fmt.Sprintf("/books/%d/characters", bookID)},
				{"POST", fmt.Sprintf("/books/%d/reads", bookID)},
				{"POST", fmt.Sprintf("/books/%d/quotes", bookID)},
				{"PUT", fmt.Sprintf("/reads/%d", readID)},
				{"DELETE", fmt.Sprintf("/reads/%d", readID)},
				{"PUT", fmt.Sprintf("/quotes/%d", quoteID)},
				{"DELETE", fmt.Sprintf("/quotes/%d", quoteID)},
				{"POST", fmt.Sprintf("/quotes/%d/flags", quoteID)},
				{"GET", fmt.Sprintf("/authors/%d", authorID)},
				{"PUT", fmt.Sprintf("/authors/%d", authorID)},
				{"DELETE", fmt.Sprintf("/authors/%d", authorID)},
				{"GET", fmt.Sprintf("/authors/%d/works", authorID)},
				{"POST", fmt.Sprintf("/authors/%d/refresh", authorID)},
				{"GET", fmt.Sprintf("/authors/%d/photo", authorID)},
				{"GET", fmt.Sprintf("/authors/%d/photos", authorID)},
				{"DELETE", fmt.Sprintf("/authors/%d/photo", authorID)},
				{"GET", "/authors/resolve?name=Ana%20Author"},
			} {
				body := map[string]any{"status": "want", "percent": 10, "title": "Hijacked", "name": "Hijacked", "text": "hijacked quote", "favorite": true,
					"finished_on": "2026-01-01", "started_on": "2026-01-01", "sets": map[string]any{"en": map[string]any{"status": "none"}}}
				if code, out := h.do(tok, c.method, c.path, body); code != 404 {
					t.Errorf("%s %s: got %d (%s), want 404", c.method, c.path, code, out)
				}
			}
			// bulk paths that take ids from the body
			_ = h.json(tok, "PUT", "/planner/order", map[string]any{"ids": []int64{bookID}}, nil)
			_ = h.json(tok, "PUT", "/series/rename", map[string]any{"from": "", "to": "x"}, nil)
			// stats, dashboard, daily, exports
			var stats store.Stats
			if h.json(tok, "GET", "/stats?year=2026", nil, &stats); stats.Totals.Books != 0 || stats.AllTime.Books != 0 || stats.Quotes != 0 || stats.Goal != 0 {
				t.Errorf("stats leaked: %+v", stats)
			}
			var dash store.Dashboard
			if h.json(tok, "GET", "/dashboard", nil, &dash); dash.Books != 0 || dash.Quotes != 0 || dash.ReadTotal != 0 || dash.YearlyGoal != 0 {
				t.Errorf("dashboard leaked: %+v", dash)
			}
			var daily struct {
				Set store.DailySet
			}
			if h.json(tok, "GET", "/daily", nil, &daily); len(daily.Set.Quotes) != 0 {
				t.Errorf("daily leaked: %+v", daily.Set.Quotes)
			}
			var ds store.DailySettings
			if h.json(tok, "GET", "/daily/settings", nil, &ds); ds.Enabled || ds.URL != "" {
				t.Errorf("daily settings leaked: %+v", ds)
			}
			var ex Export
			if h.json(tok, "GET", "/export.json", nil, &ex); len(ex.Books) != 0 || len(ex.Authors) != 0 {
				t.Errorf("export leaked: %+v", ex)
			}
			if _, out := h.do(tok, "GET", "/export.csv", nil); strings.Contains(string(out), "Secret") || strings.Count(string(out), "\n") != 1 {
				t.Errorf("csv export leaked: %s", out)
			}
			var missing struct {
				Books []struct{ ID int64 }
			}
			if h.json(tok, "GET", "/characters/missing", nil, &missing); len(missing.Books) != 0 {
				t.Errorf("missing characters leaked: %+v", missing.Books)
			}
			// personal settings and keys
			var st map[string]any
			h.json(tok, "GET", "/settings", nil, &st)
			if st["yearly_goal"] != float64(0) || st["google_books_key"] != "" || st["languages"] != "en" || st["landing"] != "now" || st["google_books_env"] != false {
				t.Errorf("settings leaked: %v", st)
			}
			if st["title"] != "Ana's Books" {
				t.Errorf("box title not shared: %v", st["title"])
			}
			var ai map[string]any
			if h.json(tok, "GET", "/settings/ai", nil, &ai); ai["key_set"] != false || ai["model"] != "" {
				t.Errorf("ai settings leaked: %v", ai)
			}
			var sys map[string]any
			h.json(tok, "GET", "/settings/system", nil, &sys)
			if c := sys["counts"].(map[string]any); c["books"] != float64(0) || c["quotes"] != float64(0) || c["users"] != nil {
				t.Errorf("system counts leaked: %v", c)
			}
			var users []store.User
			if h.json(tok, "GET", "/users", nil, &users); len(users) != 1 || users[0].ID != h.bob.ID {
				t.Errorf("member listed other accounts: %+v", users)
			}
			if code, _ := h.do(tok, "GET", "/backup", nil); code != 403 {
				t.Errorf("member downloaded the whole database: %d", code)
			}
		})
	}

	// Bob's writes stay his: the title is not his to change, user_id in a body is ignored.
	if code := h.json(h.bobTok, "PUT", "/settings", map[string]any{"title": "Bob was here", "yearly_goal": 5}, nil); code != 200 {
		t.Fatalf("bob settings: %d", code)
	}
	var created struct{ Book store.Book }
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Bob's Book", "user_id": h.ana.ID, "authors": "Ana Author"}, &created)
	if created.Book.UserID != h.bob.ID {
		t.Errorf("book created for user %d, want Bob", created.Book.UserID)
	}
	var imp map[string]int
	h.json(h.bobTok, "POST", "/import", map[string]any{"books": []map[string]any{{"title": "Imported", "user_id": h.ana.ID}}}, &imp)
	if imp["created"] != 1 {
		t.Errorf("import: %v", imp)
	}

	// Ana's shelf is untouched, and her settings are hers.
	var b struct {
		Book   store.Book
		Quotes []store.Quote
		Reads  []store.Read
	}
	if code := h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d", bookID), nil, &b); code != 200 || b.Book.Title != "Secret Diary" || b.Book.Status != "read" ||
		len(b.Quotes) != 1 || b.Quotes[0].Text != "a line long enough to be a daily highlight" || b.Quotes[0].Favorite || len(b.Reads) != 1 || b.Reads[0].FinishedOn != "2026-03-01" {
		t.Errorf("Ana's book changed: %+v", b)
	}
	if b.Book.QueuePos != 0 {
		t.Errorf("Bob reordered Ana's plan: %d", b.Book.QueuePos)
	}
	var anaBooks []store.Book
	if h.json(h.anaTok, "GET", "/books", nil, &anaBooks); len(anaBooks) != 1 {
		t.Errorf("Ana sees %d books, want only her own", len(anaBooks))
	}
	var st map[string]any
	if h.json(h.anaTok, "GET", "/settings", nil, &st); st["yearly_goal"] != float64(30) || st["google_books_key"] != "ana-google" || st["title"] != "Ana's Books" {
		t.Errorf("Ana's settings: %v", st)
	}
	if cs, err := h.st.Characters(ctx, h.ana.ID, bookID); err != nil || cs["en"].Status != "ok" {
		t.Errorf("Ana's characters changed: %+v %v", cs, err)
	}
	if a, err := h.st.GetAuthor(ctx, h.ana.ID, authorID); err != nil || a.Name != "Ana Author" {
		t.Errorf("Ana's author changed: %+v %v", a, err)
	}
	// Bob's "Ana Author" is a separate author row on his shelf.
	if a, err := h.st.FindAuthorByName(ctx, h.bob.ID, "Ana Author"); err != nil || a.ID == authorID {
		t.Errorf("Bob's author: %+v %v", a, err)
	}
}

// An admin gets no special access to a member's shelf either.
func TestAdminsCannotReadMembersShelves(t *testing.T) {
	h := newShelfHarness(t)
	var created struct{ Book store.Book }
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Bob's Private Book", "authors": "Bob Author"}, &created)
	id := created.Book.ID
	for _, c := range []struct{ method, path string }{
		{"GET", fmt.Sprintf("/books/%d", id)},
		{"GET", fmt.Sprintf("/books/%d/characters", id)},
		{"PUT", fmt.Sprintf("/books/%d/characters", id)},
		{"DELETE", fmt.Sprintf("/books/%d/characters", id)},
		{"DELETE", fmt.Sprintf("/books/%d", id)},
	} {
		if code, out := h.do(h.anaTok, c.method, c.path, map[string]any{"sets": map[string]any{"en": map[string]any{"status": "none"}}}); code != 404 {
			t.Errorf("admin %s %s: %d %s", c.method, c.path, code, out)
		}
	}
	var missing struct {
		Books []struct{ ID int64 }
	}
	if h.json(h.anaTok, "GET", "/characters/missing", nil, &missing); len(missing.Books) != 0 {
		t.Errorf("admin's missing list holds a member's books: %+v", missing.Books)
	}
	var books []store.Book
	if h.json(h.anaTok, "GET", "/books?q=Private", nil, &books); len(books) != 0 {
		t.Errorf("admin search found a member's book")
	}
	// the admin can still take the database backup (operator of the box) — but
	// only an admin: covered in TestMembersSeeOnlyTheirOwnShelf.
}

// Keys given to the box through the environment are the owner's alone.
func TestEnvironmentKeysBelongToTheOwner(t *testing.T) {
	h := newShelfHarness(t)
	var ana, bob map[string]any
	h.json(h.anaTok, "GET", "/settings", nil, &ana)
	h.json(h.bobTok, "GET", "/settings", nil, &bob)
	if ana["google_books_env"] != true || !strings.Contains(fmt.Sprint(ana["sources"]), "google") {
		t.Errorf("owner lost the environment Google key: %v", ana)
	}
	if bob["google_books_env"] != false || strings.Contains(fmt.Sprint(bob["sources"]), "google") {
		t.Errorf("member got the owner's environment Google key: %v", bob)
	}
}

// The character writer runs per person: each person's books with their own
// OpenAI key, never someone else's.
func TestCharacterWriterUsesEachPersonsOwnKey(t *testing.T) {
	h := newShelfHarness(t)
	type call struct{ key, input string }
	var mu sync.Mutex
	var calls []call
	oa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, call{strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), string(raw)})
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message",
			"content": []any{map[string]any{"type": "output_text", "text": `{"kind":"nonfiction","characters":{}}`}}}}})
	}))
	t.Cleanup(oa.Close)
	h.cfg.OpenAIBaseURL = oa.URL
	ctx := context.Background()
	var ab, bb struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Ana Reads This"}, &ab)
	h.json(h.bobTok, "POST", "/books", map[string]any{"title": "Bob Reads That"}, &bb)
	h.json(h.anaTok, "PUT", "/settings/ai", map[string]any{"key": "sk-ana-0000000000"}, nil)

	h.s.writeMissingCharacters(ctx)
	mu.Lock()
	for _, c := range calls {
		if c.key != "sk-ana-0000000000" || strings.Contains(c.input, "Bob Reads That") {
			t.Errorf("owner's key used for %q with key %q", c.input[:min(80, len(c.input))], c.key)
		}
	}
	if len(calls) == 0 {
		t.Fatal("no characters written for the owner")
	}
	calls = nil
	mu.Unlock()
	if cs, _ := h.st.Characters(ctx, h.ana.ID, ab.Book.ID); cs["en"].Status != "none" {
		t.Errorf("Ana's book: %+v", cs)
	}
	if cs, _ := h.st.Characters(ctx, h.bob.ID, bb.Book.ID); len(cs) != 0 {
		t.Errorf("Bob's book was written without his key: %+v", cs)
	}
	var st map[string]any
	if h.json(h.bobTok, "GET", "/characters/status", nil, &st); st["writer"] != "external" || st["waiting"] != float64(1) {
		t.Errorf("Bob's status: %v", st)
	}

	// Bob brings his own key: his book is written with it.
	h.json(h.bobTok, "PUT", "/settings/ai", map[string]any{"key": "sk-bob-1111111111"}, nil)
	h.s.writeMissingCharacters(ctx)
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0].key != "sk-bob-1111111111" || !strings.Contains(calls[0].input, "Bob Reads That") {
		t.Errorf("calls after Bob's key: %+v", calls)
	}
}

// Accounts created on first contact (Core tokens, OIDC) are members: only the
// first account on the box is an admin, whatever role Core gives the person.
func TestAutoCreatedAccountsAreMembers(t *testing.T) {
	h := newShelfHarness(t)
	var me store.User
	if code := h.json("pcd_carolphone", "GET", "/auth/me", nil, &me); code != 200 || me.Email != "carol@example.com" || me.Role != "member" {
		t.Fatalf("auto-created account: %d %+v", code, me)
	}
	var books []store.Book
	if h.json("pcd_carolphone", "GET", "/books", nil, &books); len(books) != 0 {
		t.Errorf("new account sees books: %+v", books)
	}
	if code, _ := h.do("pcd_carolphone", "GET", "/backup", nil); code != 403 {
		t.Errorf("auto-created account took the backup: %d", code)
	}
}
