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

// With an OpenAI key (Settings → AI; for the owner also OPENAI_API_KEY) the
// app writes book characters itself, in the background: new books right
// after they are added, and every 30 minutes whatever is still missing. Each
// person's books are written with their own key, in their own languages; a
// person without a key gets none from the app (an external writer such as
// scripts/characters.py can still do it with their API token, and
// /characters/missing tells it to stand aside while the app has their key).

type charWriter struct {
	mu        sync.Mutex
	running   bool
	paused    map[int64]bool   // per person: a key/quota problem, wait until their settings change
	lastError map[int64]string // per person
	lastRun   time.Time
	written   map[int64]int                 // per person
	retryAt   map[int64]map[int64]time.Time // per person: books that failed, not before
}

// reset forgets a person's pause, error and back-offs (new settings, "write now").
func (cw *charWriter) reset(uid int64) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	delete(cw.paused, uid)
	delete(cw.lastError, uid)
	delete(cw.retryAt, uid)
}

func (s *Server) kickCharacters() {
	select {
	case s.charKick <- struct{}{}:
	default:
	}
}

// openAI returns the selected generation client, using this person's Core link or key.
func (s *Server) openAI(ctx context.Context, uid int64) *ai.OpenAI {
	a, err := s.store.GetAISettings(ctx, uid)
	if err != nil {
		return nil
	}
	key := a.OpenAIKey
	if key == "" && s.cfg.OpenAIKey != "" && s.isOwner(ctx, uid) {
		key = s.cfg.OpenAIKey
	}
	pocket, _ := s.inferenceFor(ctx, uid, a.AIProvider, key != "")
	if key == "" && pocket == nil {
		return nil
	}
	return &ai.OpenAI{Pocket: pocket, Key: key, Model: a.Model, WebSearch: a.WebSearch, BaseURL: s.cfg.OpenAIBaseURL}
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

// writeMissingCharacters goes through everyone who has a key, one person at a
// time, each with their own key and languages and only over their own books.
func (s *Server) writeMissingCharacters(ctx context.Context) {
	cw := &s.chars
	cw.mu.Lock()
	if cw.running {
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
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		cw.mu.Lock()
		paused := cw.paused[u.ID]
		cw.mu.Unlock()
		if paused {
			continue
		}
		if oa := s.openAI(ctx, u.ID); oa != nil {
			s.writeMissingFor(ctx, u.ID, oa)
		}
	}
}

func (s *Server) writeMissingFor(ctx context.Context, uid int64, oa *ai.OpenAI) {
	cw := &s.chars
	langs := s.characterLangs(ctx, uid)
	tried := map[int64]bool{} // one attempt per book and run: a language that keeps coming back empty must not loop
	for ctx.Err() == nil {
		books, missing, err := s.store.MissingCharacters(ctx, uid, langs, 0)
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
			if t, ok := cw.retryAt[uid][b.ID]; (!ok || time.Now().After(t)) && !tried[b.ID] {
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
					if cw.written == nil {
						cw.written = map[int64]int{}
					}
					cw.written[uid]++
					delete(cw.retryAt[uid], j.b.ID)
					return
				}
				s.log.Warn("characters: AI generation failed", "user", uid, "book", j.b.ID, "title", j.b.Title, "err", err)
				if cw.lastError == nil {
					cw.lastError = map[int64]string{}
				}
				cw.lastError[uid] = err.Error()
				var ae *ai.APIError
				if errors.As(err, &ae) && (ae.Fatal() || ae.Status == http.StatusTooManyRequests) {
					if ae.Fatal() {
						if cw.paused == nil {
							cw.paused = map[int64]bool{}
						}
						cw.paused[uid] = true
					}
					stopMu.Lock()
					stop = true
					stopMu.Unlock()
					return
				}
				if cw.retryAt == nil {
					cw.retryAt = map[int64]map[int64]time.Time{}
				}
				if cw.retryAt[uid] == nil {
					cw.retryAt[uid] = map[int64]time.Time{}
				}
				cw.retryAt[uid][j.b.ID] = time.Now().Add(2 * time.Hour)
			}()
		}
		wg.Wait()
		if stop {
			return
		}
	}
}

