package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
)

// AuthorCandidate is one author search result.
type AuthorCandidate struct {
	Key       string   `json:"key"` // "OL31353A"
	Name      string   `json:"name"`
	BirthDate string   `json:"birth_date"`
	DeathDate string   `json:"death_date"`
	TopWork   string   `json:"top_work"`
	WorkCount int      `json:"work_count"`
	Subjects  []string `json:"subjects"`
	PhotoURL  string   `json:"photo_url"`
	alternate []string
}

// AuthorInfo is the full record of an author.
type AuthorInfo struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Bio       string `json:"bio"`
	BioFull   string `json:"bio_full"`
	BioSource string `json:"bio_source"`
	BirthDate string `json:"birth_date"`
	DeathDate string `json:"death_date"`
	PhotoURL  string `json:"photo_url"`
	Wikipedia string `json:"wikipedia"`
	Names     []string
}

func olAuthorPhoto(key string) string {
	return "https://covers.openlibrary.org/a/olid/" + key + "-M.jpg?default=false"
}

// SearchAuthors finds authors on Open Library by name, most prolific first.
func (c *Client) SearchAuthors(ctx context.Context, name string, limit int) ([]AuthorCandidate, error) {
	if len([]rune(strings.TrimSpace(name))) < 3 {
		return []AuthorCandidate{}, nil
	}
	var res struct {
		Docs []struct {
			Key       string   `json:"key"`
			Name      string   `json:"name"`
			Alternate []string `json:"alternate_names"`
			BirthDate string   `json:"birth_date"`
			DeathDate string   `json:"death_date"`
			TopWork   string   `json:"top_work"`
			WorkCount int      `json:"work_count"`
			Subjects  []string `json:"top_subjects"`
		} `json:"docs"`
	}
	v := url.Values{"q": {name}, "limit": {strconv.Itoa(limit)}}
	if err := c.getJSON(ctx, olBase+"/search/authors.json?"+v.Encode(), &res); err != nil {
		return nil, err
	}
	out := []AuthorCandidate{}
	for _, d := range res.Docs {
		// Records naming several people at once, or with no works, are catalogue noise.
		if strings.Contains(d.Name, ";") || d.WorkCount == 0 {
			continue
		}
		out = append(out, AuthorCandidate{Key: d.Key, Name: d.Name, BirthDate: d.BirthDate, DeathDate: d.DeathDate, TopWork: d.TopWork,
			WorkCount: d.WorkCount, Subjects: firstN(d.Subjects, 4), PhotoURL: olAuthorPhoto(d.Key), alternate: d.Alternate})
	}
	return out, nil
}

// AllNames is the name with its alternates, for matching.
func (a AuthorCandidate) AllNames() []string { return append([]string{a.Name}, a.alternate...) }

// WorkAuthorKeys returns the author keys of an Open Library work, in order.
func (c *Client) WorkAuthorKeys(ctx context.Context, workKey string) ([]string, error) {
	var w struct {
		Authors []struct {
			Author struct {
				Key string `json:"key"`
			} `json:"author"`
		} `json:"authors"`
	}
	if err := c.getJSON(ctx, olBase+workKey+".json", &w); err != nil {
		return nil, err
	}
	var out []string
	for _, a := range w.Authors {
		if k := strings.TrimPrefix(a.Author.Key, "/authors/"); k != "" {
			out = append(out, k)
		}
	}
	return out, nil
}

var (
	mdLink     = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	mdSource   = regexp.MustCompile(`(?i)\s*\(\s*\[?source\]?(\([^)]*\))?\s*\)`)
	mdRefDef   = regexp.MustCompile(`(?m)^\s*\[\d+\]:.*$`)
	mdEmphasis = regexp.MustCompile(`[*_]{1,2}([^*_]+)[*_]{1,2}`)
)

// cleanOLBio turns an Open Library biography (markdown, source notes, often
// a second language after a rule) into plain text.
func cleanOLBio(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, cut := range []string{"\n---", "\n___", "\n***"} {
		if i := strings.Index(s, cut); i > 0 {
			s = s[:i]
		}
	}
	s = mdSource.ReplaceAllString(s, "")
	s = mdRefDef.ReplaceAllString(s, "")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdEmphasis.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, `\[`, "[")
	s = strings.ReplaceAll(s, `\]`, "]")
	return PlainText(s)
}

