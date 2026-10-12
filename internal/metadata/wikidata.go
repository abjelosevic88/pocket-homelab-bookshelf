package metadata

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const wdAPI = "https://www.wikidata.org/w/api.php"

// WDWork is a book on Wikidata with the series it belongs to.
type WDWork struct {
	QID     string            `json:"qid"`
	Labels  map[string]string `json:"-"`
	Title   string            `json:"title"` // in the preferred language
	Year    string            `json:"year"`
	OLWorks []string          `json:"ol_works"` // "OL7924103W"
	Series  []WDSeriesRef     `json:"series"`
	PartOf  []string          `json:"-"` // P361: e.g. "Book Four" is part of "The Two Towers"
}

// WDSeriesRef is a work's place in one series.
type WDSeriesRef struct {
	QID     string `json:"qid"`
	Ordinal string `json:"ordinal"` // "1", "0", "2.5"; "" when Wikidata has none
}

// AuthorWikidataID returns the Wikidata item of an Open Library author.
func (c *Client) AuthorWikidataID(ctx context.Context, olKey string) (string, error) {
	return cached(c, "olq:"+olKey, 7*24*time.Hour, func(ctx context.Context) (string, error) {
		var a struct {
			RemoteIDs map[string]any `json:"remote_ids"`
		}
		if err := c.getJSON(ctx, olBase+"/authors/"+url.PathEscape(olKey)+".json", &a); err != nil {
			return "", err
		}
		q, _ := a.RemoteIDs["wikidata"].(string)
		return q, nil
	}, ctx)
}

// wdSearch returns item ids matching a CirrusSearch query ("haswbstatement:…").
func (c *Client) wdSearch(ctx context.Context, q string) ([]string, error) {
	var r struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	u := wdAPI + "?action=query&list=search&srlimit=100&format=json&srsearch=" + url.QueryEscape(q)
	if err := c.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	var out []string
	for _, s := range r.Query.Search {
		out = append(out, s.Title)
	}
	return out, nil
}

type wdEntity struct {
	Labels map[string]struct {
		Value string `json:"value"`
	} `json:"labels"`
	Claims map[string][]struct {
		Mainsnak struct {
			Datavalue struct {
				Value any `json:"value"`
			} `json:"datavalue"`
		} `json:"mainsnak"`
		Qualifiers map[string][]struct {
			Datavalue struct {
				Value any `json:"value"`
			} `json:"datavalue"`
		} `json:"qualifiers"`
	} `json:"claims"`
}

// wdEntities fetches items in batches of 50.
func (c *Client) wdEntities(ctx context.Context, ids []string, props string) (map[string]wdEntity, error) {
	out := map[string]wdEntity{}
	for i := 0; i < len(ids); i += 50 {
		j := min(len(ids), i+50)
		var r struct {
			Entities map[string]wdEntity `json:"entities"`
		}
		u := wdAPI + "?action=wbgetentities&format=json&props=" + url.QueryEscape(props) + "&ids=" + url.QueryEscape(strings.Join(ids[i:j], "|"))
		if err := c.getJSON(ctx, u, &r); err != nil {
			return out, err
		}
		for k, v := range r.Entities {
			out[k] = v
		}
	}
	return out, nil
}

func entityID(v any) string {
	if m, ok := v.(map[string]any); ok {
		if id, ok := m["id"].(string); ok {
			return id
		}
	}
	return ""
}

func (c *Client) preferredLabel(labels map[string]string) string {
	for _, l := range append(c.langs(), "en") {
		if v := labels[l]; v != "" {
			return v
		}
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return labels[keys[0]]
	}
	return ""
}