// writeCharacters asks the selected provider for one book and saves the result. An
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
	source := "openai " + model
	if oa.Pocket != nil {
		source = "pocket-ai"
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
		if err := s.store.PutCharacters(ctx, b.UserID, b.ID, l, cleanCharacters(cs, source)); err != nil {
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

// The AI settings are personal: everyone sees and changes only their own key.
func (s *Server) handleGetAISettings(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAISettings(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	_, aiStatus := s.inferenceFor(r.Context(), me(r), a.AIProvider, a.OpenAIKey != "" || (s.cfg.OpenAIKey != "" && s.isOwner(r.Context(), me(r))))
	writeJSON(w, http.StatusOK, map[string]any{
		"ai_provider": aiStatus.Provider, "ai_status": aiStatus, "ai_configured": aiStatus.Ready,
		"key_set": a.OpenAIKey != "", "key_hint": keyHint(a.OpenAIKey), "key_env": s.cfg.OpenAIKey != "" && s.isOwner(r.Context(), me(r)),
		"model": a.Model, "default_model": ai.DefaultModel, "web_search": a.WebSearch,
	})
}

// handlePutAISettings: {"key": "sk-…" (omit to keep), "clear_key": true, "model", "web_search"}.
func (s *Server) handlePutAISettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AIProvider *string `json:"ai_provider"`
		Key        *string `json:"key"`
		ClearKey   bool    `json:"clear_key"`
		Model      string  `json:"model"`
		WebSearch  bool    `json:"web_search"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	a, err := s.store.GetAISettings(r.Context(), me(r))
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
	if in.AIProvider != nil {
		if !validAIProvider(*in.AIProvider) {
			writeErr(w, 400, "choose automatic, Pocket AI or OpenAI")
			return
		}
		a.AIProvider = *in.AIProvider
	}
	a.Model, a.WebSearch = strings.TrimSpace(in.Model), in.WebSearch
	if err := s.store.SaveAISettings(r.Context(), me(r), a); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	s.chars.reset(me(r))
	s.kickCharacters()
	s.handleGetAISettings(w, r)
}

// handleTestAI checks a key ({"key"}, or the caller's saved one) and lists its models.
func (s *Server) handleTestAI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	_ = decode(r, &in)
	oa := s.openAI(r.Context(), me(r))
	if k := strings.TrimSpace(in.Key); k != "" {
		oa = &ai.OpenAI{Key: k, BaseURL: s.cfg.OpenAIBaseURL}
	}
	if oa == nil || oa.Key == "" {
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

// handleCharacterStatus reports on the caller's own books and key.
func (s *Server) handleCharacterStatus(w http.ResponseWriter, r *http.Request) {
	uid := me(r)
	books, _, _ := s.store.MissingCharacters(r.Context(), uid, s.characterLangs(r.Context(), uid), 0)
	writer := s.characterWriter(r.Context(), uid)
	cw := &s.chars
	cw.mu.Lock()
	defer cw.mu.Unlock()
	out := map[string]any{
		"writer": writer, "running": cw.running, "paused": cw.paused[uid], "waiting": len(books),
		"written": cw.written[uid], "last_error": cw.lastError[uid], "failed": len(cw.retryAt[uid]),
	}
	if !cw.lastRun.IsZero() {
		out["last_run"] = cw.lastRun.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRunCharacters writes what is missing now (the caller's failed books included).
func (s *Server) handleRunCharacters(w http.ResponseWriter, r *http.Request) {
	s.chars.reset(me(r))
	s.kickCharacters()
	w.WriteHeader(http.StatusAccepted)
}

// characterWriter is "app" when the app writes the person's characters itself, else "external".
func (s *Server) characterWriter(ctx context.Context, uid int64) string {
	if s.openAI(ctx, uid) != nil {
		return "app"
	}
	return "external"
}
