// Package server wires the HTTP API, auth and the embedded SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/config"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Version is injected at build time.
var Version = "dev"

// Server holds dependencies.
type Server struct {
	cfg    *config.Config
	store  *store.Store
	meta   *metadata.Client
	covers coverJob

	authorKick chan struct{}
	log        *slog.Logger
	webFS      fs.FS
	started    time.Time
	requests   atomic.Int64
	errors     atomic.Int64
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	s := &Server{cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now(), authorKick: make(chan struct{}, 1)}
	st.OnAuthorsChanged = s.kickAuthors
	ua := "PocketBookshelf/" + Version + " (+https://github.com/abjelosevic88/pocket-homelab-bookshelf)"
	s.meta = metadata.New(ua, cfg.MetadataTimeout, s.googleKey)
	s.meta.Langs = s.languages
	s.meta.Country = cfg.GoogleBooksCountry
	return s
}

// googleKey prefers the key saved in Settings over GOOGLE_BOOKS_API_KEY.
func (s *Server) googleKey() string {
	if st, err := s.store.GetSettings(context.Background()); err == nil && st.GoogleBooksKey != "" {
		return st.GoogleBooksKey
	}
	return s.cfg.GoogleBooksKey
}

// languages are the preferred edition languages from Settings → Metadata.
func (s *Server) languages() []string {
	st, err := s.store.GetSettings(context.Background())
	if err != nil || st.Languages == "" {
		return nil
	}
	return strings.Split(st.Languages, ",")
}

// Bootstrap seeds the first admin (if configured) and demo data.
func (s *Server) Bootstrap(ctx context.Context) error {
	if err := os.MkdirAll(s.cfg.CoverDir, 0o755); err != nil {
		return fmt.Errorf("create cover dir: %w", err)
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && s.cfg.AuthMode == "local" && s.cfg.AdminEmail != "" && s.cfg.AdminPassword != "" {
		if len(s.cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD must be at least 8 characters")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
		u := &store.User{Email: s.cfg.AdminEmail, Name: s.cfg.AdminName, PasswordHash: string(h), Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		st, _ := s.store.GetSettings(ctx)
		st.SetupComplete = true
		if err := s.store.SaveSettings(ctx, st); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	if s.cfg.DemoData {
		if err := s.seedDemo(ctx); err != nil {
			return err
		}
	}
	_ = s.store.PurgeSessions(ctx)
	// Link existing books to authors, then look new authors up in the background.
	if err := s.store.SyncAllAuthors(ctx); err != nil {
		return err
	}
	go s.authorWorker(ctx)
	s.kickAuthors()
	return nil
}

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Get("/auth/status", s.handleAuthStatus)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			r.Get("/dashboard", s.handleDashboard)
			r.Get("/stats", s.handleStats)
			r.Get("/search", s.handleSearch)
			r.Get("/search/details", s.handleSearchDetails)

			r.Get("/books", s.handleListBooks)
			r.Post("/books", s.handleCreateBook)
			r.Get("/books/{id}", s.handleGetBook)
			r.Put("/books/{id}", s.handleUpdateBook)
			r.Delete("/books/{id}", s.handleDeleteBook)
			r.Post("/books/{id}/status", s.handleSetStatus)
			r.Post("/books/{id}/progress", s.handleProgress)
			r.Get("/books/{id}/cover", s.handleGetCover)
			r.Get("/books/{id}/covers", s.handleCoverSuggestions)
			r.Post("/books/{id}/cover", s.handleSetCover)
			r.Delete("/books/{id}/cover", s.handleDeleteCover)
			r.Post("/covers/fetch-missing", s.handleFetchCovers)
			r.Get("/covers/fetch-missing", s.handleCoverStatus)
			r.Post("/books/{id}/reads", s.handleAddRead)
			r.Put("/reads/{id}", s.handleUpdateRead)
			r.Delete("/reads/{id}", s.handleDeleteRead)
			r.Post("/books/{id}/quotes", s.handleCreateQuote)
			r.Get("/quotes", s.handleListQuotes)
			r.Put("/quotes/{id}", s.handleUpdateQuote)
			r.Delete("/quotes/{id}", s.handleDeleteQuote)
			r.Put("/planner/order", s.handleReorderPlanner)

			r.Get("/authors", s.handleListAuthors)
			r.Post("/authors", s.handleCreateAuthor)
			r.Get("/authors/search", s.handleSearchAuthors)
			r.Get("/authors/resolve", s.handleResolveAuthor)
			r.Get("/authors/{id}", s.handleGetAuthor)
			r.Put("/authors/{id}", s.handleUpdateAuthor)
			r.Delete("/authors/{id}", s.handleDeleteAuthor)
			r.Get("/authors/{id}/works", s.handleAuthorWorks)
			r.Post("/authors/{id}/refresh", s.handleRefreshAuthor)
			r.Get("/authors/{id}/photo", s.handleGetAuthorPhoto)
			r.Get("/authors/{id}/photos", s.handleAuthorPhotoSuggestions)
			r.Post("/authors/{id}/photo", s.handleSetAuthorPhoto)
			r.Delete("/authors/{id}/photo", s.handleDeleteAuthorPhoto)
			r.Get("/tags", s.handleListTags)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
			r.Get("/export.json", s.handleExportJSON)
			r.Get("/export.csv", s.handleExportCSV)
			r.Post("/import", s.handleImportJSON)
			r.Post("/import/goodreads", s.handleImportGoodreads)
			r.Post("/import/kindle/preview", s.handleKindlePreview)
			r.Post("/import/kindle", s.handleKindleImport)
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "that email is already in use")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	return dec.Decode(v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	_, err := store.ParseDate(s)
	return err == nil
}

func validMonth(s string) bool {
	_, err := time.Parse("2006-01", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_bookshelf_info Build info.\n# TYPE pocket_bookshelf_info gauge\npocket_bookshelf_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_bookshelf_uptime_seconds Process uptime.\n# TYPE pocket_bookshelf_uptime_seconds gauge\npocket_bookshelf_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_bookshelf_http_requests_total HTTP requests served.\n# TYPE pocket_bookshelf_http_requests_total counter\npocket_bookshelf_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_bookshelf_http_errors_total HTTP 5xx responses.\n# TYPE pocket_bookshelf_http_errors_total counter\npocket_bookshelf_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_bookshelf_rows Rows per table.\n# TYPE pocket_bookshelf_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_bookshelf_rows{table=%q} %d\n", k, v)
	}
	var reading, read, want int
	_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(CASE WHEN status='reading' THEN 1 END), COUNT(CASE WHEN status='read' THEN 1 END), COUNT(CASE WHEN status='want' THEN 1 END) FROM books`).Scan(&reading, &read, &want)
	fmt.Fprintf(w, "# HELP pocket_bookshelf_books Books by status, all users.\n# TYPE pocket_bookshelf_books gauge\n")
	fmt.Fprintf(w, "pocket_bookshelf_books{status=\"reading\"} %d\npocket_bookshelf_books{status=\"read\"} %d\npocket_bookshelf_books{status=\"want\"} %d\n", reading, read, want)
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
