package server

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
