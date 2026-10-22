package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// kosync sends a KOSync request the way KOReader does.
func kosync(t *testing.T, base, method, path, user, key string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	req.Header.Set("Accept", "application/vnd.koreader.v1+json")
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("X-Auth-User", user)
		req.Header.Set("X-Auth-Key", key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestKosyncProtocol(t *testing.T) {
	h := newShelfHarness(t)
	var created struct{ Book store.Book }
	if code := h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Foundryside", "authors": "Robert Jackson Bennett", "status": "want", "pages": 500}, &created); code != 201 {
		t.Fatalf("create book: %d", code)
	}
	book := created.Book.ID
	var other struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "reading", "pages": 400}, &other)

	if code := h.json(h.anaTok, "PUT", "/kosync/account", map[string]any{"username": "ana", "password": "short"}, nil); code != 400 {
		t.Fatalf("short password: %d", code)
	}
	if code := h.json(h.anaTok, "PUT", "/kosync/account", map[string]any{"username": "ana", "password": "reading-is-fun"}, nil); code != 200 {
		t.Fatalf("set account: %d", code)
	}
	if code := h.json(h.bobTok, "PUT", "/kosync/account", map[string]any{"username": "ANA", "password": "something-else"}, nil); code != 409 {
		t.Fatalf("taken username: %d", code)
	}
	key := md5hex("reading-is-fun")

	// Both the main host (/kosync) and the dedicated listener speak it.
	dedicated := httptest.NewServer(h.s.KosyncRouter())
	t.Cleanup(dedicated.Close)
	for _, base := range []string{h.ts.URL + "/kosync", dedicated.URL} {
		if code, _ := kosync(t, base, "GET", "/users/auth", "ana", key, nil); code != 200 {
			t.Fatalf("%s auth: %d", base, code)
		}
		if code, out := kosync(t, base, "GET", "/users/auth", "ana", md5hex("wrong"), nil); code != 401 || out["code"] != float64(2001) {
			t.Fatalf("%s wrong key: %d %v", base, code, out)
		}
		if code, _ := kosync(t, base, "POST", "/users/create", "", "", map[string]any{"username": "x", "password": key}); code != 403 {
			t.Fatalf("%s register: %d", base, code)
		}
	}
	base := dedicated.URL
	// The dedicated listener serves nothing else.
	if code, _ := kosync(t, base, "GET", "/api/v1/books", "", "", nil); code != 404 && code != 405 {
		t.Fatalf("api on kosync port: %d", code)
	}

	// Unknown document: empty object, like the reference server.
	if code, out := kosync(t, base, "GET", "/syncs/progress/0123456789abcdef0123456789abcdef", "ana", key, nil); code != 200 || len(out) != 0 {
		t.Fatalf("unknown doc: %d %v", code, out)
	}

	// A device that says what the book is: matched, page moved, book started.
	doc := "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	code, out := kosync(t, base, "PUT", "/syncs/progress", "ana", key, map[string]any{
		"document": doc, "progress": "/body/DocFragment[8]/body/p[4]/text().96", "percentage": 0.25,
		"device": "CrossPoint", "device_id": "dev1",
		"metadata": map[string]any{"title": "Foundryside", "authors": "Robert Jackson Bennett"},
	})
	if code != 200 || out["document"] != doc {
		t.Fatalf("put: %d %v", code, out)
	}
	var got struct{ Book store.Book }
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d", book), nil, &got)
	b := got.Book
	if b.CurrentPage != 125 || b.Status != store.StatusReading || b.ReaderPercent == nil || *b.ReaderPercent != 0.25 {
		t.Fatalf("book after sync: page %d status %s reader %v", b.CurrentPage, b.Status, b.ReaderPercent)
	}
	code, out = kosync(t, base, "GET", "/syncs/progress/"+doc, "ana", key, nil)
	if code != 200 || out["percentage"] != 0.25 || out["device"] != "CrossPoint" {
		t.Fatalf("get: %d %v", code, out)
	}

	// A mirror that sends only the hash: kept, unmatched, linked by hand.
	bare := "ffffffffffffffffffffffffffffffff"
	if code, _ := kosync(t, base, "PUT", "/syncs/progress", "ana", key, map[string]any{"document": bare, "progress": "0.5", "percentage": 0.5, "device": "CrossPoint Sync"}); code != 200 {
		t.Fatalf("bare put: %d", code)
	}
	var list struct {
		Account   *store.KosyncAccount   `json:"account"`
		Documents []store.KosyncDocument `json:"documents"`
	}
	h.json(h.anaTok, "GET", "/kosync", nil, &list)
	if list.Account == nil || list.Account.Username != "ana" || len(list.Documents) != 2 {
		t.Fatalf("list: %+v", list)
	}
	for _, d := range list.Documents {
		if d.Document == bare && d.BookID != nil {
			t.Fatalf("bare doc matched: %+v", d)
		}
	}
	if code := h.json(h.anaTok, "PUT", "/kosync/documents/"+bare, map[string]any{"book_id": other.Book.ID}, nil); code != 200 {
		t.Fatalf("link: %d", code)
	}
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d", other.Book.ID), nil, &got)
	if b = got.Book; b.CurrentPage != 200 {
		t.Fatalf("linked book page: %d", b.CurrentPage)
	}
	// Bob cannot link Ana's books, nor see her documents.
	if code := h.json(h.bobTok, "PUT", "/kosync/documents/"+bare, map[string]any{"book_id": other.Book.ID}, nil); code != 404 {
		t.Fatalf("bob link: %d", code)
	}
	h.json(h.bobTok, "GET", "/kosync", nil, &list)
	if len(list.Documents) != 0 || list.Account != nil {
		t.Fatalf("bob sees: %+v", list)
	}

	// A new password locks the old one out at once (no stale auth cache).
	h.json(h.anaTok, "PUT", "/kosync/account", map[string]any{"username": "ana", "password": "new-password-1"}, nil)
	if code, _ := kosync(t, base, "GET", "/users/auth", "ana", key, nil); code != 401 {
		t.Fatalf("old key after change: %d", code)
	}
	if code, _ := kosync(t, base, "GET", "/users/auth", "Ana", md5hex("new-password-1"), nil); code != 200 {
		t.Fatalf("new key: %d", code)
	}
}

