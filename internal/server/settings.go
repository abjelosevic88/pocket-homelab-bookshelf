package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(st))
}

// publicSettings adds what the UI needs to know about the environment.
func (s *Server) publicSettings(st store.Settings) map[string]any {
	return map[string]any{
		"title": st.Title, "yearly_goal": st.YearlyGoal, "landing": st.Landing, "search_source": st.SearchSource,
		"google_books_key": st.GoogleBooksKey, "google_books_env": s.cfg.GoogleBooksKey != "", "sources": s.meta.Sources(),
		"languages": st.Languages, "thought_templates": st.ThoughtTemplates,
		"setup_complete": st.SetupComplete,
	}
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in store.Settings
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = store.DefaultTitle
	}
	if in.YearlyGoal < 0 || in.YearlyGoal > 10000 {
		in.YearlyGoal = 0
	}
	if in.Landing != "library" && in.Landing != "planner" {
		in.Landing = "now"
	}
	if in.SearchSource != "openlibrary" && in.SearchSource != "google" {
		in.SearchSource = "all"
	}
	in.GoogleBooksKey = strings.TrimSpace(in.GoogleBooksKey)
	var langs []string
	for _, l := range strings.Split(strings.ToLower(in.Languages), ",") {
		if l = strings.TrimSpace(l); metadata.MARCCode(l) != "" && !slices.Contains(langs, l) {
			langs = append(langs, l)
		}
	}
	in.Languages = strings.Join(langs, ",")
	if in.ThoughtTemplates == nil {
		in.ThoughtTemplates = cur.ThoughtTemplates
	}
	var tpls []store.ThoughtTemplate
	for _, t := range in.ThoughtTemplates {
		t.Name, t.Body = strings.TrimSpace(t.Name), strings.TrimRight(t.Body, " \n")+"\n"
		if t.Name != "" && len(tpls) < 12 && len(t.Body) < 20000 {
			tpls = append(tpls, t)
		}
	}
	in.ThoughtTemplates = tpls
	in.SetupComplete = cur.SetupComplete
	if err := s.store.SaveSettings(r.Context(), in); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if in.Languages != cur.Languages {
		s.kickCharacters() // a new preferred language needs characters
	}
	writeJSON(w, http.StatusOK, s.publicSettings(in))
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	var coverBytes int64
	var covers int
	_ = filepath.Walk(s.cfg.CoverDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			coverBytes += info.Size()
			covers++
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"cover_dir": s.cfg.CoverDir, "covers": covers, "cover_bytes": coverBytes,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.Counts(r.Context()), "sources": s.meta.Sources(),
	})
}

// coreStatus summarises the Pocket Core link for the About page.
func (s *Server) coreStatus() map[string]any {
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return map[string]any{"configured": s.core.Configured(), "url": s.cfg.CoreURL, "registered": s.coreOK, "error": s.coreErr}
}

// ---------- backup / export / import ----------

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-bookshelf-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	defer os.Remove(dst)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, dst)
}

// ExportBook is a book with everything hanging off it.
type ExportBook struct {
	store.Book
	CoverURL string        `json:"cover_url,omitempty"`
	Reads    []store.Read  `json:"reads"`
	Quotes   []store.Quote `json:"quotes"`
}

// Export is the portable JSON shape of one user's shelf (also accepted by /import).
type Export struct {
	Version    string         `json:"version"`
	ExportedAt string         `json:"exported_at"`
	Books      []ExportBook   `json:"books"`
	Authors    []store.Author `json:"authors"`
}

func (s *Server) buildExport(ctx context.Context, userID int64) (*Export, error) {
	books, err := s.store.ListBooks(ctx, store.BookFilter{UserID: userID, Sort: "added"})
	if err != nil {
		return nil, err
	}
	ex := &Export{Version: Version, ExportedAt: store.Now(), Books: []ExportBook{}}
	if ex.Authors, err = s.store.ListAuthors(ctx, userID, "", "name"); err != nil {
		return nil, err
	}
	for _, b := range books {
		reads, err := s.store.ListReads(ctx, b.ID)
		if err != nil {
			return nil, err
		}
		quotes, err := s.store.ListQuotes(ctx, store.QuoteFilter{UserID: userID, BookID: b.ID})
		if err != nil {
			return nil, err
		}
		ex.Books = append(ex.Books, ExportBook{Book: b, Reads: reads, Quotes: quotes})
	}
	return ex, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ex, err := s.buildExport(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-bookshelf-export.json"`)
	writeJSON(w, http.StatusOK, ex)
}

var csvHeader = []string{"title", "subtitle", "authors", "isbn13", "isbn10", "pages", "published", "publisher", "language", "format", "tags",
	"status", "rating", "review", "favorite", "started_on", "finished_on", "read_count", "quotes", "planned_for", "plan_note", "source", "source_url", "added"}

// handleExportCSV writes one row per book — readable in a spreadsheet and
// accepted again by /import/goodreads.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	books, err := s.store.ListBooks(r.Context(), store.BookFilter{UserID: me(r), Sort: "added"})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-bookshelf-export.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHeader)
	num := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	for _, b := range books {
		rating, fav := "", ""
		if b.Rating > 0 {
			rating = strconv.FormatFloat(b.Rating, 'f', -1, 64)
		}
		if b.Favorite {
			fav = "yes"
		}
		_ = cw.Write([]string{b.Title, b.Subtitle, b.Authors, b.ISBN13, b.ISBN10, num(b.Pages), b.Published, b.Publisher, b.Language, b.Format, b.Tags,
			b.Status, rating, b.Review, fav, b.StartedOn, b.FinishedOn, num(b.ReadCount), num(b.QuoteCount), b.PlannedFor, b.PlanNote, b.Source, b.SourceURL, b.CreatedAt[:min(10, len(b.CreatedAt))]})
	}
	cw.Flush()
}

// handleImportJSON adds the books of an export file that are not on the shelf yet.
func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	var ex Export
	if err := decode(r, &ex); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	created, skipped := 0, 0
	var newIDs []int64
	for _, eb := range ex.Books {
		b := eb.Book
		b.ID, b.UserID, b.Cover = 0, me(r), ""
		if msg := clean(&b); msg != "" {
			skipped++
			continue
		}
		if s.findDuplicate(r, &b) != nil {
			skipped++
			continue
		}
		if err := s.store.ImportBook(r.Context(), &b, eb.Reads, eb.Quotes); err != nil {
			s.fail(w, err, "import")
			return
		}
		created++
		newIDs = append(newIDs, b.ID)
	}
	s.fetchMissingCovers(me(r), newIDs)
	// Authors: keep the ones followed by hand and hand-edited biographies.
	for _, ea := range ex.Authors {
		if !ea.Followed && !ea.Edited {
			continue
		}
		a, err := s.store.FindAuthorByName(r.Context(), me(r), ea.Name)
		if err != nil {
			if !ea.Followed {
				continue
			}
			a = &store.Author{UserID: me(r), Name: ea.Name, OLKey: ea.OLKey}
			if err := s.store.CreateAuthor(r.Context(), a); err != nil {
				continue
			}
			if a, err = s.store.GetAuthor(r.Context(), me(r), a.ID); err != nil {
				continue
			}
		}
		if ea.Edited {
			a.Bio, a.BioSource, a.BirthDate, a.DeathDate, a.Edited = ea.Bio, ea.BioSource, ea.BirthDate, ea.DeathDate, true
		}
		a.Followed = a.Followed || ea.Followed
		_ = s.store.UpdateAuthor(r.Context(), a)
	}
	s.kickAuthors()
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "skipped": skipped})
}
