package server

// Covers found with the person's AI (Pocket AI, else their OpenAI key) for
// books the catalogues have no cover for: translations listed under another
// title, local editions, books without an ISBN. The model names what
// identifies the book (the edition's ISBNs, the original title, image
// addresses it is sure of); Bookshelf turns that into covers and downloads
// each one before offering or saving it, so a wrong guess costs a request,
// never a broken cover.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

var errNoAI = errors.New("connect Pocket AI (or add an OpenAI key) under Settings → AI to find covers with AI")

const aiCoverCandidates = 20 // addresses tried per book at most

// aiCovers returns up to limit covers for the book that really download, best first.
func (s *Server) aiCovers(ctx context.Context, uid int64, b *store.Book, limit int) ([]metadata.CoverOption, *ai.CoverHints, error) {
	o := s.openAI(ctx, uid)
	if o == nil {
		return nil, nil, errNoAI
	}
	h, err := o.CoverHints(ctx, ai.CoverBook{Title: b.Title, Authors: b.Authors, Language: b.Language, Publisher: b.Publisher,
		Published: b.Published, ISBN13: b.ISBN13, Series: b.Series, SeriesNo: b.SeriesNo})
	if err != nil {
		return nil, nil, err
	}
	var cands []metadata.CoverOption
	seen := map[string]bool{}
	add := func(c metadata.CoverOption) {
		if c.URL != "" && !seen[c.URL] && len(cands) < aiCoverCandidates {
			seen[c.URL] = true
			if c.Thumb == "" {
				c.Thumb = c.URL
			}
			cands = append(cands, c)
		}
	}
	// The edition's own ISBNs first: the most exact match there is.
	for _, raw := range append([]string{b.ISBN13}, h.ISBNs...) {
		if isbn := metadata.NormalizeISBN(raw); isbn != "" {
			add(metadata.CoverOption{URL: "https://covers.openlibrary.org/b/isbn/" + isbn + "-L.jpg?default=false",
				Thumb: "https://covers.openlibrary.org/b/isbn/" + isbn + "-M.jpg?default=false", Source: "openlibrary", Label: "ISBN " + isbn})
		}
	}
	for _, u := range h.CoverURLs {
		if u = strings.TrimSpace(u); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			add(metadata.CoverOption{URL: u, Source: "ai", Label: "suggested by AI · " + hostOf(u)})
		}
	}
	// Then the editions of the work the AI says this is, preferred languages first.
	title, authors := firstOf(strings.TrimSpace(h.OriginalTitle), b.Title), firstOf(strings.TrimSpace(h.OriginalAuthors), b.Authors)
	work := ""
	if b.Source == "openlibrary" {
		work = b.SourceID
	}
	for _, c := range s.metaFor(ctx, uid).CoverSuggestions(ctx, work, title, authors) {
		add(c)
	}
	return s.downloadable(ctx, cands, limit), h, nil
}

// downloadable keeps, in order, the options whose image really downloads.
func (s *Server) downloadable(ctx context.Context, cands []metadata.CoverOption, limit int) []metadata.CoverOption {
	ok := make([]bool, len(cands))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range cands {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			_, _, err := s.meta.FetchCover(cctx, cands[i].URL, s.cfg.MaxCoverBytes)
			ok[i] = err == nil
		}(i)
	}
	wg.Wait()
	out := []metadata.CoverOption{}
	for i, c := range cands {
		if ok[i] && len(out) < limit {
			out = append(out, c)
		}
	}
	return out
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexByte(u, '/'); i > 0 {
		u = u[:i]
	}
	return u
}

// aiCoverFor saves the best AI-found cover on a book with none; false when none was found.
func (s *Server) aiCoverFor(ctx context.Context, uid int64, b *store.Book) (bool, error) {
	opts, _, err := s.aiCovers(ctx, uid, b, 1)
	if err != nil || len(opts) == 0 {
		return false, err
	}
	data, ct, err := s.meta.FetchCover(ctx, opts[0].URL, s.cfg.MaxCoverBytes)
	if err != nil {
		return false, err
	}
	if err := s.writeCover(ctx, b, data, ct); err != nil {
		return false, err
	}
	s.log.Info("cover found with AI", "user", uid, "book", b.ID, "title", b.Title, "url", opts[0].URL)
	return true, nil
}

// handleAICovers lists covers found with AI for the cover picker.
func (s *Server) handleAICovers(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	// The router gives requests 60 seconds; a model thinking about an edition can take longer.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 150*time.Second)
	defer cancel()
	opts, hints, err := s.aiCovers(ctx, me(r), b, 12)
	if errors.Is(err, errNoAI) {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"covers": opts, "hints": hints})
}
