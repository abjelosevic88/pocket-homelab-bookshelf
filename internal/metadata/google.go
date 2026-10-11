package metadata

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

const gbBase = "https://www.googleapis.com/books/v1/volumes"

type gbVolume struct {
	ID         string `json:"id"`
	VolumeInfo struct {
		Title               string   `json:"title"`
		Subtitle            string   `json:"subtitle"`
		Authors             []string `json:"authors"`
		Publisher           string   `json:"publisher"`
		PublishedDate       string   `json:"publishedDate"`
		Description         string   `json:"description"`
		PageCount           int      `json:"pageCount"`
		Categories          []string `json:"categories"`
		Language            string   `json:"language"`
		CanonicalVolumeLink string   `json:"canonicalVolumeLink"`
		InfoLink            string   `json:"infoLink"`
		IndustryIdentifiers []struct {
			Type       string `json:"type"`
			Identifier string `json:"identifier"`
		} `json:"industryIdentifiers"`
		ImageLinks map[string]string `json:"imageLinks"`
	} `json:"volumeInfo"`
}

type gbSearch struct {
	Items []gbVolume `json:"items"`
}

// gbImage makes a Google Books image link usable: https, no page-curl effect.
func gbImage(u string) string {
	if u == "" {
		return ""
	}
	u = strings.Replace(u, "http://", "https://", 1)
	return strings.Replace(u, "&edge=curl", "", 1)
}

func gbCandidate(v gbVolume) Candidate {
	vi := v.VolumeInfo
	c := Candidate{
		Source:      "google",
		SourceID:    v.ID,
		SourceURL:   vi.CanonicalVolumeLink,
		Title:       vi.Title,
		Subtitle:    vi.Subtitle,
		Authors:     firstN(vi.Authors, 4),
		Publisher:   vi.Publisher,
		Pages:       vi.PageCount,
		Language:    vi.Language,
		Description: PlainText(vi.Description),
	}
	if c.SourceURL == "" {
		c.SourceURL = vi.InfoLink
	}
	if len(vi.PublishedDate) >= 4 {
		c.Published = vi.PublishedDate[:4]
	}
	for _, id := range vi.IndustryIdentifiers {
		switch id.Type {
		case "ISBN_13":
			c.ISBN13 = id.Identifier
		case "ISBN_10":
			c.ISBN10 = id.Identifier
		}
	}
	if c.ISBN13 == "" && c.ISBN10 != "" {
		c.ISBN13 = ISBN10to13(c.ISBN10)
	}
	// "Fiction / Science Fiction / General" → "Science Fiction"
	var subj []string
	for _, cat := range vi.Categories {
		parts := strings.Split(cat, "/")
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" && p != "General" {
				subj = append(subj, p)
			}
		}
	}
	c.Subjects = cleanSubjects(subj)
	il := vi.ImageLinks
	c.ThumbURL = gbImage(il["thumbnail"])
	for _, k := range []string{"large", "medium", "small", "thumbnail"} {
		if il[k] != "" {
			c.CoverURL = gbImage(il[k])
			break
		}
	}
	return c
}

// SearchGoogle searches Google Books by words or ISBN.
func (c *Client) SearchGoogle(ctx context.Context, query string, limit int) ([]Candidate, error) {
	key := c.GoogleKey()
	if key == "" {
		return nil, ErrNoGoogleKey
	}
	q := query
	if LooksLikeISBN(query) {
		q = "isbn:" + NormalizeISBN(strings.TrimLeft(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "isbn"), ": "))
	}
	v := url.Values{}
	v.Set("q", q)
	v.Set("maxResults", strconv.Itoa(min(limit, 40)))
	v.Set("printType", "books")
	v.Set("key", key)
	v.Set("country", c.googleCountry())
	var res gbSearch
	if err := c.getJSON(ctx, gbBase+"?"+v.Encode(), &res); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, gbCandidate(it))
	}
	rankByLanguage(out, c.langs())
	return out, nil
}

func (c *Client) googleVolume(ctx context.Context, id string) (*Candidate, error) {
	key := c.GoogleKey()
	if key == "" {
		return nil, ErrNoGoogleKey
	}
	var v gbVolume
	if err := c.getJSON(ctx, gbBase+"/"+url.PathEscape(id)+"?key="+url.QueryEscape(key)+"&country="+c.googleCountry(), &v); err != nil {
		return nil, err
	}
	cand := gbCandidate(v)
	return &cand, nil
}

// googleCountry is sent with every Google Books request: Google decides what
// it may show by country, and when it cannot place the server's IP address
// it answers 503 "backendFailed" instead of saying so.
func (c *Client) googleCountry() string {
	if c.Country != "" {
		return c.Country
	}
	return "US"
}
