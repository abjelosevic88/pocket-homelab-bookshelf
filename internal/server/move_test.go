package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// upload posts a file as multipart field "file".
func (h *shelfHarness) upload(tok, path string, body []byte) (int, []byte) {
	h.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "export.csv")
	_, _ = fw.Write(body)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", h.ts.URL+"/api/v1"+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (h *shelfHarness) count(tok string) int {
	h.t.Helper()
	var books []store.Book
	if code := h.json(tok, "GET", "/books", nil, &books); code != 200 {
		h.t.Fatalf("list books: %d", code)
	}
	return len(books)
}

func TestMoveLinksAndImportsTwice(t *testing.T) {
	h := newShelfHarness(t)
	h.anaShelf()
	for _, b := range []map[string]any{
		{"title": "Čarobnjak iz Oza", "authors": "L. Frank Baum", "status": "want"},
		{"title": "Na Drini ćuprija", "authors": "Ivo Andrić", "status": "read", "finished_on": "2025-06-01"},
		{"title": "Dune", "authors": "Frank Herbert", "isbn13": "9780441172719", "status": "reading"},
	} {
		if code, raw := h.do(h.anaTok, "POST", "/books", b); code != 201 {
			t.Fatalf("create %v: %d %s", b["title"], code, raw)
		}
	}
	bobOut, bobIn, anaIn := "pcx_move_bob_out", "pcx_move_bob_in", "pcx_move_ana_in"

	// move:export reads the exports and the description, nothing else
	var desc core.Portable
	if code := h.json(bobOut, "GET", strings.TrimPrefix(core.PortablePath, "/api/v1"), nil, &desc); code != 200 || len(desc.Exports) != 2 || len(desc.Imports) != 2 {
		t.Fatalf("portable %d %+v", code, desc)
	}
	for _, p := range []string{"/export.json", "/export.csv"} {
		if code, raw := h.do(bobOut, "GET", p, nil); code != 200 || strings.Contains(string(raw), "Secret Diary") {
			t.Fatalf("bob's %s through Move: %d %s", p, code, raw)
		}
	}
	for _, c := range []struct{ m, p string }{{"POST", "/import"}, {"POST", "/import/goodreads"}, {"GET", "/books"}, {"GET", "/settings"}, {"GET", "/backup"}, {"POST", "/import/kindle"}} {
		if code, _ := h.do(bobOut, c.m, c.p, map[string]any{}); code != 403 {
			t.Errorf("move:export %s %s = %d, want 403", c.m, c.p, code)
		}
	}
	// move:import posts the imports, nothing else
	for _, c := range []struct{ m, p string }{{"GET", "/export.json"}, {"GET", "/export.csv"}, {"POST", "/books"}, {"PUT", "/settings"}, {"GET", "/books"}, {"POST", "/import/kindle"}} {
		if code, _ := h.do(bobIn, c.m, c.p, map[string]any{}); code != 403 {
			t.Errorf("move:import %s %s = %d, want 403", c.m, c.p, code)
		}
	}
	if code, _ := h.do(bobIn, "GET", strings.TrimPrefix(core.PortablePath, "/api/v1"), nil); code != 200 {
		t.Errorf("move:import reads the description: %d", code)
	}

	// Ana's own export into Bob's shelf, twice: the second run adds nothing
	_, dump := h.do(h.anaTok, "GET", "/export.json", nil)
	var ex Export
	_ = json.Unmarshal(dump, &ex)
	for run, want := range []int{4, 0} {
		var res map[string]int
		req, _ := http.NewRequest("POST", h.ts.URL+"/api/v1/import", bytes.NewReader(dump))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+bobIn)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if resp.StatusCode != 200 || res["created"] != want || res["created"]+res["skipped"] != len(ex.Books) {
			t.Fatalf("run %d: %d %v", run+1, resp.StatusCode, res)
		}
	}
	if n := h.count(h.bobTok); n != 4 {
		t.Fatalf("bob has %d books", n)
	}
	// Ana's own file back into her own shelf adds nothing either
	req, _ := http.NewRequest("POST", h.ts.URL+"/api/v1/import", bytes.NewReader(dump))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+anaIn)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if n := h.count(h.anaTok); resp.StatusCode != 200 || n != 4 {
		t.Fatalf("ana's own file: %d, she has %d books", resp.StatusCode, n)
	}

	// Goodreads and StoryGraph exports, twice each
	goodreads := "Book Id,Title,Author,Additional Authors,ISBN,ISBN13,My Rating,Exclusive Shelf,Date Read\n" +
		"1,Šta je umjetnost,Lav Tolstoj,,,=\"\",4,read,2024/01/02\n" +
		"2,Dune,Frank Herbert,,=\"0441172717\",=\"9780441172719\",5,read,2023/05/06\n" +
		"3,The Hobbit,J.R.R. Tolkien,,,,0,to-read,\n"
	storygraph := "Title,Authors,Contributors,ISBN/UID,Format,Read Status,Date Added,Last Date Read,Dates Read,Read Count,Moods,Pace,Character- or Plot-Driven?,Strong Character Development?,Loveable Characters?,Diverse Characters?,Flawed Characters?,Star Rating,Review,Content Warnings,Content Warning Description,Tags,Owned?\n" +
		"Circe,Madeline Miller,,9780316556347,digital,read,2024/01/01,2024/02/03,,1,,,,,,,,4.5,,,,myth,No\n" +
		"Ćelava pjevačica,Eugène Ionesco,,,paperback,to-read,2024/01/01,,,0,,,,,,,,,,,,,No\n"
	for _, c := range []struct {
		name, body string
		first      int
	}{{"goodreads", goodreads, 2}, {"storygraph", storygraph, 2}} {
		for run, want := range []int{c.first, 0} {
			var res map[string]int
			code, raw := h.upload(bobIn, "/import/goodreads", []byte(c.body))
			_ = json.Unmarshal(raw, &res)
			if code != 200 || res["created"] != want {
				t.Fatalf("%s run %d: %d %s", c.name, run+1, code, raw)
			}
		}
	}
	if n := h.count(h.bobTok); n != 8 {
		t.Fatalf("bob has %d books, want 8", n)
	}
	if n := h.count(h.anaTok); n != 4 {
		t.Fatalf("ana has %d books, want 4", n)
	}
}
