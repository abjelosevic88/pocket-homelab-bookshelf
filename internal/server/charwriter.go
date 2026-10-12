package server

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

// With an OpenAI key (Settings → AI, or OPENAI_API_KEY) the app writes book
// characters itself, in the background: new books right after they are
// added, and every 30 minutes whatever is still missing. Without a key the
// external writer (scripts/characters.py) does it; /characters/missing tells
// it to stand aside while the app has a key.

type charWriter struct {
	mu        sync.Mutex
	running   bool
	paused    bool // a key/quota problem: wait until the settings change
	lastError string
	lastRun   time.Time
	written   int
	retryAt   map[int64]time.Time // books that failed: not before
}

func (s *Server) kickCharacters() {
	select {
	case s.charKick <- struct{}{}:
	default:
	}
}

// openAI returns the configured client, or nil without a key.
func (s *Server) openAI(ctx context.Context) *ai.OpenAI {
	a, err := s.store.GetAISettings(ctx)
	if err != nil {
		return nil
	}
	key := a.OpenAIKey
	if key == "" {
		key = s.cfg.OpenAIKey
	}
	if key == "" {
		return nil
	}
	return &ai.OpenAI{Key: key, Model: a.Model, WebSearch: a.WebSearch, BaseURL: s.cfg.OpenAIBaseURL}
}

func (s *Server) characterWorker(ctx context.Context) {
	tick := time.NewTicker(30 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.charKick:
		case <-tick.C:
		}
		s.writeMissingCharacters(ctx)
	}
}

func (s *Server) writeMissingCharacters(ctx context.Context) {
	oa := s.openAI(ctx)
	cw := &s.chars
	cw.mu.Lock()
	if oa == nil || cw.paused || cw.running {
		cw.mu.Unlock()
		return
	}
	cw.running = true
	cw.mu.Unlock()
	defer func() {
		cw.mu.Lock()
		cw.running, cw.lastRun = false, time.Now()
		cw.mu.Unlock()
	}()

	tried := map[int64]bool{} // one attempt per book and run: a language that keeps coming back empty must not loop
	for ctx.Err() == nil {
		books, missing, err := s.store.MissingCharacters(ctx, 0, s.characterLangs(), 0)
		if err != nil {
			return
		}
		type job struct {
			b     store.Book
			langs []string
		}
		var jobs []job
		cw.mu.Lock()
		for i, b := range books {
			if t, ok := cw.retryAt[b.ID]; (!ok || time.Now().After(t)) && !tried[b.ID] {
				jobs = append(jobs, job{b, missing[i]})
			}
		}
		cw.mu.Unlock()
		if len(jobs) == 0 {
			return
		}
		if len(jobs) > 10 {
			jobs = jobs[:10]
		}
		for _, j := range jobs {
			tried[j.b.ID] = true
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, 2)
		var stop bool
		var stopMu sync.Mutex
		for _, j := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				stopMu.Lock()
				halted := stop
				stopMu.Unlock()
				if halted || ctx.Err() != nil {
					return
				}
				err := s.writeCharacters(ctx, oa, j.b, j.langs)
				cw.mu.Lock()
				defer cw.mu.Unlock()
				if err == nil {
					cw.written++
					delete(cw.retryAt, j.b.ID)
					return
				}
				s.log.Warn("characters: OpenAI failed", "book", j.b.ID, "title", j.b.Title, "err", err)
				cw.lastError = err.Error()
				var ae *ai.APIError
				if errors.As(err, &ae) && (ae.Fatal() || ae.Status == http.StatusTooManyRequests) {
					if ae.Fatal() {
						cw.paused = true
					}
					stopMu.Lock()
					stop = true
					stopMu.Unlock()
					return
				}
				if cw.retryAt == nil {
					cw.retryAt = map[int64]time.Time{}
				}
				cw.retryAt[j.b.ID] = time.Now().Add(2 * time.Hour)
			}()
		}
		wg.Wait()
		if stop {
			return
		}
	}
}

