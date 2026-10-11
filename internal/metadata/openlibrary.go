package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const olBase = "https://openlibrary.org"

const olFields = "key,title,subtitle,author_name,first_publish_year,isbn,cover_i,number_of_pages_median,publisher,language,subject,edition_count"

type olDoc struct {
	Key          string   `json:"key"`
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle"`
	AuthorName   []string `json:"author_name"`
	FirstPublish int      `json:"first_publish_year"`
	ISBN         []string `json:"isbn"`
	CoverI       int      `json:"cover_i"`
	PagesMedian  int      `json:"number_of_pages_median"`
	Publisher    []string `json:"publisher"`
	Language     []string `json:"language"`
	Subject      []string `json:"subject"`
	EditionCount int      `json:"edition_count"`
}

type olSearch struct {
	Docs []olDoc `json:"docs"`
}

func olCover(id int, size string) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-%s.jpg", id, size)
}

// SearchOpenLibrary searches openlibrary.org by words or ISBN.
func (c *Client) SearchOpenLibrary(ctx context.Context, query string, limit int) ([]Candidate, error) {
	isbn := ""
	q := query
	if LooksLikeISBN(query) {
		isbn = NormalizeISBN(strings.TrimLeft(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "isbn"), ": "))
		q = "isbn:" + isbn
	}
	v := url.Values{}
	v.Set("q", q)
	v.Set("limit", strconv.Itoa(limit))
	v.Set("fields", olFields)
	var res olSearch
	var prefs map[string][]edition
	var prefOnly []olDoc
	var wg sync.WaitGroup
	if isbn == "" {
		wg.Add(1)
		go func() { defer wg.Done(); prefs, prefOnly = c.preferredEditions(ctx, q, nil, limit) }()
	}
	err := c.getJSON(ctx, olBase+"/search.json?"+v.Encode(), &res)
	wg.Wait()
	if err != nil && len(prefOnly) == 0 {
		return nil, err
	}
	// Works with an edition in a preferred language first (catalogue order),
	// then works found only through the language queries, then the rest.
	var withPref, without []olDoc
	inBase := map[string]bool{}
	for _, d := range res.Docs {
		inBase[d.Key] = true
		if len(prefs[d.Key]) > 0 {
			withPref = append(withPref, d)
		} else {
			without = append(without, d)
		}
	}
	for _, d := range prefOnly {
		if !inBase[d.Key] {
			withPref = append(withPref, d)
		}
	}
	docs := append(withPref, without...)
	if len(docs) > limit {
		docs = docs[:limit]
	}
	out := make([]Candidate, 0, len(docs))
	for _, d := range docs {
		cand := olCandidate(d, isbn)
		applyPreferred(&cand, prefs[d.Key])
		out = append(out, cand)
	}
	// An ISBN names one edition: take its page count, publisher and cover.
	if isbn != "" && len(out) > 0 {
		if ed, err := c.openLibraryEdition(ctx, isbn); err == nil {
			applyEdition(&out[0], ed)
		}
	}
	return out, nil
}

func olCandidate(d olDoc, wantISBN string) Candidate {
	c := Candidate{
		Editions:  d.EditionCount,
		Source:    "openlibrary",
		SourceID:  d.Key,
		SourceURL: olBase + d.Key,
		Title:     d.Title,
		Subtitle:  d.Subtitle,
		Authors:   firstN(d.AuthorName, 4),
		Pages:     d.PagesMedian,
		Subjects:  cleanSubjects(d.Subject),
		CoverURL:  olCover(d.CoverI, "L"),
		ThumbURL:  olCover(d.CoverI, "M"),
	}
	if d.FirstPublish > 0 {
		c.Published = strconv.Itoa(d.FirstPublish)
	}
	if len(d.Publisher) > 0 {
		c.Publisher = d.Publisher[0]
	}
	if len(d.Language) == 1 {
		c.Language = iso639(d.Language[0])
	}
	for _, l := range firstN(d.Language, 40) {
		c.Languages = append(c.Languages, iso639(l))
	}
	c.ISBN13, c.ISBN10 = pickISBNs(d.ISBN, wantISBN)
	return c
}

