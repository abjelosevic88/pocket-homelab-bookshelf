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
	t := time.NewTicker(kosyncPullEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case uid := <-s.kosyncKick:
			time.Sleep(2 * time.Second) // let the device's burst of updates land first
			s.pullOrLog(ctx, uid)
		case <-t.C:
			ids, err := s.store.KosyncRemoteUsers(ctx)
			if err != nil {
				continue
			}
			for _, uid := range ids {
				s.pullOrLog(ctx, uid)
			}
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
		if !known || it.Timestamp > before.Timestamp {
			d := &store.KosyncDocument{Document: it.Document, Progress: clip(it.Progress, 1024), Percentage: min(max(it.Percentage, 0), 1),
				Device: clip(it.Device, 100), DeviceID: clip(it.DeviceID, 100), Timestamp: it.Timestamp}
			if err := s.store.SaveKosyncProgress(ctx, uid, d); err != nil {
				return out, err
			}
			_ = s.store.AddKosyncEvent(ctx, uid, &store.KosyncEvent{Document: d.Document, Source: "crosspoint", Device: d.Device,
				DeviceID: d.DeviceID, Percentage: d.Percentage, Timestamp: d.Timestamp, Body: raw})
			out.Updated++
		}
		status := str(it.Status)
		if status != "finished" && status != "dnf" {
			status = ""
		}
		if err := s.applyKosync(ctx, u, it.Document, status); err != nil {
			s.log.Warn("kosync apply", "user", uid, "document", it.Document, "err", err)
		}
		if after, err := s.store.GetKosyncDocument(ctx, uid, it.Document); err == nil && after.BookID != nil && (!known || before.BookID == nil) {
			out.Linked++
		}
	}
	s.store.SetKosyncPull(ctx, uid, "")
	return out, nil
}

// createKosyncBook adds a book a device is reading to the shelf: the
// catalogue's edition when one has the same title and author, else what the
// device said, with CrossPoint Sync's cover and page count.
func (s *Server) createKosyncBook(ctx context.Context, u *store.User, d *store.KosyncDocument) (int64, error) {
	titles, authors := kosyncNames(d)
	b := &store.Book{UserID: u.ID, Title: d.Title, Authors: d.Authors, Pages: d.PageCount, Status: store.StatusWant, Source: "kosync"}
	cover := d.CoverURL
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
	if c := pickKosyncCandidate(cands, titles, authors); c != nil {
		b.Subtitle, b.ISBN13, b.ISBN10 = c.Subtitle, c.ISBN13, c.ISBN10
		b.Published, b.Publisher, b.Language, b.Description = c.Published, c.Publisher, c.Language, c.Description
		b.Source, b.SourceID, b.SourceURL = c.Source, c.SourceID, c.SourceURL
		if c.Pages > 0 {
			b.Pages = c.Pages
		}
		if len(c.Authors) > 0 {
			b.Authors = strings.Join(c.Authors, ", ")
		}
		if c.CoverURL != "" {
			cover = c.CoverURL
		}
	}
	if b.Authors == "" {
		b.Authors = d.Authors
	}
	if msg := clean(b); msg != "" {
		return 0, errors.New(msg)
	}
	if err := s.store.CreateBook(ctx, b, "", ""); err != nil {
		return 0, err
	}
	if cover != "" {
		if data, ct, err := s.meta.FetchCover(ctx, cover, s.cfg.MaxCoverBytes); err != nil {
			s.log.Warn("kosync cover", "book", b.ID, "url", cover, "err", err)
		} else if err := s.writeCover(ctx, b, data, ct); err != nil {
			s.log.Warn("kosync cover", "book", b.ID, "err", err)
		}
	}
	s.kickCharacters()
	s.log.Info("kosync added book", "user", u.ID, "book", b.ID, "title", b.Title, "document", d.Document)
	return b.ID, nil
}

// pickKosyncCandidate is the first catalogue result with the device's title,
// preferring one by the same author; nil when none has the title.
func pickKosyncCandidate(cands []metadata.Candidate, titles, authors []string) *metadata.Candidate {
	var byTitle *metadata.Candidate
	for i := range cands {
		c := &cands[i]
		same := false
		for _, t := range titles {
			if t == fold.String(c.Title) || t == fold.String(c.Title+" "+c.Subtitle) || (c.OriginalTitle != "" && t == fold.String(c.OriginalTitle)) {
				same = true
				break
			}
		}
		if !same {
			continue
		}
		if len(authors) == 0 || kosyncAuthorsMatch(strings.Join(c.Authors, ", "), authors) {
			return c
		}
		if byTitle == nil {
			byTitle = c
		}
	}
	if len(authors) > 0 {
		return nil // same title, someone else's book
	}
	return byTitle
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
