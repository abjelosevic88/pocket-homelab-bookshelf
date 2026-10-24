package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// Audiobooks from Pocket Audiobooks, through Pocket Core (link
// listening:read): every audiobook a person finishes there — in the web player
// or any Audiobookshelf app — becomes a read on their shelves, and one they
// are in the middle of goes on "reading". An audiobook is matched to a book
// already on the shelves by title and author; otherwise it is added as an
// audio book with its cover. A finish or start is taken in once, and a book
// the person removes is never added again. Each person can switch it off
// (setting audiobooks_sync = "off").

var audiobooksUse = core.Use{App: "audiobooks", Scopes: []string{"listening:read"}, Reason: "Audiobooks you finish (or are in the middle of) in Pocket Audiobooks, as reads on your shelves"}

const audiobooksEvery = 30 * time.Minute

var abHTTP = &http.Client{Timeout: 30 * time.Second}

// abState is how the last sync went for one person (shown in Settings).
type abState struct {
	At       time.Time `json:"at"`
	Added    int       `json:"added"`
	Finished int       `json:"finished"`
	Started  int       `json:"started"`
	Error    string    `json:"error,omitempty"`
	Off      bool      `json:"off,omitempty"` // the link is off or missing: not an error, just not connected
}

var (
	abMu     sync.Mutex
	abStates = map[int64]abState{}
	abBusy   = map[int64]bool{}
)

// audiobooksWorker syncs everyone every half hour (and soon after a start).
func (s *Server) audiobooksWorker(ctx context.Context) {
	first := time.NewTimer(90 * time.Second)
	defer first.Stop()
	t := time.NewTicker(audiobooksEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		case <-t.C:
		}
		if !s.core.Configured() {
			continue
		}
		users, err := s.store.ListUsers(ctx)
		if err != nil {
			continue
		}
		for _, u := range users {
			if s.store.UserKey(ctx, u.ID, "audiobooks_sync") == "off" {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			st := s.syncAudiobooks(cctx, u.ID, u.Email)
			cancel()
			if st.Error != "" {
				s.log.Warn("audiobooks sync", "user", u.ID, "err", st.Error)
			}
		}
	}
}

// abClient reaches the person's Pocket Audiobooks with a fresh link token.
type abClient struct {
	base, public, token string
}

func (c *abClient) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	res, err := abHTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("Pocket Audiobooks answered HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *abClient) cover(ctx context.Context, itemID string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/cover/"+url.PathEscape(itemID)+"?w=600", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "image/jpeg,image/png,image/webp")
	res, err := abHTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	return b, strings.Split(res.Header.Get("Content-Type"), ";")[0], err
}

func (s *Server) abFor(ctx context.Context, email string) (*abClient, *core.LinkError, error) {
	lt, err := s.core.ActAs(ctx, email, "audiobooks", "listening:read")
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			return nil, le, nil
		}
		return nil, nil, err
	}
	return &abClient{base: strings.TrimRight(lt.URL, "/"), public: strings.TrimRight(lt.PublicURL, "/"), token: lt.AccessToken}, nil, nil
}

// abLinkReason says why Bookshelf can't reach Pocket Audiobooks for the person.
func abLinkReason(le *core.LinkError) string {
	switch le.Reason {
	case "target_missing":
		return "Pocket Audiobooks is not on this box."
	case "off_person":
		return "Switched off for you: turn on “Pocket Bookshelf → Pocket Audiobooks” in Pocket Core, Settings → Apps acting for me."
	case "off_admin":
		return "Switched off for this box: an admin turns on “Pocket Bookshelf → Pocket Audiobooks” in Pocket Core, Links."
	case "not_in_audience":
		return "You have no Pocket Audiobooks: an admin can give you access in Pocket Core."
	case "not_offered", "not_declared":
		return "This Pocket Audiobooks or Bookshelf is too old to link the two; update both."
	case "unknown_user":
		return "Your account is not in Pocket Core."
	}
	return "Pocket Audiobooks: " + le.Message
}

type abFinish struct {
	ItemID     string `json:"item_id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	FinishedAt string `json:"finished_at"`
	Day        string `json:"day"`
}

type abListen struct {
	ItemID   string `json:"item_id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Day      string `json:"day"`
	Duration int    `json:"duration"`
	EndPos   int    `json:"end_pos"`
}

