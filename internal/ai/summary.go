package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SummaryResult is the model's answer: status is ok | unknown, summary maps
// a language to one paragraph.
type SummaryResult struct {
	Status  string            `json:"status"`
	Summary map[string]string `json:"summary"`
}

const summaryText = `You are writing the first paragraph of a book's page in a personal reading log: a short description
of what the book is about, for someone deciding whether to read it or about to start it.

The book:
%s

First make sure which book this is — the shelf title may be a translation (often Serbian). If you are not
completely sure, look it up on the web (Wikipedia, the publisher, reputable reviews) before writing. Never
invent anything.

Write ONE paragraph of 2 to 4 sentences, at most 80 words.
- Fiction: the premise as the back cover or the first chapters present it — who, where, when, and what they
  want or are up against. In a later book of a series, where the story picks up is fine.
- Non-fiction: what the book covers, its main question or argument and the author's approach.

No spoilers — the reader may not have started: nothing past the opening setup. No twists, revelations,
deaths, fates, endings or who is behind what; no "eventually", "ultimately", "turns out", "is revealed"
(nor "na kraju", "ispostavi se", "kasnije" and the like). When in doubt, leave it out.
No praise or verdicts ("a masterpiece", "gripping"), no awards, no "this book" openings — start with the story
or the subject. Plain prose: no headings, lists, quotes or markdown.

If you still cannot identify the book reliably, answer with status "unknown" and empty strings.

Write it in these languages, the same content in each:
%s

Write Latin-script languages entirely in Latin letters — not a single Cyrillic letter.`

// SummaryPrompt is the instruction for one book.
func SummaryPrompt(b Book, langs []string) string {
	var ls []string
	for _, l := range langs {
		name := langNames[l]
		if name == "" {
			name = l
		}
		if i := strings.Index(name, "."); i > 0 { // "Serbian in the Latin script (latinica). Use the character names…"
			name = name[:i]
		}
		ls = append(ls, fmt.Sprintf("- %s: %s", l, name))
	}
	return fmt.Sprintf(summaryText, bookFacts(b), strings.Join(ls, "\n"))
}

// SummarySchema is the JSON Schema of the answer.
func SummarySchema(langs []string) map[string]any {
	props := map[string]any{}
	for _, l := range langs {
		props[l] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"status", "summary"},
		"properties": map[string]any{
			"status":  map[string]any{"type": "string", "enum": []string{"ok", "unknown"}},
			"summary": map[string]any{"type": "object", "additionalProperties": false, "required": langs, "properties": props},
		},
	}
}

// Summary asks the model for a book's spoiler-free summary in langs.
func (o *OpenAI) Summary(ctx context.Context, b Book, langs []string) (*SummaryResult, error) {
	var text string
	if o.Pocket != nil {
		schema, _ := json.Marshal(SummarySchema(langs))
		out, err := o.Pocket.Complete(ctx, "Return one JSON object matching this JSON schema: "+string(schema)+". Use only facts you know; never invent. No web search is available.", SummaryPrompt(b, langs), true)
		if err != nil {
			return nil, err
		}
		text = out.Text
	} else {
		var err error
		if text, err = o.structured(ctx, "book_summary", SummarySchema(langs), SummaryPrompt(b, langs)); err != nil {
			return nil, err
		}
	}
	var r SummaryResult
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		return nil, fmt.Errorf("the AI answer is not valid JSON: %w", err)
	}
	return &r, nil
}

// Source names who wrote an answer.
func (o *OpenAI) Source() string {
	if o.Pocket != nil {
		return "pocket-ai"
	}
	m := o.Model
	if m == "" {
		m = DefaultModel
	}
	return "openai " + m
}
