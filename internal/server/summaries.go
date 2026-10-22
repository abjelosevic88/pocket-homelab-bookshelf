package server

// A short, spoiler-free "what it is about" paragraph for every book, in each
// preferred language, written in the background by the person's AI (Pocket
// AI or their OpenAI key) after the characters, and shown first under
// "About the book".

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

type summaryWriter struct {
	mu         sync.Mutex
	retryAt    map[int64]time.Time // book id → not before
	coverTried map[int64]time.Time // book id → last automatic cover lookup
}

const summaryMaxFails = 3 // failures in a row before a person's run stops until the next one

func (s *Server) writeMissingSummaries(ctx context.Context) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		if oa := s.openAI(ctx, u.ID); oa != nil {
			s.writeSummariesFor(ctx, u.ID, oa)
			s.writeMissingFacts(ctx, u.ID, oa)
			s.findMissingCovers(ctx, u.ID)
		}
	}
}

func (s *Server) writeSummariesFor(ctx context.Context, uid int64, oa *ai.OpenAI) {
	langs := s.characterLangs(ctx, uid)
	books, missing, err := s.store.MissingSummaries(ctx, uid, langs)
	if err != nil {
		return
	}
	fails := 0
	for i, b := range books {
		if ctx.Err() != nil {
			return
		}
		s.summaries.mu.Lock()
		wait := time.Now().Before(s.summaries.retryAt[b.ID])
		s.summaries.mu.Unlock()
		if wait {
			continue
		}
		err := s.writeSummary(ctx, oa, b, missing[i])
		if err == nil {
			fails = 0
			continue
		}
		s.log.Warn("summary: AI generation failed", "user", uid, "book", b.ID, "title", b.Title, "err", err)
		s.summaries.mu.Lock()
		if s.summaries.retryAt == nil {
			s.summaries.retryAt = map[int64]time.Time{}
		}
		s.summaries.retryAt[b.ID] = time.Now().Add(2 * time.Hour)
		s.summaries.mu.Unlock()
		var ae *ai.APIError
		busy := strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), "busy")
		if fails++; fails >= summaryMaxFails || busy || (errors.As(err, &ae) && ae.Fatal()) {
			return // the AI is down, busy or refusing: try again on the next run
		}
	}
}

// writeSummary asks the AI for one book and saves what it wrote. An empty
// language stays missing, so it is asked for again.
func (s *Server) writeSummary(ctx context.Context, oa *ai.OpenAI, b store.Book, langs []string) error {
	res, err := oa.Summary(ctx, s.aiBook(ctx, b), langs)
	if err != nil {
		return err
	}
	saved := 0
	for _, l := range langs {
		m := store.Summary{Status: "unknown", Source: oa.Source()}
		if res.Status == "ok" {
			text := strings.Join(strings.Fields(res.Summary[l]), " ")
			if text == "" {
				continue
			}
			if ai.LatinLangs[l] {
				text = metadata.Latin(text)
			}
			m.Status, m.Text = "ok", clip(text, 1200)
		}
		if err := s.store.PutSummary(ctx, b.UserID, b.ID, l, m); err != nil {
			return err
		}
		saved++
	}
	if saved == 0 {
		return errors.New("the answer had no summary in " + strings.Join(langs, ", "))
	}
	return nil
}

func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	sums, err := s.store.Summaries(r.Context(), b.UserID, b.ID)
	if err != nil {
		s.fail(w, err, "summaries")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"langs": s.characterLangs(r.Context(), b.UserID), "summaries": sums,
		"writer": s.openAI(r.Context(), b.UserID) != nil})
}

// handleDeleteSummary forgets a book's summaries; the AI writes them again shortly.
func (s *Server) handleDeleteSummary(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if err := s.store.DeleteSummaries(r.Context(), me(r), id); err != nil {
		s.fail(w, err, "delete summaries")
		return
	}
	s.summaries.mu.Lock()
	delete(s.summaries.retryAt, id)
	s.summaries.mu.Unlock()
	s.kickCharacters()
	w.WriteHeader(http.StatusNoContent)
}

