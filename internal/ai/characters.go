// Package ai writes book characters with a language model (OpenAI). The same
// prompt is used by scripts/characters.py — keep the two in step.
package ai

import (
	"fmt"
	"strings"
)

// Book is what the model is told about a book.
type Book struct {
	Title, Subtitle, WorkTitle, Authors, Published, Series, SeriesNo, SourceURL, Description string
}

// Character is one person in a book.
type Character struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
}

// Result is the model's answer: kind is fiction | nonfiction | unknown.
type Result struct {
	Kind       string                 `json:"kind"`
	Characters map[string][]Character `json:"characters"`
}

var langNames = map[string]string{
	"en": "English",
	"sr": "Serbian in the Latin script (latinica). Use the character names as Serbian translations of the book spell them " +
		"when that is well established (e.g. Frodo Bagins); otherwise keep the original spelling",
	"hr": "Croatian", "bs": "Bosnian", "sl": "Slovenian", "mk": "Macedonian", "de": "German", "fr": "French",
	"es": "Spanish", "it": "Italian", "pt": "Portuguese", "ru": "Russian", "hu": "Hungarian", "pl": "Polish",
	"cs": "Czech", "sk": "Slovak", "nl": "Dutch", "sv": "Swedish", "no": "Norwegian", "da": "Danish",
	"fi": "Finnish", "ro": "Romanian", "bg": "Bulgarian", "el": "Greek", "tr": "Turkish", "uk": "Ukrainian",
}

// LatinLangs are asked for in the Latin script; stray Cyrillic gets transliterated.
var LatinLangs = map[string]bool{"sr": true, "hr": true, "bs": true, "sl": true}

const promptText = `You are writing the character list for a book in a personal reading log.

The book:
%s

First make sure which book this is — the shelf title may be a translation (often Serbian). If you are not
completely sure of the book or of its characters, look it up on the web (Wikipedia, the publisher, reputable
reference or fan wikis) before writing. Never invent characters or details.

List the book's characters: every main character and the important supporting ones — usually 5 to 15, up to
25 for large ensemble novels. For each, one or two sentences that help a reader keep track of who is who:
who they are, what they do, how they relate to the others.

Spoilers — the reader may not have finished the book. Describe each character only as the reader meets them
in the first chapters of THIS book, in the present tense: who they are and where they stand at that point.
- nothing that happens after their introduction: no events from the middle or end of the book, no deaths,
  fates, betrayals, twists, secret or hidden identities, later jobs, marriages or relationships;
- no "later", "eventually", "turns out", "is revealed", "ultimately", "comes to" (nor "kasnije", "na kraju",
  "ispostavi se" and the like in other languages);
- a character who only appears later in the book: say who they are, not what happens when they appear;
- in a later book of a series, what earlier books established is fine;
- when in doubt, leave it out.

If the book has no characters — non-fiction, history, biography, memoir, essays, self-help, science, poetry —
answer with kind "nonfiction" and empty lists. If you still cannot identify the book or its characters
reliably after looking it up, answer with kind "unknown" and empty lists.

Write the list in these languages, the same characters in the same order in each:
%s

Write Latin-script languages entirely in Latin letters — not a single Cyrillic letter.`

// Prompt is the instruction for one book.
func Prompt(b Book, langs []string) string {
	var ls []string
	for _, l := range langs {
		name := langNames[l]
		if name == "" {
			name = l
		}
		ls = append(ls, fmt.Sprintf("- %s: %s", l, name))
	}
	return fmt.Sprintf(promptText, bookFacts(b), strings.Join(ls, "\n"))
}

// bookFacts lists what the shelf knows about a book, one "- " line each.
func bookFacts(b Book) string {
	title := b.Title
	if b.Subtitle != "" {
		title += ": " + b.Subtitle
	}
	facts := []string{"- Title on the shelf: " + title}
	if b.WorkTitle != "" {
		facts = append(facts, "- Original title: "+b.WorkTitle)
	}
	authors := b.Authors
	if authors == "" {
		authors = "unknown"
	}
	facts = append(facts, "- Author(s): "+authors)
	if b.Published != "" {
		facts = append(facts, "- Published: "+b.Published)
	}
	if b.Series != "" {
		s := "- Series: " + b.Series
		if b.SeriesNo != "" {
			s += ", book " + b.SeriesNo
		}
		facts = append(facts, s)
	}
	if b.SourceURL != "" {
		facts = append(facts, "- Catalogue page: "+b.SourceURL)
	}
	if b.Description != "" {
		facts = append(facts, "- Catalogue description: "+b.Description)
	}
	return strings.Join(facts, "\n")
}

// Schema is the JSON Schema of the answer (strict: every field required).
func Schema(langs []string) map[string]any {
	person := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"name", "role", "description"},
		"properties": map[string]any{
			"name":        map[string]any{"type": "string"},
			"role":        map[string]any{"type": "string", "enum": []string{"main", "supporting"}},
			"description": map[string]any{"type": "string"},
		},
	}
	props := map[string]any{}
	for _, l := range langs {
		props[l] = map[string]any{"type": "array", "items": person}
	}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"kind", "characters"},
		"properties": map[string]any{
			"kind":       map[string]any{"type": "string", "enum": []string{"fiction", "nonfiction", "unknown"}},
			"characters": map[string]any{"type": "object", "additionalProperties": false, "required": langs, "properties": props},
		},
	}
}