// AuthorDetails fetches an author record. The biography is the English
// Wikipedia summary when Open Library links the author to Wikidata, else
// Open Library's own text.
func (c *Client) AuthorDetails(ctx context.Context, key string) (*AuthorInfo, error) {
	var a struct {
		Name      string          `json:"name"`
		Personal  string          `json:"personal_name"`
		Alternate []string        `json:"alternate_names"`
		Bio       json.RawMessage `json:"bio"`
		BirthDate string          `json:"birth_date"`
		DeathDate string          `json:"death_date"`
		Photos    []int           `json:"photos"`
		Wikipedia string          `json:"wikipedia"`
		RemoteIDs map[string]any  `json:"remote_ids"`
		Links     []struct {
			URL string `json:"url"`
		} `json:"links"`
	}
	if err := c.getJSON(ctx, olBase+"/authors/"+url.PathEscape(key)+".json", &a); err != nil {
		return nil, err
	}
	info := &AuthorInfo{Key: key, Name: a.Name, BirthDate: a.BirthDate, DeathDate: a.DeathDate, Names: append([]string{a.Name, a.Personal}, a.Alternate...)}
	if bio := cleanOLBio(olText(a.Bio)); bio != "" {
		info.Bio, info.BioSource = bio, olBase+"/authors/"+key
	}
	for _, p := range a.Photos {
		if p > 0 {
			info.PhotoURL = fmt.Sprintf("https://covers.openlibrary.org/a/id/%d-L.jpg", p)
			break
		}
	}
	wikiTitle := ""
	if qid, _ := a.RemoteIDs["wikidata"].(string); qid != "" {
		wikiTitle, _ = c.wikidataEnwiki(ctx, qid)
	}
	if wikiTitle == "" && strings.Contains(a.Wikipedia, "en.wikipedia.org/wiki/") {
		wikiTitle, _ = url.PathUnescape(a.Wikipedia[strings.Index(a.Wikipedia, "/wiki/")+6:])
	}
	var sum *wikiSummary
	if wikiTitle != "" {
		sum, _ = c.wikipediaSummary(ctx, wikiTitle)
	} else {
		sum = c.wikipediaByName(ctx, info.Name)
	}
	if sum != nil && sum.Extract != "" {
		info.Bio, info.BioSource, info.Wikipedia = sum.Extract, sum.Page, sum.Page
		if info.PhotoURL == "" {
			info.PhotoURL = sum.Image
		}
		if full, err := c.wikipediaBiography(ctx, sum.Title); err == nil && len(full) > len(info.Bio)+200 {
			info.BioFull = full
		}
	}
	return info, nil
}

// bioSkip are article sections that are not the life story.
var bioSkip = regexp.MustCompile(`(?i)\b(works?|bibliography|writings|publications|books|novels|filmography|discography|themes?|style|legacy|reception|influence|criticism|critical|analysis|reputation|honou?rs|awards|prizes|recognition|adaptations?|in popular culture|selected|see also|references|notes|sources|citations|further reading|external links|footnotes|gallery|ideas|thought|teachings|philosophy|views|method|bibliograph)`)

const maxBio = 20000