// syncAudiobooks takes in one person's finished and in-progress audiobooks.
func (s *Server) syncAudiobooks(ctx context.Context, uid int64, email string) abState {
	abMu.Lock()
	if abBusy[uid] {
		st := abStates[uid]
		abMu.Unlock()
		return st
	}
	abBusy[uid] = true
	abMu.Unlock()
	st := abState{At: time.Now()}
	defer func() {
		abMu.Lock()
		abStates[uid] = st
		delete(abBusy, uid)
		abMu.Unlock()
	}()
	c, le, err := s.abFor(ctx, email)
	if err != nil {
		st.Error = "Pocket Core did not answer: " + err.Error()
		return st
	}
	if le != nil {
		st.Off, st.Error = true, abLinkReason(le)
		return st
	}
	var finishes []abFinish
	if err := c.get(ctx, "/api/v1/finished", nil, &finishes); err != nil {
		st.Error = err.Error()
		return st
	}
	// the last two months of sessions: when each book was started, what is in progress
	var hist struct {
		Items []abListen `json:"items"`
	}
	from := time.Now().AddDate(0, -2, 0).Format("2006-01-02")
	if err := c.get(ctx, "/api/v1/history", url.Values{"from": {from}, "limit": {"500"}}, &hist); err != nil {
		st.Error = err.Error()
		return st
	}
	firstDay := map[string]string{}
	latest := map[string]abListen{}
	for _, l := range hist.Items { // newest first
		if _, ok := latest[l.ItemID]; !ok {
			latest[l.ItemID] = l
		}
		firstDay[l.ItemID] = l.Day
	}
	shelf, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid})
	if err != nil {
		st.Error = err.Error()
		return st
	}
	finished := map[string]bool{}
	for _, f := range finishes {
		finished[f.ItemID] = true
		day := f.Day
		if day == "" && len(f.FinishedAt) >= 10 {
			day = f.FinishedAt[:10]
		}
		if day == "" || s.store.AudiobookEventSeen(ctx, uid, f.ItemID, "finished", day) {
			continue
		}
		bookID, added, ok := s.abBook(ctx, c, uid, f.ItemID, f.Title, f.Author, store.StatusRead, "", &shelf)
		if !ok {
			_ = s.store.MarkAudiobookEvent(ctx, uid, f.ItemID, "finished", day)
			continue
		}
		start := firstDay[f.ItemID]
		if start > day {
			start = ""
		}
		changed, err := s.store.FinishFromAudiobook(ctx, uid, bookID, start, day)
		if err != nil {
			st.Error = err.Error()
			return st
		}
		_ = s.store.MarkAudiobookEvent(ctx, uid, f.ItemID, "finished", day)
		if added {
			st.Added++
		}
		if changed {
			st.Finished++
		}
	}
	for id, l := range latest {
		if finished[id] || l.Duration <= 0 {
			continue
		}
		p := float64(l.EndPos) / float64(l.Duration)
		start := firstDay[id]
		if p < 0.02 || p > 0.98 || start == "" || s.store.AudiobookEventSeen(ctx, uid, id, "started", start) {
			continue
		}
		bookID, added, ok := s.abBook(ctx, c, uid, id, l.Title, l.Author, store.StatusReading, start, &shelf)
		_ = s.store.MarkAudiobookEvent(ctx, uid, id, "started", start)
		if !ok {
			continue
		}
		if added {
			st.Added++
			st.Started++ // created reading, from the day the listening started
			continue
		}
		if changed, err := s.store.StartFromAudiobook(ctx, uid, bookID, start); err == nil && changed {
			st.Started++
		}
	}
	return st
}

var (
	abNonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	abParens  = regexp.MustCompile(`\s*[\(\[].*?[\)\]]`)
)

// abNorm makes a title comparable: no subtitle, no "(Unabridged)", no leading article.
func abNorm(t string) string {
	t = strings.ToLower(abParens.ReplaceAllString(t, ""))
	if i := strings.Index(t, ":"); i > 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(abNonWord.ReplaceAllString(t, " "))
	for _, a := range []string{"the ", "a ", "an "} {
		t = strings.TrimPrefix(t, a)
	}
	return t
}