func (c *Client) toWorks(ents map[string]wdEntity, onlySeries string) []WDWork {
	var out []WDWork
	for qid, e := range ents {
		w := WDWork{QID: qid, Labels: map[string]string{}}
		for lang, l := range e.Labels {
			w.Labels[lang] = l.Value
		}
		w.Title = c.preferredLabel(w.Labels)
		for _, st := range e.Claims["P179"] {
			sid := entityID(st.Mainsnak.Datavalue.Value)
			if sid == "" || (onlySeries != "" && sid != onlySeries) {
				continue
			}
			ref := WDSeriesRef{QID: sid}
			for _, q := range st.Qualifiers["P1545"] {
				if s, ok := q.Datavalue.Value.(string); ok {
					ref.Ordinal = s
				}
			}
			w.Series = append(w.Series, ref)
		}
		for _, st := range e.Claims["P648"] {
			if s, ok := st.Mainsnak.Datavalue.Value.(string); ok && strings.HasSuffix(s, "W") {
				w.OLWorks = append(w.OLWorks, s)
			}
		}
		for _, st := range e.Claims["P361"] {
			if id := entityID(st.Mainsnak.Datavalue.Value); id != "" {
				w.PartOf = append(w.PartOf, id)
			}
		}
		for _, st := range e.Claims["P577"] {
			if m, ok := st.Mainsnak.Datavalue.Value.(map[string]any); ok {
				if t, ok := m["time"].(string); ok && len(t) >= 5 {
					y := strings.TrimLeft(t[:5], "+")
					if w.Year == "" || y < w.Year {
						w.Year = y
					}
				}
			}
		}
		if len(w.Series) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// SeriesWorksByAuthor lists an author's works that Wikidata places in a series.
func (c *Client) SeriesWorksByAuthor(ctx context.Context, authorQID string) ([]WDWork, error) {
	ids, err := c.wdSearch(ctx, "haswbstatement:P50="+authorQID+" haswbstatement:P179")
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	ents, err := c.wdEntities(ctx, ids, "labels|claims")
	if err != nil {
		return nil, err
	}
	return c.toWorks(ents, ""), nil
}

// SeriesMembers lists the works of a series, in order. Kept in the cache,
// refreshed in the background once a day.
func (c *Client) SeriesMembers(ctx context.Context, seriesQID string) ([]WDWork, error) {
	ck := "members:" + seriesQID + ":" + strings.Join(c.langs(), ",")
	return cached(c, ck, 24*time.Hour, func(ctx context.Context) ([]WDWork, error) { return c.seriesMembers(ctx, seriesQID) }, ctx)
}

func (c *Client) seriesMembers(ctx context.Context, seriesQID string) ([]WDWork, error) {
	ids, err := c.wdSearch(ctx, "haswbstatement:P179="+seriesQID)
	if err != nil {
		return nil, err
	}
	ents, err := c.wdEntities(ctx, ids, "labels|claims")
	if err != nil {
		return nil, err
	}
	works := c.toWorks(ents, seriesQID)
	// Drop entries that are part of another entry (The Lord of the Rings lists
	// both its three volumes and the six "books" inside them).
	member := map[string]bool{}
	for _, w := range works {
		member[w.QID] = true
	}
	kept := works[:0]
	for _, w := range works {
		inner := false
		for _, p := range w.PartOf {
			if member[p] {
				inner = true
			}
		}
		if !inner {
			kept = append(kept, w)
		}
	}
	works = kept
	// By number; when Wikidata numbers fewer than half of the entries, by year
	// (Space Odyssey: only "2001" carries a number).
	numbered := 0
	for _, w := range works {
		if _, ok := OrdinalValue(w.Series[0].Ordinal); ok {
			numbered++
		}
	}
	byYear := numbered*2 < len(works)
	sort.SliceStable(works, func(i, j int) bool {
		a, b := works[i], works[j]
		if !byYear {
			_, oka := OrdinalValue(a.Series[0].Ordinal)
			_, okb := OrdinalValue(b.Series[0].Ordinal)
			if oka || okb {
				return ordinalLess(a.Series[0].Ordinal, b.Series[0].Ordinal)
			}
		}
		if a.Year != b.Year {
			return a.Year != "" && (b.Year == "" || a.Year < b.Year)
		}
		return a.Title < b.Title
	})
	return works, nil
}

// SeriesInfo is a series' name in the preferred language and whether it is
// a series of an author's own books (not a publisher's collection, a shared
// universe or a mis-tagged topic).
type SeriesInfo struct {
	Name  string
	Valid bool
}

// literary series types that count even without an author on the item:
// novel series, literary trilogy/tetralogy/pentalogy/hexalogy, literary cycle.
var seriesTypes = map[string]bool{"Q1667921": true, "Q1371849": true, "Q2367179": true, "Q20540133": true, "Q105420": true, "Q1080479": true, "Q3297186": true}

// SeriesNames looks series items up.
func (c *Client) SeriesNames(ctx context.Context, qids []string) (map[string]SeriesInfo, error) {
	ents, err := c.wdEntities(ctx, qids, "labels|claims")
	out := map[string]SeriesInfo{}
	for q, e := range ents {
		labels := map[string]string{}
		for lang, l := range e.Labels {
			labels[lang] = l.Value
		}
		valid := len(e.Claims["P50"]) > 0
		for _, st := range e.Claims["P31"] {
			if seriesTypes[entityID(st.Mainsnak.Datavalue.Value)] {
				valid = true
			}
		}
		out[q] = SeriesInfo{Name: c.preferredLabel(labels), Valid: valid}
	}
	return out, err
}

// OrdinalValue parses "2", "2.5", "0"; ok=false for "" or text.
func OrdinalValue(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f, err == nil
}

func ordinalLess(a, b string) bool {
	fa, oka := OrdinalValue(a)
	fb, okb := OrdinalValue(b)
	switch {
	case oka && okb:
		return fa < fb
	case oka:
		return true
	case okb:
		return false
	}
	return a < b
}

var cyr = strings.NewReplacer(
	"Љ", "Lj", "Њ", "Nj", "Џ", "Dž", "љ", "lj", "њ", "nj", "џ", "dž", "А", "A", "Б", "B", "В", "V", "Г", "G", "Д", "D", "Ђ", "Đ", "Е", "E", "Ж", "Ž",
	"З", "Z", "И", "I", "Ј", "J", "К", "K", "Л", "L", "М", "M", "Н", "N", "О", "O", "П", "P", "Р", "R", "С", "S", "Т", "T", "Ћ", "Ć", "У", "U",
	"Ф", "F", "Х", "H", "Ц", "C", "Ч", "Č", "Ш", "Š", "а", "a", "б", "b", "в", "v", "г", "g", "д", "d", "ђ", "đ", "е", "e", "ж", "ž", "з", "z",
	"и", "i", "ј", "j", "к", "k", "л", "l", "м", "m", "н", "n", "о", "o", "п", "p", "р", "r", "с", "s", "т", "t", "ћ", "ć", "у", "u", "ф", "f",
	"х", "h", "ц", "c", "ч", "č", "ш", "š",
)

// Latin transliterates Serbian Cyrillic ("Велики лов" → "Veliki lov").
func Latin(s string) string { return cyr.Replace(s) }

// OpenLibraryWorkTitle returns a work's title (cached), for matching.
func (c *Client) OpenLibraryWorkTitle(ctx context.Context, key string) string {
	t, _ := cached(c, "olw:"+key, 30*24*time.Hour, func(ctx context.Context) (string, error) {
		var w struct {
			Title string `json:"title"`
		}
		err := c.getJSON(ctx, olBase+key+".json", &w)
		return w.Title, err
	}, ctx)
	return t
}

// WorkCover finds a cover for a series entry: from its Open Library work, or
// by searching title and author when Wikidata has no Open Library link.
// Returns the work key ("/works/OL…W", "" if unknown) and a cover id (0 = none).
func (c *Client) WorkCover(ctx context.Context, olWork, title, author string) (string, int) {
	type result struct {
		Key   string `json:"key"`
		Cover int    `json:"cover"`
	}
	key := "cover:" + olWork
	if olWork == "" {
		key = "cover:" + title + "|" + author
	}
	r, _ := cached(c, key, 30*24*time.Hour, func(ctx context.Context) (result, error) {
		res := result{Key: olWork}
		var err error
		if olWork != "" {
			var w struct {
				Covers []int `json:"covers"`
			}
			if err = c.getJSON(ctx, olBase+olWork+".json", &w); err == nil {
				for _, id := range w.Covers {
					if id > 0 {
						res.Cover = id
						break
					}
				}
			}
		}
		if res.Cover == 0 && title != "" {
			v := url.Values{"title": {title}, "author": {author}, "limit": {"1"}, "fields": {"key,cover_i"}}
			var sr struct {
				Docs []struct {
					Key    string `json:"key"`
					CoverI int    `json:"cover_i"`
				} `json:"docs"`
			}
			if err = c.getJSON(ctx, olBase+"/search.json?"+v.Encode(), &sr); err == nil && len(sr.Docs) > 0 {
				if res.Key == "" {
					res.Key = sr.Docs[0].Key
				}
				res.Cover = sr.Docs[0].CoverI
			}
		}
		return res, err // a failed lookup is tried again next time
	}, ctx)
	return r.Key, r.Cover
}
