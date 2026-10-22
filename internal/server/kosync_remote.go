package server

// CrossPoint Sync (crosspoint-sync, e.g. sync.crosspointreader.com) mirrors
// progress here as bare file hashes. With the person's CrossPoint Sync account
// Bookshelf reads that server's list of documents, which carries the title,
// author, cover and page count devices sent it, plus its newest progress, and
// matches each to a book on the shelf or adds it.

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

const (
	kosyncPullEvery = 15 * time.Minute
	kosyncPullGap   = 30 * time.Second // a mirror pushing every page turn asks at most this often
)

// kickKosyncPull asks the worker to read the person's CrossPoint Sync account soon.
func (s *Server) kickKosyncPull(userID int64) {
	if last, ok := s.kosyncLast.Load(userID); ok && time.Since(last.(time.Time)) < kosyncPullGap {
		return
	}
	select {
	case s.kosyncKick <- userID:
	default:
	}
}

func (s *Server) kosyncWorker(ctx context.Context) {
	pullAll := func() {
		ids, err := s.store.KosyncRemoteUsers(ctx)
		if err != nil {
			return
		}
		for _, uid := range ids {
			s.pullOrLog(ctx, uid)
		}
	}
	first := time.NewTimer(30 * time.Second) // catch up after a restart
	defer first.Stop()
	t := time.NewTicker(kosyncPullEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case uid := <-s.kosyncKick:
			time.Sleep(2 * time.Second) // let the device's burst of updates land first
			s.pullOrLog(ctx, uid)
		case <-first.C:
			pullAll()
		case <-t.C:
			pullAll()
		}
	}
}

func (s *Server) pullOrLog(ctx context.Context, uid int64) {
	if r, err := s.pullCrossPoint(ctx, uid); err != nil {
		s.log.Warn("crosspoint pull", "user", uid, "err", err)
	} else if r.Updated+r.Linked > 0 {
		s.log.Info("crosspoint pull", "user", uid, "documents", r.Documents, "updated", r.Updated, "linked", r.Linked)
	}
}

type pullResult struct {
	Documents int `json:"documents"`
	Updated   int `json:"updated"` // newer progress than Bookshelf had
	Linked    int `json:"linked"`  // documents that found (or became) their book
}

