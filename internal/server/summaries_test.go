package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

func TestSummariesWrittenInEveryLanguage(t *testing.T) {
	h := newShelfHarness(t)
	var asked []string
	var facts int
	var mu sync.Mutex
	var duneID, oddID atomic.Int64
	oai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var answer string
		switch {
		case strings.Contains(body.Input, "which series"):
			mu.Lock()
			facts++
			mu.Unlock()
			answer = fmt.Sprintf(`{"books":[{"id":%d,"series":"Dune Chronicles","series_no":"1","original_title":""},{"id":%d,"series":"","series_no":"","original_title":""}]}`, duneID.Load(), oddID.Load())
		case strings.Contains(body.Input, "without a cover picture"):
			answer = `{"original_title":"","original_authors":"","isbns":[],"publisher":"","year":"","cover_urls":[]}`
		default:
			mu.Lock()
			asked = append(asked, body.Input)
			mu.Unlock()
			answer = `{"status":"ok","summary":{"en":"Paul Atreides moves to the desert planet Arrakis.","sr":"Пол Атреид се сели на пустињску планету Аракис."}}`
			if strings.Contains(body.Input, "Nobody Knows") {
				answer = `{"status":"unknown","summary":{"en":"","sr":""}}`
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message",
			"content": []any{map[string]any{"type": "output_text", "text": answer}}}}})
	}))
	t.Cleanup(oai.Close)
	h.cfg.OpenAIBaseURL = oai.URL
	ctx := context.Background()
	if code := h.json(h.anaTok, "PUT", "/settings", map[string]any{"languages": "en,sr"}, nil); code != 200 {
		t.Fatalf("languages: %d", code)
	}
	if err := h.st.SaveAISettings(ctx, h.ana.ID, store.AISettings{AIProvider: "openai", OpenAIKey: "sk-test"}); err != nil {
		t.Fatal(err)
	}
	var dune, odd struct{ Book store.Book }
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Dune", "authors": "Frank Herbert"}, &dune)
	h.json(h.anaTok, "POST", "/books", map[string]any{"title": "Nobody Knows", "authors": "X"}, &odd)
	duneID.Store(dune.Book.ID)
	oddID.Store(odd.Book.ID)

	h.s.writeMissingSummaries(ctx)
	var got struct {
		Langs     []string                 `json:"langs"`
		Summaries map[string]store.Summary `json:"summaries"`
	}
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d/summary", dune.Book.ID), nil, &got)
	if got.Summaries["en"].Status != "ok" || !strings.HasPrefix(got.Summaries["en"].Text, "Paul Atreides") {
		t.Fatalf("en: %+v", got.Summaries)
	}
	if sr := got.Summaries["sr"].Text; sr != "Pol Atreid se seli na pustinjsku planetu Arakis." {
		t.Fatalf("sr not in Latin script: %q", sr)
	}
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d/summary", odd.Book.ID), nil, &got)
	if got.Summaries["en"].Status != "unknown" {
		t.Fatalf("unknown book: %+v", got.Summaries)
	}
	mu.Lock()
	n, first := len(asked), asked[0]
	mu.Unlock()
	if n != 2 || !strings.Contains(first, "No spoilers") {
		t.Fatalf("asked %d times", len(asked))
	}
	// Written once: a second run asks nothing; deleting asks again.
	h.s.writeMissingSummaries(ctx)
	if len(asked) != 2 {
		t.Fatalf("asked again: %d", len(asked))
	}
	if code, _ := h.do(h.anaTok, "DELETE", fmt.Sprintf("/books/%d/summary", dune.Book.ID), nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	h.s.writeMissingSummaries(ctx)
	if len(asked) != 3 {
		t.Fatalf("not written again: %d", len(asked))
	}
	// Series asked once for both books, in one request; only blanks filled.
	var got2 struct{ Book store.Book }
	h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d", dune.Book.ID), nil, &got2)
	if got2.Book.Series != "Dune Chronicles" || got2.Book.SeriesNo != "1" {
		t.Fatalf("series: %+v", got2.Book)
	}
	mu.Lock()
	if facts != 1 {
		t.Fatalf("facts asked %d times", facts)
	}
	mu.Unlock()
	// Bob cannot read Ana's.
	if code, _ := h.do(h.bobTok, "GET", fmt.Sprintf("/books/%d/summary", dune.Book.ID), nil); code != 404 {
		t.Fatalf("bob: %d", code)
	}
}