// wikipediaBiography returns the article's introduction plus its life
// sections (early life, career, death, personal life …) as plain text, with
// section titles on their own lines as "## Title" ("### Title" below that).
func (c *Client) wikipediaBiography(ctx context.Context, title string) (string, error) {
	var r struct {
		Query struct {
			Pages []struct {
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}
	u := "https://en.wikipedia.org/w/api.php?action=query&prop=extracts&explaintext=1&exsectionformat=wiki&redirects=1&format=json&formatversion=2&titles=" + url.QueryEscape(title)
	if err := c.getJSON(ctx, u, &r); err != nil {
		return "", err
	}
	if len(r.Query.Pages) == 0 {
		return "", nil
	}
	return selectBiography(r.Query.Pages[0].Extract), nil
}

var headingRe = regexp.MustCompile(`^(={2,6})\s*(.*?)\s*={2,6}$`)

// selectBiography keeps the introduction and the top-level sections that are
// about the person's life, with their subsections; it stops at maxBio
// characters on a paragraph boundary.
func selectBiography(extract string) string {
	var out strings.Builder
	keep := true // the introduction
	for _, line := range strings.Split(extract, "\n") {
		line = strings.TrimSpace(line)
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level, name := len(m[1]), m[2]
			if level == 2 {
				keep = !bioSkip.MatchString(name)
			}
			if keep && name != "" {
				if level >= 4 && bioSkip.MatchString(name) {
					continue
				}
				prefix := "## "
				if level > 2 {
					prefix = "### "
				}
				out.WriteString("\n" + prefix + name + "\n")
			}
			continue
		}
		if !keep || line == "" {
			continue
		}
		if out.Len()+len(line) > maxBio {
			break
		}
		out.WriteString(tidyWiki(line) + "\n\n")
	}
	// Drop headings left without text: followed by nothing, or by a heading of
	// the same or a higher level.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var clean []string
	for i, l := range lines {
		if lvl := headingLevel(l); lvl > 0 {
			next := 0
			found := false
			for _, n := range lines[i+1:] {
				if strings.TrimSpace(n) == "" {
					continue
				}
				next, found = headingLevel(n), true
				break
			}
			if !found || (next > 0 && next <= lvl) {
				continue
			}
		}
		clean = append(clean, l)
	}
	return strings.TrimSpace(blank3.ReplaceAllString(strings.Join(clean, "\n"), "\n\n"))
}

// writerWords: a Wikipedia page found by name alone is only trusted when it
// describes someone who writes books, not a footballer of the same name.
var writerWords = regexp.MustCompile(`(?i)\b(author|writer|novelist|poet|playwright|essayist|journalist|philosopher|psychologist|psychiatrist|psychotherapist|historian|scientist|professor|translator|monk|teacher|scholar|physician|economist|engineer|programmer|blogger|entrepreneur|investor)\b`)

// wikipediaByName tries the English Wikipedia page titled exactly like the author.
func (c *Client) wikipediaByName(ctx context.Context, name string) *wikiSummary {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	sum, err := c.wikipediaSummary(ctx, name)
	if err != nil || sum.Extract == "" {
		return nil
	}
	if !writerWords.MatchString(sum.Description + " " + firstSentence(sum.Extract)) {
		return nil
	}
	return sum
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i]
	}
	return s
}

func (c *Client) wikidataEnwiki(ctx context.Context, qid string) (string, error) {
	var r struct {
		Entities map[string]struct {
			Sitelinks map[string]struct {
				Title string `json:"title"`
			} `json:"sitelinks"`
		} `json:"entities"`
	}
	u := "https://www.wikidata.org/w/api.php?action=wbgetentities&props=sitelinks&sitefilter=enwiki&format=json&ids=" + url.QueryEscape(qid)
	if err := c.getJSON(ctx, u, &r); err != nil {
		return "", err
	}
	return r.Entities[qid].Sitelinks["enwiki"].Title, nil
}

type wikiSummary struct{ Title, Extract, Image, Page, Description string }

func (c *Client) wikipediaSummary(ctx context.Context, title string) (*wikiSummary, error) {
	var r struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	u := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	if err := c.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	if r.Type == "disambiguation" {
		return &wikiSummary{}, nil
	}
	return &wikiSummary{Title: r.Title, Extract: strings.Join(strings.Fields(r.Extract), " "), Image: r.Thumbnail.Source, Page: r.ContentURLs.Desktop.Page, Description: r.Description}, nil
}

// junkWork filters study guides, summaries and box sets out of a bibliography.
var junkWork = regexp.MustCompile(`(?i)(\b(study guide|sparknotes|cliffsnotes|summary of|summary and analysis|workbook for|box set|boxed set|collection set|coles notes|york notes|bookrags|supersummary|complete works|collected works|selected works|works of|novels and stories|omnibus|anthology|best .*short stories|great short works|sochinenii|izbrannoe)\b|собрание сочинений|сочинения|избранное|polnoe sobranie)`)

