package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

func TestSummariesWrittenInEveryLanguage(t *testing.T) {
	h := newShelfHarness(t)
	var asked []string
	oai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		asked = append(asked, body.Input)
		answer := `{"status":"ok","summary":{"en":"Paul Atreides moves to the desert planet Arrakis.","sr":"Пол Атреид се сели на пустињску планету Аракис."}}`
		if strings.Contains(body.Input, "Nobody Knows") {
			answer = `{"status":"unknown","summary":{"en":"","sr":""}}`
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
	if len(asked) != 2 || !strings.Contains(asked[0], "No spoilers") {
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
	// Bob cannot read Ana's.
	if code, _ := h.do(h.bobTok, "GET", fmt.Sprintf("/books/%d/summary", dune.Book.ID), nil); code != 404 {
		t.Fatalf("bob: %d", code)
	}
}