// pickISBNs chooses the ISBNs to show for a work: the one searched for if
// present, otherwise the first of each length.
func pickISBNs(all []string, want string) (isbn13, isbn10 string) {
	for _, s := range all {
		switch {
		case want != "" && s == want && len(s) == 13:
			return s, ""
		case want != "" && s == want && len(s) == 10:
			return ISBN10to13(s), s
		}
	}
	for _, s := range all {
		if len(s) == 13 && isbn13 == "" && (strings.HasPrefix(s, "978") || strings.HasPrefix(s, "979")) {
			isbn13 = s
		}
		if len(s) == 10 && isbn10 == "" {
			isbn10 = s
		}
	}
	return isbn13, isbn10
}

// cleanSubjects keeps short, human subjects ("Science fiction") and drops
// catalogue noise ("Fiction, science fiction, general", "nyt:…").
func cleanSubjects(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if s == "" || len(s) > 28 || strings.ContainsAny(s, ":,()") || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
		if len(out) == 8 {
			break
		}
	}
	return out
}

type olEdition struct {
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle"`
	Publishers    []string `json:"publishers"`
	NumberOfPages int      `json:"number_of_pages"`
	PublishDate   string   `json:"publish_date"`
	Covers        []int    `json:"covers"`
	ISBN10        []string `json:"isbn_10"`
	ISBN13        []string `json:"isbn_13"`
	Languages     []struct {
		Key string `json:"key"`
	} `json:"languages"`
}

func (c *Client) openLibraryEdition(ctx context.Context, isbn string) (*olEdition, error) {
	var ed olEdition
	if err := c.getJSON(ctx, olBase+"/isbn/"+url.PathEscape(isbn)+".json", &ed); err != nil {
		return nil, err
	}
	return &ed, nil
}

func applyEdition(c *Candidate, ed *olEdition) {
	if ed.NumberOfPages > 0 {
		c.Pages = ed.NumberOfPages
	}
	if len(ed.Publishers) > 0 {
		c.Publisher = ed.Publishers[0]
	}
	if len(ed.Covers) > 0 && ed.Covers[0] > 0 {
		c.CoverURL, c.ThumbURL = olCover(ed.Covers[0], "L"), olCover(ed.Covers[0], "M")
	}
	if len(ed.ISBN13) > 0 {
		c.ISBN13 = ed.ISBN13[0]
	}
	if len(ed.ISBN10) > 0 {
		c.ISBN10 = ed.ISBN10[0]
	}
	if len(ed.Languages) > 0 {
		c.Language = strings.TrimPrefix(ed.Languages[0].Key, "/languages/")
	}
}

type olWork struct {
	Title       string          `json:"title"`
	Subtitle    string          `json:"subtitle"`
	Description json.RawMessage `json:"description"`
	Subjects    []string        `json:"subjects"`
	Covers      []int           `json:"covers"`
	FirstPub    string          `json:"first_publish_date"`
}

// olText reads Open Library's "string or {type, value}" text fields.
func olText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var v struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &v) == nil {
		return v.Value
	}
	return ""
}

// stripOLSources drops the "([source][1])" footnotes Open Library descriptions carry.
func stripOLSources(s string) string {
	if i := strings.Index(s, "\n----------"); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func (c *Client) openLibraryWork(ctx context.Context, key string) (*Candidate, error) {
	if !strings.HasPrefix(key, "/works/") {
		return nil, fmt.Errorf("not an Open Library work key: %q", key)
	}
	var w olWork
	if err := c.getJSON(ctx, olBase+key+".json", &w); err != nil {
		return nil, err
	}
	out := &Candidate{
		Source: "openlibrary", SourceID: key, SourceURL: olBase + key,
		Title: w.Title, Subtitle: w.Subtitle,
		Description: PlainText(stripOLSources(olText(w.Description))),
		Subjects:    cleanSubjects(w.Subjects),
	}
	if len(w.Covers) > 0 && w.Covers[0] > 0 {
		out.CoverURL, out.ThumbURL = olCover(w.Covers[0], "L"), olCover(w.Covers[0], "M")
	}
	return out, nil
}
