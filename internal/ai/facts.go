package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FactsBook is one book asked about.
type FactsBook struct {
	ID                              int64
	Title, Authors, Published, Lang string
}

// BookFacts is what the model knows of a book's place in the world.
type BookFacts struct {
	ID            int64  `json:"id"`
	Series        string `json:"series"`
	SeriesNo      string `json:"series_no"`
	OriginalTitle string `json:"original_title"`
}

const factsText = `For each book on a reader's shelf, say which series it belongs to and its original title.

Books (the shelf title may be a translation, often Serbian):
%s

Series already on this shelf — use exactly one of these names when the book belongs to it:
%s

For each book answer:
- series: the series or cycle it is part of (e.g. "The Wheel of Time"); empty for a standalone book.
- series_no: its number in that series as digits ("5", "0" for a prequel, "2.5" for a novella between); empty when unsure.
- original_title: the title of the original work in its original language when the shelf title is a translation
  (e.g. "The Fires of Heaven" for "Nebeski oganj"); empty when the shelf title already is the original.
Only facts you are sure of; leave a field empty rather than guess.`

// FactsSchema is the JSON schema of the answer.
func FactsSchema() map[string]any {
	str := map[string]any{"type": "string"}
	book := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "series", "series_no", "original_title"},
		"properties": map[string]any{"id": map[string]any{"type": "integer"}, "series": str, "series_no": str, "original_title": str}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"books"},
		"properties": map[string]any{"books": map[string]any{"type": "array", "items": book}}}
}

// FactsPrompt asks about several books at once.
func FactsPrompt(books []FactsBook, series []string) string {
	var lines []string
	for _, b := range books {
		l := fmt.Sprintf("- id %d: %q by %s", b.ID, b.Title, firstNonEmpty(b.Authors, "unknown"))
		if b.Published != "" {
			l += ", published " + b.Published
		}
		if b.Lang != "" {
			l += ", language " + b.Lang
		}
		lines = append(lines, l)
	}
	known := "(none yet)"
	if len(series) > 0 {
		known = "- " + strings.Join(series, "\n- ")
	}
	return fmt.Sprintf(factsText, strings.Join(lines, "\n"), known)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Facts asks the model for the series and original titles of books.
func (o *OpenAI) Facts(ctx context.Context, books []FactsBook, series []string) ([]BookFacts, error) {
	var text string
	if o.Pocket != nil {
		schema, _ := json.Marshal(FactsSchema())
		out, err := o.Pocket.Complete(ctx, "Return one JSON object matching this JSON schema: "+string(schema)+". Use only facts you know. No web search is available.", FactsPrompt(books, series), true)
		if err != nil {
			return nil, err
		}
		text = out.Text
	} else {
		var err error
		if text, err = o.structured(ctx, "book_facts", FactsSchema(), FactsPrompt(books, series)); err != nil {
			return nil, err
		}
	}
	var r struct {
		Books []BookFacts `json:"books"`
	}
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		return nil, fmt.Errorf("the AI answer is not valid JSON: %w", err)
	}
	return r.Books, nil
}
