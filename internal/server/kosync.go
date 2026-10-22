package server

// Reading progress from e-readers over the KOReader sync protocol (KOSync), the
// one sync.koreader.rocks speaks: KOReader, CrossPoint and CrossPoint Sync's
// "Another KOSync server" mirror only need this server's URL and the sync
// username and password the person picks in Settings → Reader sync.
//
// A device knows a book by the MD5 of its file (the "document"), not by title.
// A document is matched to a book on the shelf by the title and authors the
// device sends along (CrossPoint and newer KOReader do), else by its file name;
// the rest are linked by hand once in Settings. Progress on a matched book
// moves its current page (for books with a page count) and starts a book that
// was only on the want-to-read list.

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

var (
	kosyncUsernameRE = regexp.MustCompile(`^[A-Za-z0-9._@+-]{1,64}$`)
	kosyncDocumentRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	kosyncKeyRE      = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
)

// kosyncAuth remembers keys that checked out, so a device syncing every page
// turn does not pay for bcrypt each time, and counts failures per address.
type kosyncAuth struct {
	mu    sync.Mutex
	good  map[[32]byte]kosyncGood
	fails map[string][]time.Time
}

type kosyncGood struct {
	userID int64
	hash   string // the key hash it was checked against; a new password drops it
	until  time.Time
}

const (
	kosyncGoodFor    = 15 * time.Minute
	kosyncFailWindow = 10 * time.Minute
	kosyncMaxFails   = 20
)

// KOSync answers errors as {"code": n, "message": "..."} with the codes of the
// reference server, which clients show or act on.
func kosyncErr(w http.ResponseWriter, status, code int, msg string) {
	writeJSON(w, status, map[string]any{"code": code, "message": msg})
}

// kosyncRoutes is the protocol at the root of whatever it is mounted on.
func (s *Server) kosyncRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/users/create", s.handleKosyncCreate)
	r.Group(func(r chi.Router) {
		r.Use(s.kosyncRequireAuth)
		r.Get("/users/auth", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"authorized": "OK"})
		})
		r.Put("/syncs/progress", s.handleKosyncPut)
		r.Get("/syncs/progress/{document}", s.handleKosyncGet)
	})
	return r
}

// KosyncRouter serves only the sync protocol, for a listener of its own that
// can be published to the internet without the rest of the app.
func (s *Server) KosyncRouter() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	// Health without the app's details: this listener may face the internet.
	ok := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"state": "OK"})
	}
	r.Get("/healthz", ok)
	r.Get("/healthcheck", ok) // the reference server's path
	r.Mount("/", s.kosyncRoutes())
	return r
}

func (s *Server) handleKosyncCreate(w http.ResponseWriter, r *http.Request) {
	kosyncErr(w, http.StatusForbidden, 2003, "Registration is closed. Pick a sync username and password in Bookshelf → Settings → Reader sync, then log in.")
}

func (s *Server) kosyncRequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if s.kosync.limited(ip) {
			kosyncErr(w, http.StatusTooManyRequests, 2001, "Too many failed logins, try again later.")
			return
		}
		user, key := r.Header.Get("X-Auth-User"), strings.ToLower(r.Header.Get("X-Auth-Key"))
		if user == "" || key == "" {
			// Calibre-Web style clients send Basic auth with the plain password.
			if u, p, ok := r.BasicAuth(); ok {
				sum := md5.Sum([]byte(p))
				user, key = u, hex.EncodeToString(sum[:])
			}
		}
		u, err := s.kosyncUser(r.Context(), user, key)
		if err != nil {
			s.kosync.fail(ip)
			kosyncErr(w, http.StatusUnauthorized, 2001, "Unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (s *Server) kosyncUser(ctx context.Context, username, key string) (*store.User, error) {
	if !kosyncUsernameRE.MatchString(username) || !kosyncKeyRE.MatchString(key) {
		return nil, errors.New("bad credentials")
	}
	acc, err := s.store.KosyncAccountByUsername(ctx, username)
	if err != nil {
		// Spend the same time as a wrong password, so usernames cannot be probed.
		_ = bcrypt.CompareHashAndPassword(kosyncDummyHash, []byte(key))
		return nil, err
	}
	if !s.kosync.known(username, key, acc) {
		if err := bcrypt.CompareHashAndPassword([]byte(acc.KeyHash), []byte(key)); err != nil {
			return nil, err
		}
		s.kosync.remember(username, key, acc)
	}
	u, err := s.store.GetUser(ctx, acc.UserID)
	if err != nil {
		return nil, err
	}
	s.store.TouchKosyncAccount(ctx, u.ID)
	return u, nil
}

var kosyncDummyHash, _ = bcrypt.GenerateFromPassword([]byte("00000000000000000000000000000000"), bcrypt.DefaultCost)

func kosyncCacheKey(username, key string) [32]byte {
	return sha256.Sum256([]byte(strings.ToLower(username) + "\x00" + key))
}

func (a *kosyncAuth) known(username, key string, acc *store.KosyncAccount) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	g, ok := a.good[kosyncCacheKey(username, key)]
	return ok && g.userID == acc.UserID && g.hash == acc.KeyHash && time.Now().Before(g.until)
}