type worksCacheEntry struct {
	at    time.Time
	works []Candidate
}

var (
	worksMu    sync.Mutex
	worksCache = map[string]worksCacheEntry{}
)

// AuthorWorks lists an author's works on Open Library, most published first;
// works with an edition in a preferred language come first and are shown as
// that edition. Results are cached for six hours.
func (c *Client) AuthorWorks(ctx context.Context, key string, limit int) ([]Candidate, error) {
	langs := c.langs()
	ck := key + "|" + strconv.Itoa(limit) + "|" + strings.Join(langs, ",")
	worksMu.Lock()
	if e, ok := worksCache[ck]; ok && time.Since(e.at) < 6*time.Hour {
		worksMu.Unlock()
		return e.works, nil
	}
	worksMu.Unlock()
	q := "author_key:" + key
	sortBy := url.Values{"sort": {"editions"}}
	var prefs map[string][]edition
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); prefs, _ = c.preferredEditions(ctx, q, sortBy, 150) }()
	v := url.Values{}
	v.Set("q", q)
	v.Set("sort", "editions")
	v.Set("limit", strconv.Itoa(limit))
	v.Set("fields", olFields)
	var res olSearch
	err := c.getJSON(ctx, olBase+"/search.json?"+v.Encode(), &res)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	var withPref, without []Candidate
	seen := map[string]bool{}
	for _, d := range res.Docs {
		// Barely catalogued records (one edition, no cover) are mostly noise.
		if d.Title == "" || junkWork.MatchString(d.Title) || (d.EditionCount <= 1 && d.CoverI == 0) {
			continue
		}
		cand := olCandidate(d, "")
		cand.Subjects = nil
		applyPreferred(&cand, prefs[d.Key])
		k, ko := workKey(cand.Title), workKey(cand.OriginalTitle)
		if seen[k] || (ko != "" && seen[ko]) { // a less published duplicate (results come most published first)
			continue
		}
		seen[k] = true
		if ko != "" {
			seen[ko] = true
		}
		if len(prefs[d.Key]) > 0 {
			withPref = append(withPref, cand)
		} else {
			without = append(without, cand)
		}
	}
	out := append(withPref, without...)
	worksMu.Lock()
	worksCache[ck] = worksCacheEntry{at: time.Now(), works: out}
	worksMu.Unlock()
	return out, nil
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// englishTitle tidies an edition title for display: drops translator notes
// after ";" and turns SHOUTED titles into Title Case. "" when it looks unusable.
func englishTitle(t string) string {
	if i := strings.Index(t, ";"); i > 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t), ".,:"))
	if t == "" || len(t) > 120 {
		return ""
	}
	if strings.ToUpper(t) == t {
		words := strings.Fields(strings.ToLower(t))
		for i, w := range words {
			if i == 0 || len(w) > 3 {
				words[i] = strings.ToUpper(w[:1]) + w[1:]
			}
		}
		t = strings.Join(words, " ")
	}
	return t
}

// workKey compares titles loosely: no subtitle, no leading article, no accents.
// "The Double: A St Petersburg Poem", "Double" → "double".
func workKey(t string) string {
	for _, sep := range []string{":", " (", "[", " - "} {
		if i := strings.Index(t, sep); i > 0 {
			t = t[:i]
		}
	}
	k := fold.String(t)
	for _, art := range []string{"the ", "a ", "an "} {
		k = strings.TrimPrefix(k, art)
	}
	return k
}

// headingLevel is 2 for "## x", 3 for "### x", 0 for text.
func headingLevel(l string) int {
	switch {
	case strings.HasPrefix(l, "### "):
		return 3
	case strings.HasPrefix(l, "## "):
		return 2
	}
	return 0
}

var (
	emptyParenLead = regexp.MustCompile(`\(\s*[;,]\s*`)
	emptyParens    = regexp.MustCompile(`\s*\(\s*\)`)
)

// tidyWiki removes what is left of stripped pronunciation guides:
// "Andy Weir ( ; born 1972)" → "Andy Weir (born 1972)".
func tidyWiki(s string) string {
	s = emptyParenLead.ReplaceAllString(s, "(")
	return emptyParens.ReplaceAllString(s, "")
}