// writeMissingFacts asks the AI, ten books at a time, for the series and
// original title of books that have neither; only blanks are filled.
func (s *Server) writeMissingFacts(ctx context.Context, uid int64, oa *ai.OpenAI) {
	s.dropScriptTwins(ctx, uid)
	books, err := s.store.BooksWithoutAIFacts(ctx, uid)
	if err != nil || len(books) == 0 {
		return
	}
	var series []string
	if list, err := s.store.ListSeries(ctx, uid); err == nil {
		for _, sr := range list {
			series = append(series, sr.Name)
		}
	}
	for start := 0; start < len(books) && ctx.Err() == nil; start += 10 {
		batch := books[start:min(start+10, len(books))]
		in := make([]ai.FactsBook, len(batch))
		byID := map[int64]*store.Book{}
		for i := range batch {
			b := &batch[i]
			in[i] = ai.FactsBook{ID: b.ID, Title: b.Title, Authors: b.Authors, Published: b.Published, Lang: b.Language}
			byID[b.ID] = b
		}
		facts, err := oa.Facts(ctx, in, series)
		if err != nil {
			s.log.Warn("book facts: AI generation failed", "user", uid, "err", err)
			return // try again on the next run
		}
		for _, f := range facts {
			b := byID[f.ID]
			if b == nil {
				continue
			}
			changed := false
			if name := strings.TrimSpace(f.Series); b.Series == "" && name != "" {
				b.Series, b.SeriesNo, changed = clip(name, 120), clip(strings.TrimSpace(f.SeriesNo), 10), true
				if !slicesContains(series, name) {
					series = append(series, name)
				}
			}
			if t := strings.TrimSpace(f.OriginalTitle); b.OriginalTitle == "" && t != "" && !sameWorkTitle(t, b.Title) {
				b.OriginalTitle, changed = clip(t, 300), true
			}
			if changed {
				if err := s.store.UpdateBook(ctx, b); err != nil {
					s.log.Warn("book facts: save", "book", b.ID, "err", err)
					continue
				}
				s.log.Info("book facts", "user", uid, "book", b.ID, "title", b.Title, "series", b.Series, "no", b.SeriesNo, "original", b.OriginalTitle)
			}
		}
		for _, b := range batch {
			_ = s.store.MarkAIFacts(ctx, b.ID)
		}
	}
}

// sameWorkTitle reports whether two titles are the same apart from script, case
// and punctuation: "На Дрини ћуприја" is "Na Drini ćuprija", not its original.
func sameWorkTitle(a, b string) bool {
	return fold.String(metadata.Latin(a)) == fold.String(metadata.Latin(b))
}

// dropScriptTwins clears original titles that only spell the shelf title in another script.
func (s *Server) dropScriptTwins(ctx context.Context, uid int64) {
	books, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid})
	if err != nil {
		return
	}
	for i := range books {
		b := &books[i]
		if b.OriginalTitle != "" && sameWorkTitle(b.OriginalTitle, b.Title) {
			b.OriginalTitle = ""
			if err := s.store.UpdateBook(ctx, b); err == nil {
				s.log.Info("book facts: dropped a same-title original", "book", b.ID, "title", b.Title)
			}
		}
	}
}

func slicesContains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// findMissingCovers runs the cover lookup (ISBN, then AI) for books still
// without a cover, each at most once a day.
func (s *Server) findMissingCovers(ctx context.Context, uid int64) {
	books, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid})
	if err != nil {
		return
	}
	var ids []int64
	s.summaries.mu.Lock()
	if s.summaries.coverTried == nil {
		s.summaries.coverTried = map[int64]time.Time{}
	}
	for _, b := range books {
		if b.Cover == "" && time.Since(s.summaries.coverTried[b.ID]) > 24*time.Hour {
			ids = append(ids, b.ID)
		}
	}
	s.summaries.mu.Unlock()
	if len(ids) > 0 && s.fetchMissingCovers(uid, ids) {
		s.summaries.mu.Lock()
		for _, id := range ids {
			s.summaries.coverTried[id] = time.Now()
		}
		s.summaries.mu.Unlock()
	}
}
