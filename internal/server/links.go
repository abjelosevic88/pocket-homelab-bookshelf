package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core): another
// Pocket app may act here for a person with a pcx_ token, only within these
// scopes. Every route not listed in linkRoutes stays closed to it.

var linkOffers = []core.Offer{
	{Scope: "books:read", Name: "See your books", Description: "List your books, reading progress and finished dates."},
	{Scope: "books:write", Name: "Update your books", Description: "Set a book's status and rating, add it to your shelves, and link it to the same book in another app."},
}

// portable is what Pocket Move may copy out and bring in (docs/move.md in
// pocket-homelab-core). Imports add; books already on the shelf (same ISBN,
// or same title and authors) are skipped, so a second run adds nothing.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "bookshelf.json", Kind: core.KindPocket, Title: "Every book with its reads, quotes and followed authors"},
		{Path: "/api/v1/export.csv", File: "books.csv", Kind: core.KindCSV, Title: "Every book as a table (Goodreads-style columns)"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", MaxBytes: 8 << 20, Title: "Adds the books from another box; books already on the shelf are left alone"},
		{Path: "/api/v1/import/goodreads", Kind: core.KindGoodreads, Form: "file", MaxBytes: 32 << 20, Title: "Adds the books from a Goodreads or StoryGraph export; books already on the shelf are skipped"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/books$`), "books:read"},
	{"GET", regexp.MustCompile(`^/api/v1/books/\d+$`), "books:read"},
	{"GET", regexp.MustCompile(`^/api/v1/books/\d+/sessions$`), "books:read"},
	{"GET", regexp.MustCompile(`^/api/v1/reading/sessions$`), "books:read"},
	// the same book in Pocket Library: which one it is here, and linking, unlinking and updating it
	{"GET", regexp.MustCompile(`^/api/v1/library/links/\d+$`), "books:read"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/library/links/\d+$`), "books:write"},
	{"POST", regexp.MustCompile(`^/api/v1/library/links/\d+/apply$`), "books:write"},
}

var errLinkScope = errors.New("outside the link's scope")

func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token and maps it to the person it acts for.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" {
		return nil, http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, errLinkScope
	}
	return s.coreIdentityUser(ctx, in)
}
