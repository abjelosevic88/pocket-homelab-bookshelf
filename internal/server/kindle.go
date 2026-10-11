package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"unicode"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/kindle"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Importing Kindle highlights is two calls: preview parses "My Clippings.txt"
// and proposes a shelf book for every Kindle book; import then writes the
// highlights where the user decided, creating books where asked.

var foldReplacer = strings.NewReplacer(
	"č", "c", "ć", "c", "ž", "z", "š", "s", "đ", "dj", "á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "ě", "e", "í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o",
	"ö", "o", "õ", "o", "ő", "o", "ø", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ű", "u", "ů", "u", "ñ", "n", "ń", "n",
	"ň", "n", "ý", "y", "ç", "c", "ł", "l", "ľ", "l", "ś", "s", "ź", "z", "ż", "z", "ř", "r", "ť", "t", "ď", "d", "ß", "ss",
	"&", " and ",
)

// fold lower-cases, strips accents and punctuation: "Moć sadašnjeg trenutka!" → "moc sadasnjeg trenutka".
func fold(s string) string {
	s = foldReplacer.Replace(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case !space && b.Len() > 0:
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// shortTitle drops subtitles and series: "Sapiens: A Brief History…" → "sapiens".
func shortTitle(s string) string {
	for _, sep := range []string{":", " - ", " – ", " (", "[", "/"} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return fold(s)
}

func sameTitle(a, b string) bool {
	fa, fb := fold(a), fold(b)
	if fa == "" || fb == "" {
		return false
	}
	return fa == fb || shortTitle(a) == shortTitle(b) || shortTitle(a) == fb || fa == shortTitle(b)
}

// lastName is the folded surname of the first author.
func lastName(authors string) string {
	first := strings.Split(authors, ",")[0]
	f := strings.Fields(fold(first))
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

func authorMatches(want, have string) bool {
	ln := lastName(want)
	return ln == "" || strings.Contains(fold(have), ln)
}

type kindleMatch struct {
	BookID int64  `json:"book_id"`
	Title  string `json:"title"`
	By     string `json:"by"` // title | openlibrary
}

type kindlePreview struct {
	kindle.Book
	Highlights int                 `json:"highlights"`
	Notes      int                 `json:"notes"`
	Known      int                 `json:"known"` // already saved in the matched book
	Match      *kindleMatch        `json:"match"`
	Candidate  *metadata.Candidate `json:"candidate"` // catalogue record, for creating the book
}

func (s *Server) handleKindlePreview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var src io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "malformed upload")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "form field 'file' is required")
			return
		}
		defer f.Close()
		src = f
	}
	clips, err := kindle.Parse(src)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the file: "+err.Error())
		return
	}
	books := kindle.Group(clips)
	if len(books) == 0 {
		writeErr(w, http.StatusBadRequest, "no highlights found — is this the “My Clippings.txt” file from the Kindle's documents folder?")
		return
	}
	shelf, err := s.store.ListBooks(r.Context(), store.BookFilter{UserID: me(r)})
	if err != nil {
		s.fail(w, err, "list books")
		return
	}
	out := make([]kindlePreview, len(books))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // be gentle with Open Library
	for i, kb := range books {
		p := &out[i]
		p.Book = kb
		for _, e := range kb.Entries {
			if e.NoteOnly {
				p.Notes++
			} else {
				p.Highlights++
				if e.Note != "" {
					p.Notes++
				}
			}
		}
		if m := matchByTitle(kb, shelf); m != nil {
			p.Match = m
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p.Candidate = s.lookupKindleBook(r.Context(), kb)
			if p.Candidate != nil {
				for _, b := range shelf {
					if b.Source == "openlibrary" && b.SourceID == p.Candidate.SourceID {
						p.Match = &kindleMatch{BookID: b.ID, Title: b.Title, By: "openlibrary"}
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	for i := range out {
		if out[i].Match == nil {
			continue
		}
		known, _ := s.store.ExistingQuoteTexts(r.Context(), out[i].Match.BookID)
		for _, e := range out[i].Entries {
			if store.QuoteIsKnown(known, e.Text) {
				out[i].Known++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"books": out, "clippings": len(clips)})
}

func matchByTitle(kb kindle.Book, shelf []store.Book) *kindleMatch {
	var hit *store.Book
	for i := range shelf {
		b := &shelf[i]
		if !sameTitle(kb.Title, b.Title) {
			continue
		}
		if authorMatches(kb.Author, b.Authors) {
			return &kindleMatch{BookID: b.ID, Title: b.Title, By: "title"}
		}
		if hit == nil {
			hit = b
		}
	}
	if hit != nil && kb.Author == "" {
		return &kindleMatch{BookID: hit.ID, Title: hit.Title, By: "title"}
	}
	return nil
}

// lookupKindleBook finds the catalogue record of a Kindle book; nil when no
// result has a matching title and author.
func (s *Server) lookupKindleBook(ctx context.Context, kb kindle.Book) *metadata.Candidate {
	short := kb.Title
	for _, sep := range []string{":", " (", "["} {
		if i := strings.Index(short, sep); i > 0 {
			short = short[:i]
		}
	}
	for _, q := range []string{strings.TrimSpace(short + " " + kb.Author), kb.Title} {
		results, err := s.meta.SearchOpenLibrary(ctx, q, 6)
		if err != nil {
			continue
		}
		for _, c := range results {
			if sameTitle(kb.Title, c.Title) && authorMatches(kb.Author, strings.Join(c.Authors, ", ")) {
				c := c
				return &c
			}
		}
	}
	return nil
}

type kindleImportBook struct {
	Target    string              `json:"target"` // book | new | skip
	BookID    int64               `json:"book_id"`
	Status    string              `json:"status"` // for new books
	Title     string              `json:"title"`
	Author    string              `json:"author"`
	Candidate *metadata.Candidate `json:"candidate"`
	Entries   []kindle.Entry      `json:"entries"`
}

func (s *Server) handleKindleImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Books []kindleImportBook `json:"books"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := r.Context()
	res := map[string]int{"books_created": 0, "books_updated": 0, "added": 0, "updated": 0, "skipped": 0}
	for _, kb := range in.Books {
		var bookID int64
		switch kb.Target {
		case "book":
			bookID = kb.BookID
		case "new":
			b, created, err := s.createKindleBook(r, kb)
			if err != nil {
				s.fail(w, err, "create book")
				return
			}
			bookID = b.ID
			if created {
				res["books_created"]++
			}
		default:
			continue
		}
		quotes := make([]store.Quote, 0, len(kb.Entries))
		for _, e := range kb.Entries {
			q := store.Quote{Text: strings.TrimSpace(e.Text), Page: e.Where(), Note: strings.TrimSpace(e.Note), CreatedAt: e.Added}
			if e.NoteOnly {
				q.Note = "Kindle note"
			}
			quotes = append(quotes, q)
		}
		added, updated, skipped, err := s.store.ImportQuotes(ctx, me(r), bookID, quotes)
		if err != nil {
			s.fail(w, err, "import highlights")
			return
		}
		if kb.Target == "book" && added+updated > 0 {
			res["books_updated"]++
		}
		res["added"] += added
		res["updated"] += updated
		res["skipped"] += skipped
	}
	s.log.Info("kindle import", "user", me(r), "result", res)
	writeJSON(w, http.StatusOK, res)
}

// createKindleBook adds a book for a Kindle title, from its catalogue record
// when the preview found one. A book already on the shelf (same ISBN, or same
// title and authors) is reused, so running an import twice adds no copies.
func (s *Server) createKindleBook(r *http.Request, kb kindleImportBook) (*store.Book, bool, error) {
	b := &store.Book{UserID: me(r), Title: strings.TrimSpace(kb.Title), Authors: kb.Author, Format: "ebook", Status: kb.Status, Source: "manual"}
	if !store.ValidStatus(b.Status) {
		b.Status = store.StatusRead
	}
	coverURL := ""
	if c := kb.Candidate; c != nil {
		if full, err := s.meta.Details(r.Context(), c.Source, c.SourceID); err == nil {
			if c.Description == "" {
				c.Description = full.Description
			}
			if c.CoverURL == "" {
				c.CoverURL = full.CoverURL
			}
		}
		b.Subtitle, b.ISBN13, b.ISBN10, b.Pages, b.Published = c.Subtitle, c.ISBN13, c.ISBN10, c.Pages, c.Published
		b.Publisher, b.Language, b.Description = c.Publisher, c.Language, c.Description
		b.Source, b.SourceID, b.SourceURL = c.Source, c.SourceID, c.SourceURL
		if len(c.Authors) > 0 {
			b.Authors = strings.Join(c.Authors, ", ")
		}
		coverURL = c.CoverURL
	}
	if b.Title == "" {
		b.Title = "Untitled"
	}
	_ = clean(b)
	if dup := s.findDuplicate(r, b); dup != nil {
		return dup, false, nil
	}
	if err := s.store.CreateBook(r.Context(), b, "", ""); err != nil {
		return nil, false, err
	}
	if coverURL != "" {
		if err := s.saveCoverFromURL(r, b, coverURL); err != nil {
			s.log.Warn("kindle import: cover download failed", "book", b.ID, "err", err)
		}
	}
	return b, true, nil
}