// abSameAuthor is true when the two credit lines share a surname (or one is empty).
func abSameAuthor(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return true
	}
	last := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, n := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == '&' || r == ';' }) {
			f := strings.Fields(abNonWord.ReplaceAllString(n, " "))
			if len(f) > 0 {
				out[f[len(f)-1]] = true
			}
		}
		return out
	}
	la := last(a)
	for k := range last(b) {
		if la[k] {
			return true
		}
	}
	return false
}

// abBook finds the book an audiobook is on the person's shelves, or adds it
// (as an audio book with the cover from Pocket Audiobooks). ok is false when
// the person removed the book the audiobook was linked to.
func (s *Server) abBook(ctx context.Context, c *abClient, uid int64, itemID, title, author, status, startedOn string, shelf *[]store.Book) (bookID int64, added, ok bool) {
	if id, found := s.store.AudiobookLink(ctx, uid, itemID); found {
		if _, err := s.store.GetBook(ctx, uid, id); err != nil {
			return 0, false, false // removed by the person: leave it out
		}
		return id, false, true
	}
	want := abNorm(title)
	for _, b := range *shelf {
		if b.SourceID == itemID && b.Source == "audiobooks" || want != "" && abNorm(b.Title) == want && abSameAuthor(b.Authors, author) {
			_ = s.store.SetAudiobookLink(ctx, uid, itemID, b.ID)
			return b.ID, false, true
		}
	}
	if strings.TrimSpace(title) == "" {
		return 0, false, false
	}
	b := &store.Book{UserID: uid, Title: strings.TrimSpace(title), Authors: strings.TrimSpace(author), Format: "audio", Status: status, Source: "audiobooks", SourceID: itemID}
	if c.public != "" {
		b.SourceURL = c.public + "/book/" + url.PathEscape(itemID)
	}
	// a finished book gets its read from FinishFromAudiobook (it knows the dates); a reading one starts on startedOn
	if err := s.store.CreateBook(ctx, b, startedOn, ""); err != nil {
		s.log.Warn("audiobooks: add book", "title", title, "err", err)
		return 0, false, false
	}
	_ = s.store.SetAudiobookLink(ctx, uid, itemID, b.ID)
	if data, ct, err := c.cover(ctx, itemID); err == nil && len(data) > 0 {
		if err := s.writeCover(ctx, b, data, ct); err != nil {
			s.log.Debug("audiobooks: cover", "title", title, "err", err)
		}
	}
	*shelf = append(*shelf, *b)
	return b.ID, true, true
}

// ---- API ----

// GET /api/v1/audiobooks — whether audiobooks come in, and how the last sync went.
func (s *Server) handleAudiobooksStatus(w http.ResponseWriter, r *http.Request) {
	uid := me(r)
	abMu.Lock()
	st, ran := abStates[uid]
	busy := abBusy[uid]
	abMu.Unlock()
	out := map[string]any{"core": s.core.Configured(), "enabled": s.store.UserKey(r.Context(), uid, "audiobooks_sync") != "off", "syncing": busy}
	if ran {
		out["last"] = st
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /api/v1/audiobooks {"enabled": bool}
func (s *Server) handleAudiobooksSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	v := "on"
	if !in.Enabled {
		v = "off"
	}
	if err := s.store.PutUserKey(r.Context(), me(r), "audiobooks_sync", v); err != nil {
		s.fail(w, err, "audiobooks")
		return
	}
	s.handleAudiobooksStatus(w, r)
}

// POST /api/v1/audiobooks/sync — take in the caller's audiobooks now.
func (s *Server) handleAudiobooksSync(w http.ResponseWriter, r *http.Request) {
	if !s.core.Configured() {
		writeErr(w, http.StatusConflict, "audiobooks come from Pocket Audiobooks through Pocket Core, which this Bookshelf is not joined to")
		return
	}
	u, err := s.store.GetUser(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "audiobooks")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	writeJSON(w, http.StatusOK, s.syncAudiobooks(ctx, u.ID, u.Email))
}
