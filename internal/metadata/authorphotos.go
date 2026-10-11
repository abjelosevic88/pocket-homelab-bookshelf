package metadata

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

func commonsFile(name string, width int) string {
	name = strings.TrimPrefix(strings.TrimPrefix(name, "File:"), "Image:")
	return fmt.Sprintf("https://commons.wikimedia.org/wiki/Special:FilePath/%s?width=%d", url.PathEscape(strings.ReplaceAll(name, " ", "_")), width)
}

// AuthorPhotoSuggestions collects pictures of an author: their Open Library
// photos, the Wikidata portrait(s), images from their Wikipedia article and
// photos on other Open Library records with the same name. Portraits first.
func (c *Client) AuthorPhotoSuggestions(ctx context.Context, olKey, name, wikipedia string) []CoverOption {
	var mu sync.Mutex
	groups := make([][]CoverOption, 4)
	add := func(g int, o CoverOption) { mu.Lock(); groups[g] = append(groups[g], o); mu.Unlock() }
	var wg sync.WaitGroup

	if olKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var a struct {
				Photos    []int          `json:"photos"`
				RemoteIDs map[string]any `json:"remote_ids"`
			}
			if c.getJSON(ctx, olBase+"/authors/"+url.PathEscape(olKey)+".json", &a) != nil {
				return
			}
			for _, p := range a.Photos {
				if p > 0 {
					add(0, CoverOption{URL: fmt.Sprintf("https://covers.openlibrary.org/a/id/%d-L.jpg", p), Thumb: fmt.Sprintf("https://covers.openlibrary.org/a/id/%d-M.jpg", p), Source: "openlibrary", Label: "Open Library"})
				}
			}
			if qid, _ := a.RemoteIDs["wikidata"].(string); qid != "" {
				var r struct {
					Claims map[string][]struct {
						Mainsnak struct {
							Datavalue struct {
								Value any `json:"value"`
							} `json:"datavalue"`
						} `json:"mainsnak"`
					} `json:"claims"`
				}
				if c.getJSON(ctx, "https://www.wikidata.org/w/api.php?action=wbgetclaims&property=P18&format=json&entity="+url.QueryEscape(qid), &r) == nil {
					for _, cl := range r.Claims["P18"] {
						if f, ok := cl.Mainsnak.Datavalue.Value.(string); ok {
							add(1, CoverOption{URL: commonsFile(f, 800), Thumb: commonsFile(f, 240), Source: "wikidata", Label: "Wikidata portrait"})
						}
					}
				}
			}
		}()
	}
	if title := wikiTitleFromURL(wikipedia); title != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var r struct {
				Query struct {
					Pages []struct {
						Images []struct {
							Title string `json:"title"`
						} `json:"images"`
					} `json:"pages"`
				} `json:"query"`
			}
			u := "https://en.wikipedia.org/w/api.php?action=query&prop=images&imlimit=60&format=json&formatversion=2&titles=" + url.QueryEscape(title)
			if c.getJSON(ctx, u, &r) != nil || len(r.Query.Pages) == 0 {
				return
			}
			n := 0
			for _, im := range r.Query.Pages[0].Images {
				low := strings.ToLower(im.Title)
				// Photos and paintings only: no icons, logos, maps or signatures.
				if !(strings.HasSuffix(low, ".jpg") || strings.HasSuffix(low, ".jpeg")) || strings.Contains(low, "signature") || strings.Contains(low, "logo") {
					continue
				}
				add(2, CoverOption{URL: commonsFile(im.Title, 800), Thumb: commonsFile(im.Title, 240), Source: "wikipedia", Label: "From the Wikipedia article"})
				if n++; n == 16 {
					break
				}
			}
		}()
	}
	if name != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found, err := c.SearchAuthors(ctx, name, 6)
			if err != nil {
				return
			}
			n := 0
			for _, a := range found {
				if a.Key == olKey || !namesOverlap(name, a.AllNames()) {
					continue
				}
				var rec struct {
					Photos []int `json:"photos"`
				}
				if c.getJSON(ctx, olBase+"/authors/"+url.PathEscape(a.Key)+".json", &rec) != nil {
					continue
				}
				for _, p := range rec.Photos {
					if p > 0 {
						add(3, CoverOption{URL: fmt.Sprintf("https://covers.openlibrary.org/a/id/%d-L.jpg", p), Thumb: fmt.Sprintf("https://covers.openlibrary.org/a/id/%d-M.jpg", p), Source: "openlibrary", Label: "Open Library (other record)"})
					}
				}
				if n++; n == 3 {
					break
				}
			}
		}()
	}
	wg.Wait()
	var out []CoverOption
	seen := map[string]bool{}
	for _, g := range groups {
		for _, o := range g {
			if !seen[o.URL] {
				seen[o.URL] = true
				out = append(out, o)
			}
		}
	}
	return out
}

func wikiTitleFromURL(u string) string {
	i := strings.Index(u, "/wiki/")
	if !strings.Contains(u, "wikipedia.org") || i < 0 {
		return ""
	}
	t, err := url.PathUnescape(u[i+6:])
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(t, "_", " ")
}

// namesOverlap: same surname, and the first letters of the first names agree.
func namesOverlap(want string, names []string) bool {
	w := strings.Fields(strings.ToLower(want))
	if len(w) == 0 {
		return false
	}
	for _, n := range names {
		f := strings.Fields(strings.ToLower(n))
		if len(f) > 0 && f[len(f)-1] == w[len(w)-1] && f[0][:1] == w[0][:1] {
			return true
		}
	}
	return false
}
