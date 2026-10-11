package metadata

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// marc maps ISO 639-1 codes (what settings store) to the MARC codes Open
// Library uses for edition languages.
var marc = map[string]string{
	"en": "eng", "sr": "srp", "hr": "hrv", "bs": "bos", "sl": "slv", "mk": "mac", "de": "ger", "fr": "fre", "es": "spa",
	"it": "ita", "pt": "por", "ru": "rus", "hu": "hun", "pl": "pol", "cs": "cze", "sk": "slo", "nl": "dut", "sv": "swe",
	"no": "nor", "da": "dan", "fi": "fin", "ro": "rum", "bg": "bul", "el": "gre", "tr": "tur", "uk": "ukr", "ja": "jpn",
	"zh": "chi", "ko": "kor", "ar": "ara", "he": "heb",
}

// MARCCode returns the Open Library language code for an ISO 639-1 code.
func MARCCode(iso string) string { return marc[strings.ToLower(iso)] }

// iso639 converts back: "srp" → "sr".
func iso639(m string) string {
	for k, v := range marc {
		if v == m {
			return k
		}
	}
	return m
}

func (c *Client) langs() []string {
	if c.Langs == nil {
		return nil
	}
	var out []string
	for _, l := range c.Langs() {
		if MARCCode(l) != "" {
			out = append(out, l)
		}
		if len(out) == 3 { // each preferred language costs one extra catalogue query
			break
		}
	}
	return out
}

// edition is the version of a work in one language.
type edition struct {
	title, lang, isbn13, isbn10 string
	cover                       int
}

type olEditionDocs struct {
	Docs []struct {
		Title    string   `json:"title"`
		Language []string `json:"language"`
		CoverI   int      `json:"cover_i"`
		ISBN     []string `json:"isbn"`
	} `json:"docs"`
}

// preferredEditions asks Open Library, once per preferred language, which
// works matching q have an edition in that language, and returns them per
// work key in preference order, plus the works found only that way.
func (c *Client) preferredEditions(ctx context.Context, q string, extra url.Values, limit int) (map[string][]edition, []olDoc) {
	langs := c.langs()
	type res struct {
		i    int
		docs []struct {
			olDoc
			Editions olEditionDocs `json:"editions"`
		}
	}
	results := make([]res, len(langs))
	var wg sync.WaitGroup
	for i, l := range langs {
		wg.Add(1)
		go func(i int, l string) {
			defer wg.Done()
			v := url.Values{}
			for k, vv := range extra {
				v[k] = vv
			}
			v.Set("q", q+" language:"+MARCCode(l))
			v.Set("limit", strconv.Itoa(limit))
			v.Set("fields", olFields+",editions,editions.title,editions.language,editions.cover_i,editions.isbn")
			var r struct {
				Docs []struct {
					olDoc
					Editions olEditionDocs `json:"editions"`
				} `json:"docs"`
			}
			if err := c.getJSON(ctx, olBase+"/search.json?"+v.Encode(), &r); err == nil {
				results[i] = res{i: i, docs: r.Docs}
			}
		}(i, l)
	}
	wg.Wait()
	byWork := map[string][]edition{}
	var found []olDoc
	seen := map[string]bool{}
	for i, r := range results {
		for _, d := range r.docs {
			if len(d.Editions.Docs) == 0 {
				continue
			}
			e := d.Editions.Docs[0]
			ed := edition{title: e.Title, lang: langs[i], cover: e.CoverI}
			ed.isbn13, ed.isbn10 = pickISBNs(e.ISBN, "")
			byWork[d.Key] = append(byWork[d.Key], ed)
			if !seen[d.Key] {
				seen[d.Key] = true
				found = append(found, d.olDoc)
			}
		}
	}
	return byWork, found
}

// applyPreferred shows a work as its edition in a preferred language.
func applyPreferred(c *Candidate, eds []edition) {
	if len(eds) == 0 {
		return
	}
	e := eds[0]
	c.Publisher = "" // the work's first publisher belongs to some other edition
	if t := englishTitle(e.title); t != "" && !strings.EqualFold(t, c.Title) && !junkWork.MatchString(t) {
		c.OriginalTitle, c.Title = c.Title, t
	}
	if e.cover > 0 {
		c.CoverURL, c.ThumbURL = olCover(e.cover, "L"), olCover(e.cover, "M")
	}
	if e.isbn13 != "" {
		c.ISBN13, c.ISBN10 = e.isbn13, e.isbn10
	}
	c.Language = e.lang
}

// rankByLanguage puts results available in a preferred language first,
// keeping the catalogue's order otherwise.
func rankByLanguage(cands []Candidate, langs []string) {
	rank := func(l string) int {
		for i, p := range langs {
			if strings.EqualFold(p, l) || MARCCode(p) == l {
				return i
			}
		}
		return len(langs)
	}
	sort.SliceStable(cands, func(i, j int) bool { return rank(cands[i].Language) < rank(cands[j].Language) })
}
