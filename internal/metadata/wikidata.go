package metadata

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
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
}

// WDSeriesRef is a work's place in one series.
type WDSeriesRef struct {
	QID     string `json:"qid"`
	Ordinal string `json:"ordinal"` // "1", "0", "2.5"; "" when Wikidata has none
}

type wdCache struct {
	mu   sync.Mutex
	at   map[string]time.Time
	data map[string]any
}

var wdc = wdCache{at: map[string]time.Time{}, data: map[string]any{}}

func (w *wdCache) get(k string, ttl time.Duration) (any, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.at[k]; ok && time.Since(t) < ttl {
		return w.data[k], true
	}
	return nil, false
}

func (w *wdCache) put(k string, v any) {
	w.mu.Lock()
	w.at[k], w.data[k] = time.Now(), v
	w.mu.Unlock()
}

// AuthorWikidataID returns the Wikidata item of an Open Library author.
func (c *Client) AuthorWikidataID(ctx context.Context, olKey string) (string, error) {
	if v, ok := wdc.get("olq:"+olKey, 24*time.Hour); ok {
		return v.(string), nil
	}
	var a struct {
		RemoteIDs map[string]any `json:"remote_ids"`
	}
	if err := c.getJSON(ctx, olBase+"/authors/"+url.PathEscape(olKey)+".json", &a); err != nil {
		return "", err
	}
	q, _ := a.RemoteIDs["wikidata"].(string)
	wdc.put("olq:"+olKey, q)
	return q, nil
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

// SeriesMembers lists the works of a series, in order (cached for a day).
func (c *Client) SeriesMembers(ctx context.Context, seriesQID string) ([]WDWork, error) {
	ck := "members:" + seriesQID + ":" + strings.Join(c.langs(), ",")
	if v, ok := wdc.get(ck, 24*time.Hour); ok {
		return v.([]WDWork), nil
	}
	ids, err := c.wdSearch(ctx, "haswbstatement:P179="+seriesQID)
	if err != nil {
		return nil, err
	}
	ents, err := c.wdEntities(ctx, ids, "labels|claims")
	if err != nil {
		return nil, err
	}
	works := c.toWorks(ents, seriesQID)
	sort.SliceStable(works, func(i, j int) bool { return ordinalLess(works[i].Series[0].Ordinal, works[j].Series[0].Ordinal) })
	wdc.put(ck, works)
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
	if v, ok := wdc.get("olw:"+key, 7*24*time.Hour); ok {
		return v.(string)
	}
	var w struct {
		Title string `json:"title"`
	}
	if c.getJSON(ctx, olBase+key+".json", &w) != nil {
		return ""
	}
	wdc.put("olw:"+key, w.Title)
	return w.Title
}