func TestKosyncFilenameMatch(t *testing.T) {
	d := &store.KosyncDocument{Filename: "/mnt/books/Moć sadašnjeg trenutka - Eckhart Tolle.epub"}
	titles, authors := kosyncNames(d)
	if len(titles) != 2 || titles[0] != "moc sadasnjeg trenutka" || len(authors) != 2 {
		t.Fatalf("names: %v %v", titles, authors)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no network in tests")
}

// A CrossPoint Sync mirror sends bare hashes; Bookshelf reads titles from the
// CrossPoint Sync account, links the book it has and adds the one it lacks.
func TestKosyncCrossPointPull(t *testing.T) {
	h := newShelfHarness(t)
	h.s.meta.HTTP = &http.Client{Timeout: time.Second, Transport: failingTransport{}}
	var dune struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "want", "pages": 400}, &dune)

	known, missing := "11111111111111111111111111111111", "01a01cdb62f88f29ecbdbe3808822849"
	var calls int
	cps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-User") != "ana" || r.Header.Get("X-Auth-Key") != md5hex("cps-password") {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/users/auth":
			w.Write([]byte(`{"authorized":"OK"}`))
		case "/api/v1/progress":
			calls++
			fmt.Fprintf(w, `{"items":[
				{"document":%q,"title":"Dune","author":"Frank Herbert","filename":null,"percentage":0.5,"progress":"/body/p[1]","device":"CrossPoint","device_id":"x4","timestamp":%d,"status":"reading","cover_url":null,"page_count":412},
				{"document":%q,"title":"Nebeski oganj","author":"Robert Jordan","filename":"Nebeski oganj - Robert Jordan.epub","percentage":0.011707,"progress":"/body/DocFragment[7]","device":"CrossPoint","device_id":"x4","timestamp":%d,"status":"reading","cover_url":null,"page_count":900}
			]}`, known, time.Now().Unix()+60, missing, time.Now().Unix()-60)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cps.Close)

	if code, out := h.do(h.anaTok, "PUT", "/kosync/prefs", map[string]any{"remote_url": cps.URL, "remote_user": "ana", "remote_password": "wrong"}); code != 400 {
		t.Fatalf("wrong password accepted: %d %s", code, out)
	}
	var st struct {
		Prefs     store.KosyncPrefs      `json:"prefs"`
		Documents []store.KosyncDocument `json:"documents"`
	}
	if code := h.json(h.anaTok, "PUT", "/kosync/prefs", map[string]any{"remote_url": cps.URL, "remote_user": "ana", "remote_password": "cps-password"}, &st); code != 200 {
		t.Fatalf("connect: %d", code)
	}
	if calls != 1 || st.Prefs.RemoteUser != "ana" || st.Prefs.LastError != "" || len(st.Documents) != 2 {
		t.Fatalf("after connect: calls %d %+v", calls, st)
	}
	byDoc := map[string]store.KosyncDocument{}
	for _, d := range st.Documents {
		byDoc[d.Document] = d
	}
	if d := byDoc[known]; d.BookID == nil || *d.BookID != dune.Book.ID {
		t.Fatalf("dune not linked: %+v", d)
	}
	nd := byDoc[missing]
	if nd.BookID == nil || nd.Title != "Nebeski oganj" || nd.PageCount != 900 {
		t.Fatalf("missing book not added: %+v", nd)
	}
	var got struct{ Book store.Book }
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d", *nd.BookID), nil, &got)
	if got.Book.Title != "Nebeski oganj" || got.Book.Authors != "Robert Jordan" || got.Book.Status != store.StatusReading || got.Book.Pages != 900 || got.Book.CurrentPage != 11 {
		t.Fatalf("added book: %+v", got.Book)
	}
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d", dune.Book.ID), nil, &got)
	if got.Book.Status != store.StatusReading || got.Book.CurrentPage != 200 {
		t.Fatalf("dune: %+v", got.Book)
	}

	// Raw updates are kept as they arrived; a second pull with nothing new adds none.
	var ev []store.KosyncEvent
	h.json(h.anaTok, "GET", "/kosync/documents/"+missing+"/events", nil, &ev)
	if len(ev) != 1 || ev[0].Source != "crosspoint" || !strings.Contains(string(ev[0].Body), "Nebeski oganj") {
		t.Fatalf("events: %+v", ev)
	}
	if code := h.json(h.anaTok, "POST", "/kosync/pull", nil, nil); code != 200 {
		t.Fatalf("pull: %d", code)
	}
	h.json(h.anaTok, "GET", "/kosync/documents/"+missing+"/events", nil, &ev)
	if len(ev) != 1 {
		t.Fatalf("duplicate events: %d", len(ev))
	}
	books, _ := h.st.ListBooks(context.Background(), store.BookFilter{UserID: h.ana.ID})
	if len(books) != 2 {
		t.Fatalf("books after second pull: %d", len(books))
	}
}

func TestKosyncCandidateForTranslation(t *testing.T) {
	d := &store.KosyncDocument{Title: "Nebeski oganj", Authors: "Robert Jordan"}
	titles, authors := kosyncNames(d)
	cands := []metadata.Candidate{
		{Title: "Nebo", Authors: []string{"Someone Else"}},
		{Title: "The Fires of Heaven", Authors: []string{"Robert Jordan"}, Pages: 894, ISBN13: "9780312854270"},
	}
	c, exact := pickKosyncCandidate(cands, titles, authors)
	if c == nil || exact || c.Pages != 894 {
		t.Fatalf("translation: %+v exact=%v", c, exact)
	}
	cands = append(cands, metadata.Candidate{Title: "Nebeski oganj", Authors: []string{"Robert Jordan"}})
	if c, exact = pickKosyncCandidate(cands, titles, authors); c == nil || !exact || c.Title != "Nebeski oganj" {
		t.Fatalf("exact: %+v %v", c, exact)
	}
	if c, _ = pickKosyncCandidate([]metadata.Candidate{{Title: "Nebeski oganj", Authors: []string{"Other Author"}}}, titles, authors); c != nil {
		t.Fatalf("someone else's book picked: %+v", c)
	}
}