// crossPointItem is one entry of crosspoint-sync's GET /api/v1/progress.
type crossPointItem struct {
	Document   string  `json:"document"`
	Title      *string `json:"title"`
	Author     *string `json:"author"`
	Filename   *string `json:"filename"`
	Percentage float64 `json:"percentage"`
	Progress   string  `json:"progress"`
	Device     string  `json:"device"`
	DeviceID   string  `json:"device_id"`
	Timestamp  int64   `json:"timestamp"`
	Status     *string `json:"status"`
	StatusAt   *int64  `json:"status_at"`
	CoverURL   *string `json:"cover_url"`
	PageCount  *int    `json:"page_count"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func crossPointRequest(ctx context.Context, p *store.KosyncPrefs, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.RemoteURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-User", p.RemoteUser)
	req.Header.Set("X-Auth-Key", p.RemoteKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PocketBookshelf/"+Version)
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized {
		res.Body.Close()
		return nil, errors.New("CrossPoint Sync did not accept the username or password")
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("CrossPoint Sync answered %s", res.Status)
	}
	return res, nil
}

// pullCrossPoint reads the person's CrossPoint Sync account once.
func (s *Server) pullCrossPoint(ctx context.Context, uid int64) (pullResult, error) {
	var out pullResult
	p, err := s.store.GetKosyncPrefs(ctx, uid)
	if err != nil || !p.Connected() {
		return out, err
	}
	s.kosyncLast.Store(uid, time.Now())
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return out, err
	}
	res, err := crossPointRequest(ctx, p, "/api/v1/progress?limit=500")
	if err != nil {
		s.store.SetKosyncPull(ctx, uid, err.Error())
		return out, err
	}
	defer res.Body.Close()
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 16<<20)).Decode(&list); err != nil {
		err = fmt.Errorf("CrossPoint Sync sent something unexpected: %w", err)
		s.store.SetKosyncPull(ctx, uid, err.Error())
		return out, err
	}
	for _, raw := range list.Items {
		var it crossPointItem
		if json.Unmarshal(raw, &it) != nil || !kosyncDocumentRE.MatchString(it.Document) {
			continue
		}
		out.Documents++
		before, err := s.store.GetKosyncDocument(ctx, uid, it.Document)
		known := err == nil
		pages := 0
		if it.PageCount != nil {
			pages = *it.PageCount
		}
		meta := &store.KosyncDocument{Document: it.Document, Title: clip(str(it.Title), 512), Authors: clip(str(it.Author), 512),
			Filename: clip(str(it.Filename), 512), CoverURL: clip(str(it.CoverURL), 1024), PageCount: pages}
		if err := s.store.SaveKosyncMeta(ctx, uid, meta); err != nil {
			return out, err
		}
		newer := !known || it.Timestamp > before.Timestamp
		if newer || !s.store.HasKosyncEvents(ctx, uid, it.Document) {
			_ = s.store.AddKosyncEvent(ctx, uid, &store.KosyncEvent{Document: it.Document, Source: "crosspoint", Device: it.Device,
				DeviceID: it.DeviceID, Percentage: it.Percentage, Timestamp: it.Timestamp, Body: raw})
		}
		if newer {
			d := &store.KosyncDocument{Document: it.Document, Progress: clip(it.Progress, 1024), Percentage: min(max(it.Percentage, 0), 1),
				Device: clip(it.Device, 100), DeviceID: clip(it.DeviceID, 100), Timestamp: it.Timestamp}
			if err := s.store.SaveKosyncProgress(ctx, uid, d); err != nil {
				return out, err
			}
			out.Updated++
		}
		status := str(it.Status)
		if status != "finished" && status != "dnf" {
			status = ""
		}
		if err := s.applyKosync(ctx, u, it.Document, status); err != nil {
			s.log.Warn("kosync apply", "user", uid, "document", it.Document, "err", err)
		}
		if after, err := s.store.GetKosyncDocument(ctx, uid, it.Document); err == nil {
			if after.BookID != nil && (!known || before.BookID == nil) {
				out.Linked++
			}
			s.retryKosyncCovers(ctx, u, after)
		}
	}
	s.store.SetKosyncPull(ctx, uid, "")
	return out, nil
}

// createKosyncBook adds a book a device is reading to the shelf, under the
// title and author the device gave, filled in from the catalogue.
func (s *Server) createKosyncBook(ctx context.Context, u *store.User, d *store.KosyncDocument) (int64, error) {
	b := &store.Book{UserID: u.ID, Title: d.Title, Authors: d.Authors, Pages: d.PageCount, Status: store.StatusWant, Source: "kosync"}
	if msg := clean(b); msg != "" {
		return 0, errors.New(msg)
	}
	if err := s.store.CreateBook(ctx, b, "", ""); err != nil {
		return 0, err
	}
	s.enrichKosyncBook(ctx, u, d, b)
	s.kickCharacters()
	s.log.Info("kosync added book", "user", u.ID, "book", b.ID, "title", b.Title, "document", d.Document)
	return b.ID, nil
}

// enrichKosyncBook fills the blanks of a book sync added: details from the
// catalogue edition with the same title and author, else from the work the
// catalogue finds for that title by that author (a translation: its cover,
// page count and description, not its ISBN); else CrossPoint Sync's cover.
func (s *Server) enrichKosyncBook(ctx context.Context, u *store.User, d *store.KosyncDocument, b *store.Book) {
	titles, authors := kosyncNames(d)
	first := ""
	if a := splitList(d.Authors); len(a) > 0 {
		first = a[0]
	}
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	cands, _, err := s.metaFor(ctx, u.ID).Search(sctx, strings.TrimSpace(d.Title+" "+first), "", 10)
	cancel()
	if err != nil {
		s.log.Warn("kosync catalogue search", "title", d.Title, "err", err)
	}
	cover := d.CoverURL
	if c, exact := pickKosyncCandidate(cands, titles, authors); c != nil {
		if exact {
			b.Subtitle = firstOf(b.Subtitle, c.Subtitle)
			b.ISBN13, b.ISBN10 = firstOf(b.ISBN13, c.ISBN13), firstOf(b.ISBN10, c.ISBN10)
			b.Publisher, b.Language = firstOf(b.Publisher, c.Publisher), firstOf(b.Language, c.Language)
		}
		b.Published, b.Description = firstOf(b.Published, c.Published), firstOf(b.Description, c.Description)
		if b.Source == "kosync" || b.Source == "" {
			b.Source, b.SourceID, b.SourceURL = c.Source, c.SourceID, c.SourceURL
		}
		if b.Pages == 0 && c.Pages > 0 {
			b.Pages = c.Pages
			if d.Percentage > 0 {
				b.CurrentPage = int(math.Round(d.Percentage * float64(b.Pages)))
			}
		}
		if c.CoverURL != "" {
			cover = c.CoverURL
		}
		if msg := clean(b); msg == "" {
			if err := s.store.UpdateBook(ctx, b); err != nil {
				s.log.Warn("kosync enrich", "book", b.ID, "err", err)
			}
		}
	}
	if cover != "" && b.Cover == "" {
		if data, ct, err := s.meta.FetchCover(ctx, cover, s.cfg.MaxCoverBytes); err != nil {
			s.log.Warn("kosync cover", "book", b.ID, "url", cover, "err", err)
		} else if err := s.writeCover(ctx, b, data, ct); err != nil {
			s.log.Warn("kosync cover", "book", b.ID, "err", err)
		} else {
			return
		}
	}
	if b.Cover == "" {
		s.fetchMissingCovers(u.ID, []int64{b.ID}) // ISBN, then the person's AI, in the background
	}
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// pickKosyncCandidate is the catalogue result for the device's book: the first
// with the same title (preferring the same author), exact; else, for a
// translated title, the first by the same author, not exact. nil when the
// title matches only someone else's book or nothing fits.
func pickKosyncCandidate(cands []metadata.Candidate, titles, authors []string) (c *metadata.Candidate, exact bool) {
	var byTitle, byAuthor *metadata.Candidate
	for i := range cands {
		c := &cands[i]
		sameAuthor := len(authors) > 0 && kosyncAuthorsMatch(strings.Join(c.Authors, ", "), authors)
		same := false
		for _, t := range titles {
			if t == fold.String(c.Title) || t == fold.String(c.Title+" "+c.Subtitle) || (c.OriginalTitle != "" && t == fold.String(c.OriginalTitle)) {
				same = true
				break
			}
		}
		switch {
		case same && (len(authors) == 0 || sameAuthor):
			return c, true
		case same && byTitle == nil:
			byTitle = c
		case sameAuthor && byAuthor == nil:
			byAuthor = c
		}
	}
	if len(authors) == 0 && byTitle != nil {
		return byTitle, true
	}
	if byAuthor != nil {
		return byAuthor, false
	}
	return nil, false
}

// retryKosyncCovers looks up again, once per run, the books sync added that
// still have no cover (the catalogue or CrossPoint Sync may know it by now).
func (s *Server) retryKosyncCovers(ctx context.Context, u *store.User, d *store.KosyncDocument) {
	if d.BookID == nil || d.Ignored {
		return
	}
	if _, done := s.kosyncRetried.LoadOrStore(*d.BookID, true); done {
		return
	}
	b, err := s.store.GetBook(ctx, u.ID, *d.BookID)
	if err != nil || b.Cover != "" || b.Source != "kosync" {
		return // has a cover, or is the person's own book: leave it as they made it
	}
	s.enrichKosyncBook(ctx, u, d, b)
}

// ---- Settings → Reader sync: CrossPoint Sync and choices ----

func (s *Server) handleSaveKosyncPrefs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AddMissing *bool   `json:"add_missing"`
		RemoteURL  *string `json:"remote_url"`
		RemoteUser *string `json:"remote_user"`
		Password   string  `json:"remote_password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p, err := s.store.GetKosyncPrefs(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "get kosync prefs")
		return
	}
	if in.AddMissing != nil {
		p.AddMissing = *in.AddMissing
	}
	connect := false
	if in.RemoteURL != nil || in.RemoteUser != nil {
		raw, user := "", ""
		if in.RemoteURL != nil {
			raw = strings.TrimSpace(*in.RemoteURL)
		}
		if in.RemoteUser != nil {
			user = strings.TrimSpace(*in.RemoteUser)
		}
		if raw == "" && user == "" { // disconnect
			p.RemoteURL, p.RemoteUser, p.RemoteKey, p.LastError = "", "", "", ""
		} else {
			if !strings.Contains(raw, "://") {
				raw = "https://" + raw
			}
			pu, err := url.Parse(raw)
			if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
				writeErr(w, http.StatusBadRequest, "the server address does not look like https://…")
				return
			}
			if !kosyncUsernameRE.MatchString(user) {
				writeErr(w, http.StatusBadRequest, "enter your CrossPoint Sync username")
				return
			}
			raw = strings.TrimRight(raw, "/")
			if in.Password != "" {
				sum := md5.Sum([]byte(in.Password))
				p.RemoteKey = hex.EncodeToString(sum[:])
			} else if raw != p.RemoteURL || !strings.EqualFold(user, p.RemoteUser) {
				p.RemoteKey = ""
			}
			if p.RemoteKey == "" {
				writeErr(w, http.StatusBadRequest, "enter your CrossPoint Sync password")
				return
			}
			p.RemoteURL, p.RemoteUser = raw, user
			res, err := crossPointRequest(r.Context(), p, "/users/auth")
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			res.Body.Close()
			connect = true
		}
	}
	if err := s.store.SaveKosyncPrefs(r.Context(), p); err != nil {
		s.fail(w, err, "save kosync prefs")
		return
	}
	if connect {
		if _, err := s.pullCrossPoint(r.Context(), p.UserID); err != nil {
			s.log.Warn("crosspoint first pull", "user", p.UserID, "err", err)
		}
	}
	s.handleGetKosync(w, r)
}

func (s *Server) handleKosyncPull(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetKosyncPrefs(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "get kosync prefs")
		return
	}
	if !p.Connected() {
		writeErr(w, http.StatusBadRequest, "connect a CrossPoint Sync account first")
		return
	}
	res, err := s.pullCrossPoint(r.Context(), me(r))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleKosyncEvents(w http.ResponseWriter, r *http.Request) {
	ev, err := s.store.ListKosyncEvents(r.Context(), me(r), chi.URLParam(r, "document"), 200)
	if err != nil {
		s.fail(w, err, "list kosync events")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}
