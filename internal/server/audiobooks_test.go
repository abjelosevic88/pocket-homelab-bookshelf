package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Audiobooks finished in Pocket Audiobooks become reads: matched to a book on
// the shelves by title and author, or added as an audio book with its cover;
// one in progress goes on "reading". A second sync adds nothing, and a book the
// person removed is not added again.
func TestAudiobooksSync(t *testing.T) {
	h := newShelfHarness(t)
	if err := os.MkdirAll(h.cfg.CoverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	ab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pcx_ab_ana" {
			http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/finished":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"item_id": "li-dune", "title": "Dune (Unabridged)", "author": "Frank Herbert", "finished_at": today + "T10:00:00Z", "day": today},
				{"item_id": "li-tm", "title": "The Time Machine", "author": "H. G. Wells", "finished_at": today + "T09:00:00Z", "day": today},
			})
		case r.URL.Path == "/api/v1/history":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
				{"item_id": "li-drac", "title": "Dracula", "author": "Bram Stoker", "day": today, "duration": 55000, "end_pos": 9000},
				{"item_id": "li-dune", "title": "Dune", "author": "Frank Herbert", "day": "2026-01-03", "duration": 70000, "end_pos": 70000},
			}})
		case strings.HasPrefix(r.URL.Path, "/api/v1/cover/"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nfake"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ab.Close)
	h.linkToken = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			User   string   `json:"user"`
			App    string   `json:"app"`
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.App != "audiobooks" || len(in.Scopes) != 1 || in.Scopes[0] != "listening:read" || in.User != "ana@example.com" {
			http.Error(w, `{"error":"switched off","reason":"off_person"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_ab_ana", "url": ab.URL, "public_url": "https://audio.example.com"})
	}
	// Dune is already on Ana's shelves (wanted): it is matched, not added
	var dune struct{ Book struct{ ID int64 } }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "want"}, &dune)

	var st abState
	h.json(h.anaTok, "POST", "/audiobooks/sync", nil, &st)
	if st.Error != "" || st.Added != 2 || st.Finished != 2 || st.Started != 1 {
		t.Fatalf("first sync: %+v", st)
	}
	var lib []struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		Format     string `json:"format"`
		Cover      string `json:"cover"`
		StartedOn  string `json:"started_on"`
		FinishedOn string `json:"finished_on"`
	}
	h.json(h.anaTok, "GET", "/books", nil, &lib)
	got := map[string]string{}
	for _, b := range lib {
		got[b.Title] = b.Status + "|" + b.Format + "|" + b.StartedOn + "|" + b.FinishedOn + "|" + map[bool]string{true: "cover", false: ""}[b.Cover != ""]
	}
	if len(lib) != 3 || got["Dune"] != "read||2026-01-03|"+today+"|" || got["The Time Machine"] != "read|audio||"+today+"|cover" || got["Dracula"] != "reading|audio|"+today+"||cover" {
		t.Fatalf("shelves: %v", got)
	}

	// again: nothing new
	h.json(h.anaTok, "POST", "/audiobooks/sync", nil, &st)
	if st.Added != 0 || st.Finished != 0 || st.Started != 0 {
		t.Fatalf("second sync: %+v", st)
	}
	// Ana removes Dracula: it stays gone
	for _, b := range lib {
		if b.Title == "Dracula" {
			if c, _ := h.do(h.anaTok, "DELETE", "/books/"+strconv.FormatInt(b.ID, 10), nil); c != http.StatusNoContent && c != http.StatusOK {
				t.Fatalf("delete: %d", c)
			}
		}
	}
	h.json(h.anaTok, "POST", "/audiobooks/sync", nil, &st)
	h.json(h.anaTok, "GET", "/books", nil, &lib)
	if len(lib) != 2 {
		t.Fatalf("removed book came back: %d books", len(lib))
	}

	// Bob's link is off: said, not an error, and nothing of Ana's shows for him
	h.json(h.bobTok, "POST", "/audiobooks/sync", nil, &st)
	if !st.Off || !strings.Contains(st.Error, "Apps acting for me") {
		t.Fatalf("bob: %+v", st)
	}
	h.json(h.bobTok, "GET", "/books", nil, &lib)
	if len(lib) != 0 {
		t.Fatalf("bob sees %d books", len(lib))
	}
}
