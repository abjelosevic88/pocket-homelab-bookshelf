package server

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Characters are written outside the app — by a language model through the
// API (scripts/characters.py lists what is missing and sends results back) —
// in each preferred language, and shown on the book page.

var langCode = regexp.MustCompile(`^[a-z]{2,3}$`)

func (s *Server) characterLangs() []string {
	langs := s.languages()
	if len(langs) == 0 {
		return []string{"en"}
	}
	return langs
}

func (s *Server) handleGetCharacters(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	sets, err := s.store.Characters(r.Context(), b.ID)
	if err != nil {
		s.fail(w, err, "characters")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"langs": s.characterLangs(), "sets": sets, "writer": s.characterWriter(r.Context())})
}

// handlePutCharacters saves characters in one or more languages:
// {"source": "…", "sets": {"en": {"status": "ok", "characters": [{name, role, description}]}}}.
func (s *Server) handlePutCharacters(w http.ResponseWriter, r *http.Request) {
	b, err := s.characterBook(r)
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	var in struct {
		Source string                        `json:"source"`
		Sets   map[string]store.CharacterSet `json:"sets"`
	}
	if err := decode(r, &in); err != nil || len(in.Sets) == 0 {
		writeErr(w, http.StatusBadRequest, `send {"sets": {"en": {"status": "ok", "characters": [...]}}}`)
		return
	}
	for lang, cs := range in.Sets {
		if !langCode.MatchString(lang) {
			writeErr(w, http.StatusBadRequest, "language codes are ISO 639-1, like en or sr")
			return
		}
		if cs.Status == "" {
			cs.Status = "ok"
		}
		if !store.ValidCharacterStatus(cs.Status) {
			writeErr(w, http.StatusBadRequest, "status is ok, none or unknown")
			return
		}
		in.Sets[lang] = cleanCharacters(cs, in.Source)
	}
	for lang, cs := range in.Sets {
		if err := s.store.PutCharacters(r.Context(), b.ID, lang, cs); err != nil {
			s.fail(w, err, "save characters")
			return
		}
	}
	sets, _ := s.store.Characters(r.Context(), b.ID)
	writeJSON(w, http.StatusOK, map[string]any{"langs": s.characterLangs(), "sets": sets})
}

// characterBook is the book a write is for: the user's own, or any book for
// an admin (the character writer runs with an admin token for the household).
func (s *Server) characterBook(r *http.Request) (*store.Book, error) {
	id := idParam(r, "id")
	if u := userFrom(r.Context()); u != nil && u.Role == "admin" {
		return s.store.GetBookAnyUser(r.Context(), id)
	}
	return s.store.GetBook(r.Context(), me(r), id)
}

func cleanCharacters(cs store.CharacterSet, source string) store.CharacterSet {
	out := store.CharacterSet{Status: cs.Status, Source: clip(strings.TrimSpace(source), 60), Characters: []store.Character{}}
	if cs.Status != "ok" {
		return out
	}
	for _, c := range cs.Characters {
		c.Name, c.Description = clip(strings.TrimSpace(c.Name), 120), clip(strings.TrimSpace(c.Description), 800)
		if c.Name == "" {
			continue
		}
		if c.Role != "main" {
			c.Role = "supporting"
		}
		out.Characters = append(out.Characters, c)
		if len(out.Characters) == 40 {
			break
		}
	}
	return out
}

// handleDeleteCharacters forgets a book's characters so they are written again.
func (s *Server) handleDeleteCharacters(w http.ResponseWriter, r *http.Request) {
	b, err := s.characterBook(r)
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	if err := s.store.DeleteCharacters(r.Context(), b.ID); err != nil {
		s.fail(w, err, "delete characters")
		return
	}
	s.chars.mu.Lock()
	delete(s.chars.retryAt, b.ID)
	s.chars.mu.Unlock()
	s.kickCharacters()
	w.WriteHeader(http.StatusNoContent)
}

// handleMissingCharacters lists books without characters in some preferred
// language, with what a writer needs to identify the book. Admins get every
// user's books.
func (s *Server) handleMissingCharacters(w http.ResponseWriter, r *http.Request) {
	uid := me(r)
	if u := userFrom(r.Context()); u != nil && u.Role == "admin" {
		uid = 0
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	books, missing, err := s.store.MissingCharacters(r.Context(), uid, s.characterLangs(), limit)
	if err != nil {
		s.fail(w, err, "characters")
		return
	}
	type item struct {
		ID          int64    `json:"id"`
		Title       string   `json:"title"`
		Subtitle    string   `json:"subtitle,omitempty"`
		WorkTitle   string   `json:"work_title,omitempty"` // the original-language title, from Open Library
		Authors     string   `json:"authors"`
		Published   string   `json:"published,omitempty"`
		Language    string   `json:"language,omitempty"`
		Series      string   `json:"series,omitempty"`
		SeriesNo    string   `json:"series_no,omitempty"`
		Description string   `json:"description,omitempty"`
		SourceURL   string   `json:"source_url,omitempty"`
		Missing     []string `json:"missing"`
	}
	out := make([]item, len(books))
	for i, b := range books {
		it := item{ID: b.ID, Title: b.Title, Subtitle: b.Subtitle, Authors: b.Authors, Published: b.Published, Language: b.Language,
			Series: b.Series, SeriesNo: b.SeriesNo, Description: clip(b.Description, 1500), SourceURL: b.SourceURL, Missing: missing[i]}
		if b.Source == "openlibrary" && strings.HasPrefix(b.SourceID, "/works/") {
			if t := s.meta.OpenLibraryWorkTitle(r.Context(), b.SourceID); t != "" && t != b.Title {
				it.WorkTitle = t
			}
		}
		out[i] = it
	}
	// "writer": "app" — the app writes characters itself (it has an OpenAI key); external writers stand aside.
	writeJSON(w, http.StatusOK, map[string]any{"langs": s.characterLangs(), "books": out, "writer": s.characterWriter(r.Context())})
}
