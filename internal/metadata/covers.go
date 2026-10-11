package metadata

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// CoverOption is one cover to choose from.
type CoverOption struct {
	URL      string `json:"url"`   // large, for saving
	Thumb    string `json:"thumb"` // small, for the picker
	Source   string `json:"source"`
	Label    string `json:"label"` // "2019 · Laguna" (the language is separate)
	Language string `json:"language,omitempty"`
}

// CoverSuggestions collects covers for a book: every edition of its Open
// Library work (found by title and author when the book is not linked), and
// Google Books volumes when a key is set. Covers in a preferred language come
// first.
func (c *Client) CoverSuggestions(ctx context.Context, workKey, title, authors string) []CoverOption {
	if workKey == "" && title != "" {
		if res, err := c.SearchOpenLibrary(ctx, strings.TrimSpace(title+" "+strings.Split(authors, ",")[0]), 5); err == nil && len(res) > 0 {
			workKey = res[0].SourceID
		}
	}
	var ol, gb []CoverOption
	var wg sync.WaitGroup
	if strings.HasPrefix(workKey, "/works/") {
		wg.Add(1)
		go func() { defer wg.Done(); ol = c.editionCovers(ctx, workKey) }()
	}
	if c.GoogleKey() != "" && title != "" {
		wg.Add(1)
		go func() { defer wg.Done(); gb = c.googleCovers(ctx, title, authors) }()
	}
	wg.Wait()
	out := append(ol, gb...)
	langs := c.langs()
	rank := func(l string) int {
		for i, p := range langs {
			if p == l {
				return i
			}
		}
		return len(langs)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i].Language) < rank(out[j].Language) })
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func (c *Client) editionCovers(ctx context.Context, workKey string) []CoverOption {
	var r struct {
		Entries []struct {
			Covers      []int    `json:"covers"`
			PublishDate string   `json:"publish_date"`
			Publishers  []string `json:"publishers"`
			Languages   []struct {
				Key string `json:"key"`
			} `json:"languages"`
		} `json:"entries"`
	}
	if err := c.getJSON(ctx, olBase+workKey+"/editions.json?limit=300", &r); err != nil {
		return nil
	}
	seen := map[int]bool{}
	var out []CoverOption
	for _, e := range r.Entries {
		id := 0
		for _, cv := range e.Covers {
			if cv > 0 {
				id = cv
				break
			}
		}
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		lang := ""
		if len(e.Languages) > 0 {
			lang = iso639(strings.TrimPrefix(e.Languages[0].Key, "/languages/"))
		}
		var label []string
		if y := firstYear(e.PublishDate); y != "" {
			label = append(label, y)
		}
		if len(e.Publishers) > 0 {
			label = append(label, e.Publishers[0])
		}
		out = append(out, CoverOption{URL: olCover(id, "L"), Thumb: olCover(id, "M"), Source: "openlibrary", Label: strings.Join(label, " · "), Language: lang})
	}
	return out
}

func (c *Client) googleCovers(ctx context.Context, title, authors string) []CoverOption {
	q := fmt.Sprintf(`intitle:"%s"`, title)
	if a := strings.TrimSpace(strings.Split(authors, ",")[0]); a != "" {
		q += fmt.Sprintf(` inauthor:"%s"`, a)
	}
	v := url.Values{"q": {q}, "maxResults": {"20"}, "printType": {"books"}, "key": {c.GoogleKey()}, "country": {c.googleCountry()}}
	var res gbSearch
	if err := c.getJSON(ctx, gbBase+"?"+v.Encode(), &res); err != nil {
		return nil
	}
	var out []CoverOption
	seen := map[string]bool{}
	for _, it := range res.Items {
		cand := gbCandidate(it)
		if cand.ThumbURL == "" || seen[cand.ThumbURL] {
			continue
		}
		seen[cand.ThumbURL] = true
		label := []string{"Google Books"}
		if cand.Published != "" {
			label = append(label, cand.Published)
		}
		if cand.Publisher != "" {
			label = append(label, cand.Publisher)
		}
		out = append(out, CoverOption{URL: cand.CoverURL, Thumb: cand.ThumbURL, Source: "google", Label: strings.Join(label, " · "), Language: cand.Language})
	}
	return out
}

func firstYear(s string) string {
	for i := 0; i+4 <= len(s); i++ {
		if y, err := strconv.Atoi(s[i : i+4]); err == nil && y > 1400 && y < 2200 {
			return s[i : i+4]
		}
	}
	return ""
}
