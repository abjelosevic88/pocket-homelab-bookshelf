package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CoverBook is what the model is told about a book whose cover is missing.
type CoverBook struct {
	Title, Authors, Language, Publisher, Published, ISBN13, Series, SeriesNo string
}

// CoverHints are what a model knows that identifies the book, and so its
// cover: the edition's ISBNs, the original it was translated from, and image
// addresses it is sure of. Nothing is trusted as is — every address is
// downloaded and checked before it is offered.
type CoverHints struct {
	OriginalTitle   string   `json:"original_title"`
	OriginalAuthors string   `json:"original_authors"`
	ISBNs           []string `json:"isbns"`
	Publisher       string   `json:"publisher"`
	Year            string   `json:"year"`
	CoverURLs       []string `json:"cover_urls"`
}

// CoverSchema is the JSON schema of CoverHints.
func CoverSchema() map[string]any {
	str := map[string]any{"type": "string"}
	list := map[string]any{"type": "array", "items": str}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"original_title", "original_authors", "isbns", "publisher", "year", "cover_urls"},
		"properties": map[string]any{
			"original_title": str, "original_authors": str, "isbns": list, "publisher": str, "year": str, "cover_urls": list,
		},
	}
}

// CoverPrompt asks for what identifies the edition.
func CoverPrompt(b CoverBook) string {
	var sb strings.Builder
	sb.WriteString("A reader's shelf has this book without a cover picture. Identify it so its cover can be found.\n\n")
	fmt.Fprintf(&sb, "Title: %s\nAuthors: %s\n", b.Title, b.Authors)
	for _, kv := range [][2]string{{"Language", b.Language}, {"Publisher", b.Publisher}, {"Published", b.Published}, {"ISBN", b.ISBN13}, {"Series", strings.TrimSpace(b.Series + " " + b.SeriesNo)}} {
		if kv[1] != "" {
			fmt.Fprintf(&sb, "%s: %s\n", kv[0], kv[1])
		}
	}
	sb.WriteString(`
Answer with:
- original_title / original_authors: when this is a translation, the original work's title and author as the original publisher spells them; else the same as given.
- isbns: ISBN-13s (or ISBN-10s) you are confident belong to this book, the edition in this title's language first (e.g. the translation's own publisher), then the original. Digits only. Never guess digits: leave the list empty when unsure.
- publisher / year: of the edition in this title's language, when you know them.
- cover_urls: direct image addresses (.jpg/.png/.webp) of this book's cover that you are confident exist, e.g. on the publisher's site. Empty when unsure — every address is checked and invented ones only waste time.
Use empty strings and lists for anything you do not know.`)
	return sb.String()
}

// CoverHints asks the model what identifies a book with no cover. Pocket AI
// answers from what the model knows; OpenAI may also search the web.
func (o *OpenAI) CoverHints(ctx context.Context, b CoverBook) (*CoverHints, error) {
	var text string
	if o.Pocket != nil {
		schema, _ := json.Marshal(CoverSchema())
		out, err := o.Pocket.Complete(ctx, "You identify book editions. Return one JSON object matching this JSON schema: "+string(schema)+". Use only facts you know. No web search is available.", CoverPrompt(b), true)
		if err != nil {
			return nil, err
		}
		text = out.Text
	} else {
		var err error
		if text, err = o.structured(ctx, "book_cover_hints", CoverSchema(), CoverPrompt(b)); err != nil {
			return nil, err
		}
	}
	var h CoverHints
	if err := json.Unmarshal([]byte(text), &h); err != nil {
		return nil, fmt.Errorf("the AI answer is not valid JSON: %w", err)
	}
	return &h, nil
}