func (a *kosyncAuth) remember(username, key string, acc *store.KosyncAccount) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.good == nil || len(a.good) > 1000 {
		a.good = map[[32]byte]kosyncGood{}
	}
	a.good[kosyncCacheKey(username, key)] = kosyncGood{userID: acc.UserID, hash: acc.KeyHash, until: time.Now().Add(kosyncGoodFor)}
}

func (a *kosyncAuth) recent(ip string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range a.fails[ip] {
		if now.Sub(t) < kosyncFailWindow {
			keep = append(keep, t)
		}
	}
	return keep
}

func (a *kosyncAuth) limited(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.recent(ip, time.Now())) >= kosyncMaxFails
}

func (a *kosyncAuth) fail(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if a.fails == nil || len(a.fails) > 10000 {
		a.fails = map[string][]time.Time{}
	}
	a.fails[ip] = append(a.recent(ip, now), now)
}

// clientIP is the address failures are counted against: Cloudflare's view of
// the client when the request came through a tunnel, else the remote address.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" && net.ParseIP(ip) != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) handleKosyncPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Document   string   `json:"document"`
		Progress   string   `json:"progress"`
		Percentage *float64 `json:"percentage"`
		Device     string   `json:"device"`
		DeviceID   string   `json:"device_id"`
		Metadata   *struct {
			Title    string `json:"title"`
			Authors  string `json:"authors"`
			Filename string `json:"filename"`
		} `json:"metadata"`
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || json.Unmarshal(raw, &in) != nil {
		kosyncErr(w, http.StatusForbidden, 2003, "Invalid request")
		return
	}
	if !kosyncDocumentRE.MatchString(in.Document) {
		kosyncErr(w, http.StatusForbidden, 2004, "Field 'document' not provided.")
		return
	}
	if in.Percentage == nil || math.IsNaN(*in.Percentage) || in.Progress == "" || in.Device == "" {
		kosyncErr(w, http.StatusForbidden, 2003, "Invalid request")
		return
	}
	d := &store.KosyncDocument{
		Document:   in.Document,
		Progress:   clip(in.Progress, 1024),
		Percentage: math.Max(0, math.Min(1, *in.Percentage)),
		Device:     clip(in.Device, 100),
		DeviceID:   clip(in.DeviceID, 100),
		Timestamp:  time.Now().Unix(),
	}
	if m := in.Metadata; m != nil {
		d.Title, d.Authors, d.Filename = clip(strings.TrimSpace(m.Title), 512), clip(strings.TrimSpace(m.Authors), 512), clip(strings.TrimSpace(m.Filename), 512)
	}
	u := userFrom(r.Context())
	if err := s.store.SaveKosyncProgress(r.Context(), u.ID, d); err != nil {
		s.log.Error("kosync save", "err", err)
		kosyncErr(w, http.StatusInternalServerError, 2000, "Unknown server error")
		return
	}
	if err := s.store.AddKosyncEvent(r.Context(), u.ID, &store.KosyncEvent{Document: d.Document, Source: "device", Device: d.Device,
		DeviceID: d.DeviceID, Percentage: d.Percentage, Timestamp: d.Timestamp, Body: raw}); err != nil {
		s.log.Warn("kosync event", "err", err)
	}
	if err := s.applyKosync(r.Context(), u, d.Document, ""); err != nil {
		s.log.Warn("kosync apply", "user", u.ID, "document", d.Document, "err", err)
	}
	// A mirror sends the file hash only: ask CrossPoint Sync what the book is.
	if saved, err := s.store.GetKosyncDocument(r.Context(), u.ID, d.Document); err == nil && saved.BookID == nil && saved.Title == "" && !saved.Ignored {
		s.kickKosyncPull(u.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": d.Document, "timestamp": d.Timestamp})
}

func (s *Server) handleKosyncGet(w http.ResponseWriter, r *http.Request) {
	doc := chi.URLParam(r, "document")
	if !kosyncDocumentRE.MatchString(doc) {
		kosyncErr(w, http.StatusForbidden, 2004, "Field 'document' not provided.")
		return
	}
	d, err := s.store.GetKosyncDocument(r.Context(), me(r), doc)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if err != nil {
		kosyncErr(w, http.StatusInternalServerError, 2000, "Unknown server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"document": d.Document, "progress": d.Progress, "percentage": d.Percentage,
		"device": d.Device, "device_id": d.DeviceID, "timestamp": d.Timestamp,
	})
}

// applyKosync carries a document's latest position over to its book, matching
// the book first when the document is new, or adding it to the shelf when it
// is not there. status is what CrossPoint Sync says ("finished", "dnf"), if anything.
func (s *Server) applyKosync(ctx context.Context, u *store.User, document, status string) error {
	s.kosyncMu.Lock()
	defer s.kosyncMu.Unlock()
	d, err := s.store.GetKosyncDocument(ctx, u.ID, document)
	if err != nil || d.Ignored {
		return err
	}
	if d.BookID == nil {
		id, ambiguous := s.matchKosync(ctx, u.ID, d)
		if id == 0 && !ambiguous && d.Title != "" {
			if prefs, err := s.store.GetKosyncPrefs(ctx, u.ID); err == nil && prefs.AddMissing {
				if id, err = s.createKosyncBook(ctx, u, d); err != nil {
					return err
				}
			}
		}
		if id == 0 {
			return nil
		}
		if err := s.store.LinkKosyncDocument(ctx, u.ID, d.Document, &id, false); err != nil {
			return err
		}
		d.BookID = &id
		s.log.Info("kosync linked", "user", u.ID, "document", d.Document, "book", id)
	}
	return s.kosyncToBook(ctx, u, *d.BookID, d, status)
}

func (s *Server) kosyncToBook(ctx context.Context, u *store.User, bookID int64, d *store.KosyncDocument, status string) error {
	b, err := s.store.GetBook(ctx, u.ID, bookID)
	if err != nil {
		return err
	}
	if b.Status != store.StatusWant && b.Status != store.StatusReading {
		return nil // finished or abandoned: a device reopening it changes nothing
	}
	pct := d.Percentage
	changed := false
	if b.Pages == 0 && d.PageCount > 0 {
		b.Pages, changed = d.PageCount, true
	}
	if b.Pages > 0 {
		if page := int(math.Round(pct * float64(b.Pages))); page != b.CurrentPage && page <= b.Pages {
			b.CurrentPage, changed = page, true
		}
	}
	if changed {
		if err := s.store.UpdateBook(ctx, b); err != nil {
			return err
		}
	}
	day := ""
	if d.Timestamp > 0 {
		day = time.Unix(d.Timestamp, 0).Format("2006-01-02")
	}
	switch {
	case status == "finished":
		_, err = s.store.SetStatus(ctx, u.ID, b.ID, store.StatusRead, day, s.bookStatusEvent(ctx, u, day))
	case status == "dnf":
		_, err = s.store.SetStatus(ctx, u.ID, b.ID, store.StatusAbandoned, day, s.bookStatusEvent(ctx, u, day))
	case b.Status == store.StatusWant && pct > 0:
		if b, err = s.store.SetStatus(ctx, u.ID, b.ID, store.StatusReading, "", s.bookStatusEvent(ctx, u, "")); err != nil {
			return err
		}
		s.askFinished(ctx, u.ID, b, d)
	default:
		s.askFinished(ctx, u.ID, b, d)
	}
	return err
}

// matchKosync finds the one book on the shelf the document is: same title
// (and authors, when several books share it), books being read first. 0 when
// the device said nothing usable or no book has that title; ambiguous when
// several do and it is not clear which.
func (s *Server) matchKosync(ctx context.Context, userID int64, d *store.KosyncDocument) (id int64, ambiguous bool) {
	titles, authors := kosyncNames(d)
	if len(titles) == 0 {
		return 0, false
	}
	books, err := s.store.ListBooks(ctx, store.BookFilter{UserID: userID})
	if err != nil {
		return 0, true
	}
	var hits []store.Book
	for _, b := range books {
		t, full := fold.String(b.Title), fold.String(b.Title+" "+b.Subtitle)
		for _, want := range titles {
			if want == t || want == full {
				hits = append(hits, b)
				break
			}
		}
	}
	if len(hits) > 1 && len(authors) > 0 {
		var byAuthor []store.Book
		for _, b := range hits {
			if kosyncAuthorsMatch(b.Authors, authors) {
				byAuthor = append(byAuthor, b)
			}
		}
		if len(byAuthor) > 0 {
			hits = byAuthor
		}
	}
	if len(hits) > 1 {
		var open []store.Book
		for _, b := range hits {
			if b.Status == store.StatusReading || b.Status == store.StatusWant {
				open = append(open, b)
			}
		}
		hits = open
	}
	if len(hits) != 1 {
		return 0, len(hits) > 1
	}
	return hits[0].ID, false
}

// kosyncNames are the folded titles and author names a document may be: from
// the metadata, else from a file name like "Title - Author.epub".
func kosyncNames(d *store.KosyncDocument) (titles, authors []string) {
	add := func(list *[]string, v string) {
		if f := fold.String(v); f != "" {
			*list = append(*list, f)
		}
	}
	if d.Title != "" {
		add(&titles, d.Title)
		if i := strings.IndexAny(d.Title, ":("); i > 0 {
			add(&titles, d.Title[:i])
		}
	}
	for _, a := range splitList(strings.ReplaceAll(d.Authors, "\n", ",")) {
		add(&authors, a)
	}
	if len(titles) == 0 && d.Filename != "" {
		name := strings.TrimSuffix(path.Base(d.Filename), path.Ext(d.Filename))
		for _, part := range strings.Split(name, " - ") {
			add(&titles, part)
			add(&authors, part)
		}
	}
	return titles, authors
}

func kosyncAuthorsMatch(bookAuthors string, authors []string) bool {
	for _, a := range splitList(bookAuthors) {
		fa := fold.Name(a)
		for _, want := range authors {
			if fa == want || fold.Name(want) == fa {
				return true
			}
		}
	}
	return false
}

// ---- Settings → Reader sync ----

func (s *Server) kosyncURL() string {
	if s.cfg.KosyncPublicURL != "" {
		return s.cfg.KosyncPublicURL
	}
	return s.cfg.BaseURL + "/kosync"
}

func (s *Server) handleGetKosync(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"url": s.kosyncURL(), "account": nil}
	acc, err := s.store.GetKosyncAccount(r.Context(), me(r))
	switch {
	case err == nil:
		out["account"] = acc
	case !errors.Is(err, store.ErrNotFound):
		s.fail(w, err, "get kosync account")
		return
	}
	docs, err := s.store.ListKosyncDocuments(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "list kosync documents")
		return
	}
	out["documents"] = docs
	prefs, err := s.store.GetKosyncPrefs(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "get kosync prefs")
		return
	}
	out["prefs"] = prefs
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSaveKosyncAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !kosyncUsernameRE.MatchString(in.Username) {
		writeErr(w, http.StatusBadRequest, "the username can have up to 64 letters, digits and . _ @ + -")
		return
	}
	_, err := s.store.GetKosyncAccount(r.Context(), me(r))
	isNew := errors.Is(err, store.ErrNotFound)
	if err != nil && !isNew {
		s.fail(w, err, "get kosync account")
		return
	}
	hash := ""
	if in.Password != "" || isNew {
		if len(in.Password) < 8 {
			writeErr(w, http.StatusBadRequest, "the password needs at least 8 characters")
			return
		}
		sum := md5.Sum([]byte(in.Password))
		h, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(sum[:])), bcrypt.DefaultCost)
		if err != nil {
			s.fail(w, err, "hash")
			return
		}
		hash = string(h)
	}
	if err := s.store.SaveKosyncAccount(r.Context(), me(r), in.Username, hash); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "someone else on this box already uses that sync username")
			return
		}
		s.fail(w, err, "save kosync account")
		return
	}
	s.handleGetKosync(w, r)
}

func (s *Server) handleDeleteKosyncAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteKosyncAccount(r.Context(), me(r)); err != nil {
		s.fail(w, err, "delete kosync account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLinkKosyncDocument(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BookID  *int64 `json:"book_id"`
		Ignored bool   `json:"ignored"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	doc := chi.URLParam(r, "document")
	if err := s.store.LinkKosyncDocument(r.Context(), me(r), doc, in.BookID, in.Ignored); err != nil {
		s.fail(w, err, "link kosync document")
		return
	}
	d, err := s.store.GetKosyncDocument(r.Context(), me(r), doc)
	if err != nil {
		s.fail(w, err, "get kosync document")
		return
	}
	if d.BookID != nil && !d.Ignored {
		if err := s.kosyncToBook(r.Context(), userFrom(r.Context()), *d.BookID, d, ""); err != nil {
			s.fail(w, err, "apply progress")
			return
		}
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteKosyncDocument(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteKosyncDocument(r.Context(), me(r), chi.URLParam(r, "document")); err != nil {
		s.fail(w, err, "delete kosync document")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
