package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoverHintsOpenAI(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		answer, _ := json.Marshal(map[string]any{"original_title": "The Fires of Heaven", "original_authors": "Robert Jordan",
			"isbns": []string{"9788652100000"}, "publisher": "Laguna", "year": "2005", "cover_urls": []string{}})
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message",
			"content": []any{map[string]any{"type": "output_text", "text": string(answer)}}}}})
	}))
	defer srv.Close()
	o := &OpenAI{Key: "k", BaseURL: srv.URL}
	h, err := o.CoverHints(context.Background(), CoverBook{Title: "Nebeski oganj", Authors: "Robert Jordan", Language: "sr"})
	if err != nil {
		t.Fatal(err)
	}
	if h.OriginalTitle != "The Fires of Heaven" || len(h.ISBNs) != 1 || h.Publisher != "Laguna" {
		t.Fatalf("hints: %+v", h)
	}
	if !strings.Contains(got["input"].(string), "Nebeski oganj") || got["text"].(map[string]any)["format"].(map[string]any)["name"] != "book_cover_hints" {
		t.Fatalf("request: %v", got)
	}
}
