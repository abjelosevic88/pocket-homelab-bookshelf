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
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

type summaryWriter struct {
	mu      sync.Mutex
	retryAt map[int64]time.Time // book id → not before
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
