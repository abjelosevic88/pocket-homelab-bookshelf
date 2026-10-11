// Package metadata looks books up in free public catalogues — Open Library
// (no key) and Google Books (free API key) — and downloads their covers.
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Candidate is one search result, normalised across sources.
type Candidate struct {
	Source      string   `json:"source"`    // openlibrary | google
	SourceID    string   `json:"source_id"` // "/works/OL45804W" or a Google volume id
	SourceURL   string   `json:"source_url"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Authors     []string `json:"authors"`
	Published   string   `json:"published"`
	Pages       int      `json:"pages"`
	ISBN13      string   `json:"isbn13"`
	ISBN10      string   `json:"isbn10"`
	Publisher   string   `json:"publisher"`
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Subjects    []string `json:"subjects"`
	CoverURL    string   `json:"cover_url"` // large, for saving
	ThumbURL    string   `json:"thumb_url"` // small, for the result list
}

// Client talks to the catalogues.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// GoogleKey returns the current Google Books API key ("" = Google disabled).
	GoogleKey func() string
}

// New returns a client with the given timeout.
func New(userAgent string, timeout time.Duration, googleKey func() string) *Client {
	if googleKey == nil {
		googleKey = func() string { return "" }
	}
	return &Client{HTTP: &http.Client{Timeout: timeout}, UserAgent: userAgent, GoogleKey: googleKey}
}

// ErrNoGoogleKey is returned when Google Books is asked for without a key.
var ErrNoGoogleKey = errors.New("Google Books needs an API key (Settings → Metadata)")

// Sources lists which sources are usable right now.
func (c *Client) Sources() []string {
	out := []string{"openlibrary"}
	if c.GoogleKey() != "" {
		out = append(out, "google")
	}
	return out
}

func (c *Client) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		msg := strings.TrimSpace(string(body))
		var ge struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &ge) == nil && ge.Error.Message != "" {
			msg = ge.Error.Message
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, msg)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(v)
}

var nonDigit = regexp.MustCompile(`[^0-9Xx]`)

// NormalizeISBN strips separators and returns a 10/13 character ISBN, or "".
func NormalizeISBN(s string) string {
	d := strings.ToUpper(nonDigit.ReplaceAllString(s, ""))
	if len(d) == 13 && !strings.Contains(d, "X") {
		return d
	}
	if len(d) == 10 && !strings.Contains(d[:9], "X") {
		return d
	}
	return ""
}

// LooksLikeISBN reports whether a search query is an ISBN rather than words.
func LooksLikeISBN(q string) bool {
	q = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(q)), "isbn"))
	q = strings.TrimLeft(q, ": ")
	return NormalizeISBN(q) != "" && !strings.ContainsAny(strings.ToLower(q), "abcdefghijklmnopqrstuvwyz")
}

// ISBN10to13 converts an ISBN-10 to ISBN-13.
func ISBN10to13(isbn10 string) string {
	if len(isbn10) != 10 {
		return ""
	}
	core := "978" + isbn10[:9]
	sum := 0
	for i, r := range core {
		n := int(r - '0')
		if i%2 == 1 {
			n *= 3
		}
		sum += n
	}
	return core + string(rune('0'+(10-sum%10)%10))
}

// Search queries the requested source ("all", "openlibrary" or "google").
// With "all", sources run in parallel and a failing one only adds a warning.
func (c *Client) Search(ctx context.Context, query, source string, limit int) ([]Candidate, []string, error) {
	query = strings.TrimSpace(query)
	// Open Library answers shorter queries with HTTP 422; nothing useful matches them anyway.
	if len([]rune(query)) < 3 {
		return []Candidate{}, nil, nil
	}
	if limit <= 0 || limit > 40 {
		limit = 12
	}
	switch source {
	case "openlibrary":
		r, err := c.SearchOpenLibrary(ctx, query, limit)
		return nonNil(r), nil, err
	case "google":
		r, err := c.SearchGoogle(ctx, query, limit)
		return nonNil(r), nil, err
	}
	var wg sync.WaitGroup
	var ol, gb []Candidate
	var olErr, gbErr error
	wg.Add(1)
	go func() { defer wg.Done(); ol, olErr = c.SearchOpenLibrary(ctx, query, limit) }()
	useGoogle := c.GoogleKey() != ""
	if useGoogle {
		wg.Add(1)
		go func() { defer wg.Done(); gb, gbErr = c.SearchGoogle(ctx, query, limit) }()
	}
	wg.Wait()
	var warnings []string
	if olErr != nil {
		warnings = append(warnings, "Open Library: "+olErr.Error())
	}
	if gbErr != nil {
		warnings = append(warnings, "Google Books: "+gbErr.Error())
	}
	if olErr != nil && (!useGoogle || gbErr != nil) {
		return []Candidate{}, warnings, olErr
	}
	return Merge(ol, gb, limit*2), warnings, nil
}

func nonNil(c []Candidate) []Candidate {
	if c == nil {
		return []Candidate{}
	}
	return c
}

// Merge interleaves two result lists, dropping the second copy of a book
// both sources know (same ISBN-13) after borrowing its missing fields.
func Merge(a, b []Candidate, limit int) []Candidate {
	out := []Candidate{}
	byISBN := map[string]int{}
	add := func(c Candidate) {
		if c.ISBN13 != "" {
			if i, ok := byISBN[c.ISBN13]; ok {
				fillMissing(&out[i], c)
				return
			}
			byISBN[c.ISBN13] = len(out)
		}
		out = append(out, c)
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		if i < len(a) {
			add(a[i])
		}
		if i < len(b) {
			add(b[i])
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func fillMissing(dst *Candidate, src Candidate) {
	if dst.Subtitle == "" {
		dst.Subtitle = src.Subtitle
	}
	if dst.Pages == 0 {
		dst.Pages = src.Pages
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	if dst.Publisher == "" {
		dst.Publisher = src.Publisher
	}
	if dst.Language == "" {
		dst.Language = src.Language
	}
	if dst.CoverURL == "" {
		dst.CoverURL, dst.ThumbURL = src.CoverURL, src.ThumbURL
	}
	if dst.ISBN10 == "" {
		dst.ISBN10 = src.ISBN10
	}
	if len(dst.Subjects) == 0 {
		dst.Subjects = src.Subjects
	}
}

// Details fetches the full record of one result (description, subjects,
// a bigger cover) — the search endpoints leave those out.
func (c *Client) Details(ctx context.Context, source, id string) (*Candidate, error) {
	switch source {
	case "openlibrary":
		return c.openLibraryWork(ctx, id)
	case "google":
		return c.googleVolume(ctx, id)
	}
	return nil, fmt.Errorf("unknown source %q", source)
}

var (
	tagBreak = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li)\s*/?>`)
	tagAny   = regexp.MustCompile(`<[^>]*>`)
	blank3   = regexp.MustCompile(`\n{3,}`)
)

// PlainText turns the light HTML some catalogues return into plain text.
func PlainText(s string) string {
	s = tagBreak.ReplaceAllString(s, "\n")
	s = tagAny.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = blank3.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func firstN(ss []string, n int) []string {
	if len(ss) > n {
		return ss[:n]
	}
	return ss
}