// writeCharacters asks OpenAI for one book and saves what came back. An
// empty language stays missing, so it is asked for again.
func (s *Server) writeCharacters(ctx context.Context, oa *ai.OpenAI, b store.Book, langs []string) error {
	in := ai.Book{Title: b.Title, Subtitle: b.Subtitle, Authors: b.Authors, Published: b.Published, Series: b.Series,
		SeriesNo: b.SeriesNo, SourceURL: b.SourceURL, Description: clip(b.Description, 1500)}
	if b.Source == "openlibrary" && strings.HasPrefix(b.SourceID, "/works/") {
		if t := s.meta.OpenLibraryWorkTitle(ctx, b.SourceID); t != "" && t != b.Title {
			in.WorkTitle = t
		}
	}
	res, err := oa.Characters(ctx, in, langs)
	if err != nil {
		return err
	}
	model := oa.Model
	if model == "" {
		model = ai.DefaultModel
	}
	saved := 0
	for _, l := range langs {
		cs := store.CharacterSet{Status: "unknown"}
		switch res.Kind {
		case "nonfiction":
			cs.Status = "none"
		case "fiction":
			list := res.Characters[l]
			if len(list) == 0 {
				continue
			}
			cs.Status = "ok"
			for _, c := range list {
				if ai.LatinLangs[l] {
					c.Name, c.Description = metadata.Latin(c.Name), metadata.Latin(c.Description)
				}
				cs.Characters = append(cs.Characters, store.Character{Name: c.Name, Role: c.Role, Description: c.Description})
			}
		}
		if err := s.store.PutCharacters(ctx, b.ID, l, cleanCharacters(cs, "openai "+model)); err != nil {
			return err
		}
		saved++
	}
	if saved == 0 {
		return errors.New("the answer had no characters in " + strings.Join(langs, ", "))
	}
	return nil
}

// ---------- settings and status ----------

func keyHint(k string) string {
	if len(k) < 12 {
		return "set"
	}
	return k[:3] + "…" + k[len(k)-4:]
}

func (s *Server) handleGetAISettings(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAISettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key_set": a.OpenAIKey != "", "key_hint": keyHint(a.OpenAIKey), "key_env": s.cfg.OpenAIKey != "",
		"model": a.Model, "default_model": ai.DefaultModel, "web_search": a.WebSearch,
	})
}

// handlePutAISettings: {"key": "sk-…" (omit to keep), "clear_key": true, "model", "web_search"}.
func (s *Server) handlePutAISettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in struct {
		Key       *string `json:"key"`
		ClearKey  bool    `json:"clear_key"`
		Model     string  `json:"model"`
		WebSearch bool    `json:"web_search"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	a, err := s.store.GetAISettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	if in.Key != nil && strings.TrimSpace(*in.Key) != "" {
		a.OpenAIKey = strings.TrimSpace(*in.Key)
	}
	if in.ClearKey {
		a.OpenAIKey = ""
	}
	a.Model, a.WebSearch = strings.TrimSpace(in.Model), in.WebSearch
	if err := s.store.SaveAISettings(r.Context(), a); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	s.chars.mu.Lock()
	s.chars.paused, s.chars.lastError, s.chars.retryAt = false, "", nil
	s.chars.mu.Unlock()
	s.kickCharacters()
	s.handleGetAISettings(w, r)
}

// handleTestAI checks a key ({"key"}, or the saved one) and lists its models.
func (s *Server) handleTestAI(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in struct {
		Key string `json:"key"`
	}
	_ = decode(r, &in)
	oa := s.openAI(r.Context())
	if k := strings.TrimSpace(in.Key); k != "" {
		oa = &ai.OpenAI{Key: k, BaseURL: s.cfg.OpenAIBaseURL}
	}
	if oa == nil {
		writeErr(w, http.StatusBadRequest, "no OpenAI key yet")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	models, err := oa.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) handleCharacterStatus(w http.ResponseWriter, r *http.Request) {
	books, _, _ := s.store.MissingCharacters(r.Context(), 0, s.characterLangs(), 0)
	cw := &s.chars
	cw.mu.Lock()
	defer cw.mu.Unlock()
	out := map[string]any{
		"writer": s.characterWriter(r.Context()), "running": cw.running, "paused": cw.paused, "waiting": len(books),
		"written": cw.written, "last_error": cw.lastError, "failed": len(cw.retryAt),
	}
	if !cw.lastRun.IsZero() {
		out["last_run"] = cw.lastRun.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRunCharacters writes what is missing now (failed books included).
func (s *Server) handleRunCharacters(w http.ResponseWriter, r *http.Request) {
	s.chars.mu.Lock()
	s.chars.paused, s.chars.retryAt = false, nil
	s.chars.mu.Unlock()
	s.kickCharacters()
	w.WriteHeader(http.StatusAccepted)
}

// characterWriter is "app" when the app writes characters itself, else "external".
func (s *Server) characterWriter(ctx context.Context) string {
	if s.openAI(ctx) != nil {
		return "app"
	}
	return "external"
}
