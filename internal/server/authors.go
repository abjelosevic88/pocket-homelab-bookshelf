package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/fold"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/metadata"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Authors are created from the books' author strings. A background worker
// looks each new one up on Open Library (bio from Wikipedia when linked),
// one at a time, and downloads a photo.

// kickAuthors wakes the background worker without blocking.
func (s *Server) kickAuthors() {
	s.kickCharacters() // books changed: some may need characters
	select {
	case s.authorKick <- struct{}{}:
	default:
	}
}

func (s *Server) authorWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.authorKick:
		}
		for ctx.Err() == nil {
			pending, err := s.store.PendingAuthors(ctx, 20)
			if err != nil || len(pending) == 0 {
				break
			}
			progress := false
			for i := range pending {
				a := &pending[i]
				if err := s.enrichAuthor(ctx, a); err != nil {
					s.log.Warn("author lookup failed, will retry later", "author", a.Name, "err", err)
					continue
				}
				progress = true
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second): // gentle on Open Library and Wikipedia
				}
			}
			if !progress { // catalogues unreachable: try again on the next kick
				break
			}
		}
		// Authors are linked now: look new books up for series, then fetch
		// what the series pages show so they open instantly.
		s.detectSeries(ctx)
		s.warmSeries(ctx)
	}
}

// namesMatch: does any of the catalogue's names belong to the author on the shelf?
func namesMatch(want string, names []string) bool {
	w := fold.Name(want)
	for _, n := range names {
		if fold.Name(n) == w {
			return true
		}
	}
	ln := fold.LastName(want)
	for _, n := range names {
		if ln != "" && fold.LastName(n) == ln {
			return true
		}
	}
	return false
}

// resolveAuthorKey finds the Open Library key of a shelf author: through the
// works of their books first (exact), then by name search.
func (s *Server) resolveAuthorKey(ctx context.Context, a *store.Author) (string, *metadata.AuthorInfo, error) {
	if a.OLKey != "" {
		info, err := s.meta.AuthorDetails(ctx, a.OLKey)
		return a.OLKey, info, err
	}
	keys, books, err := s.store.AuthorWorkKeys(ctx, a.ID)
	if err != nil {
		return "", nil, err
	}
	tried := map[string]bool{}
	n := 0
	for workKey, bookID := range keys {
		if n++; n > 3 {
			break
		}
		authorKeys, err := s.meta.WorkAuthorKeys(ctx, workKey)
		if err != nil {
			return "", nil, err
		}
		single := false
		for _, b := range books {
			if b.ID == bookID {
				full, _ := s.store.GetBook(ctx, a.UserID, b.ID)
				single = full != nil && len(store.SplitAuthors(full.Authors)) == 1
			}
		}
		for _, k := range authorKeys {
			if tried[k] {
				continue
			}
			tried[k] = true
			info, err := s.meta.AuthorDetails(ctx, k)
			if err != nil {
				return "", nil, err
			}
			if namesMatch(a.Name, info.Names) || (single && len(authorKeys) == 1) {
				// Open Library has duplicate, nearly empty author records and a work
				// may point at one. When the name search finds the same person with
				// far more works, that is the real record.
				if k2, n2, n1 := s.bestByName(ctx, a.Name, k); k2 != "" && k2 != k && n2 > 2*n1+5 {
					if info2, err := s.meta.AuthorDetails(ctx, k2); err == nil {
						return k2, info2, nil
					}
				}
				return k, info, nil
			}
		}
	}
	k, info := s.searchAuthorKey(ctx, a.Name)
	return k, info, nil
}

// bestByName returns the most prolific search result whose name matches,
// its work count, and the work count of `other` among the same results.
func (s *Server) bestByName(ctx context.Context, name, other string) (key string, works, otherWorks int) {
	found, err := s.meta.SearchAuthors(ctx, name, 10)
	if err != nil {
		return "", 0, 0
	}
	for _, c := range found {
		if c.Key == other {
			otherWorks = c.WorkCount
		}
		exact := fold.Name(c.Name) == fold.Name(name)
		if !exact && !(namesMatch(name, c.AllNames()) && c.WorkCount > 0) {
			continue
		}
		if key == "" || c.WorkCount > works {
			key, works = c.Key, c.WorkCount
		}
	}
	return key, works, otherWorks
}

// searchAuthorKey finds an author by name alone.
func (s *Server) searchAuthorKey(ctx context.Context, name string) (string, *metadata.AuthorInfo) {
	key, _, _ := s.bestByName(ctx, name, "")
	if key == "" {
		return "", nil
	}
	info, err := s.meta.AuthorDetails(ctx, key)
	if err != nil {
		return "", nil
	}
	return key, info
}

// enrichAuthor fills bio, dates, Wikipedia link and photo from the catalogues.
// The name on the shelf is kept; hand-edited fields are not overwritten.
func (s *Server) enrichAuthor(ctx context.Context, a *store.Author) error {
	key, info, err := s.resolveAuthorKey(ctx, a)
	if err != nil {
		return err
	}
	if key == "" || info == nil {
		return s.store.MarkEnriched(ctx, a.ID)
	}
	a.OLKey = key
	if !a.Edited || a.Bio == "" {
		a.Bio, a.BioFull, a.BioSource = info.Bio, info.BioFull, info.BioSource
	}
	if !a.Edited || a.BirthDate == "" {
		a.BirthDate = info.BirthDate
	}
	if !a.Edited || a.DeathDate == "" {
		a.DeathDate = info.DeathDate
	}
	a.Wikipedia = info.Wikipedia
	if err := s.store.SaveEnrichment(ctx, a); err != nil {
		return err
	}
	if a.Photo == "" {
		for _, u := range []string{info.PhotoURL, "https://covers.openlibrary.org/a/olid/" + key + "-L.jpg?default=false"} {
			if u == "" {
				continue
			}
			if data, ct, err := s.meta.FetchCover(ctx, u, s.cfg.MaxCoverBytes); err == nil {
				_ = s.writeAuthorPhoto(ctx, a, data, ct)
				break
			}
		}
	}
	return nil
}

func (s *Server) writeAuthorPhoto(ctx context.Context, a *store.Author, data []byte, contentType string) error {
	name := "author-" + strconv.FormatInt(a.ID, 10) + "-" + store.RandomToken(4) + metadata.CoverExt[contentType]
	if err := os.WriteFile(filepath.Join(s.cfg.CoverDir, name), data, 0o644); err != nil {
		return err
	}
	if err := s.store.SetAuthorPhoto(ctx, a.ID, name); err != nil {
		_ = os.Remove(filepath.Join(s.cfg.CoverDir, name))
		return err
	}
	s.removeCoverFile(a.Photo)
	a.Photo = name
	return nil
}

// ---- handlers ----

func (s *Server) handleListAuthors(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListAuthors(r.Context(), me(r), r.URL.Query().Get("q"), r.URL.Query().Get("sort"))
	if err != nil {
		s.fail(w, err, "list authors")
		return
	}
	for i := range list {
		list[i].Bio, list[i].BioFull = shortBio(list[i].Bio), ""
	}
	writeJSON(w, http.StatusOK, list)
}

// shortBio is the first sentence or two, for lists.
func shortBio(b string) string {
	if len(b) <= 220 {
		return b
	}
	cut := b[:220]
	if i := strings.LastIndex(cut, ". "); i > 80 {
		return cut[:i+1]
	}
	return strings.TrimSpace(cut) + "…"
}

func (s *Server) handleGetAuthor(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	books, err := s.store.AuthorBooks(r.Context(), me(r), a.ID)
	if err != nil {
		s.fail(w, err, "author books")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"author": a, "books": books})
}

func (s *Server) handleResolveAuthor(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.FindAuthorByName(r.Context(), me(r), r.URL.Query().Get("name"))
	if err != nil {
		s.fail(w, err, "find author")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": a.ID})
}

// handleSearchAuthors searches Open Library; results already on the shelf
// carry their author id.
func (s *Server) handleSearchAuthors(w http.ResponseWriter, r *http.Request) {
	found, err := s.meta.SearchAuthors(r.Context(), r.URL.Query().Get("q"), 12)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}})
		return
	}
	mine, _ := s.store.ListAuthors(r.Context(), me(r), "", "name")
	type result struct {
		metadata.AuthorCandidate
		AuthorID int64 `json:"author_id,omitempty"`
	}
	out := make([]result, 0, len(found))
	for _, c := range found {
		res := result{AuthorCandidate: c}
		for _, a := range mine {
			if (a.OLKey != "" && a.OLKey == c.Key) || fold.Name(a.Name) == fold.Name(c.Name) {
				res.AuthorID = a.ID
				break
			}
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// handleCreateAuthor adds an author by hand ({"name"} and/or {"key"}) and
// looks them up right away.
func (s *Server) handleCreateAuthor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Name, in.Key = strings.TrimSpace(in.Name), strings.TrimSpace(strings.TrimPrefix(in.Key, "/authors/"))
	if in.Name == "" && in.Key != "" {
		if info, err := s.meta.AuthorDetails(r.Context(), in.Key); err == nil {
			in.Name = info.Name
		}
	}
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	a := &store.Author{UserID: me(r), Name: in.Name, OLKey: in.Key}
	if err := s.store.CreateAuthor(r.Context(), a); err != nil {
		s.fail(w, err, "create author")
		return
	}
	full, err := s.store.GetAuthor(r.Context(), a.UserID, a.ID)
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	if full.EnrichedAt == "" {
		if err := s.enrichAuthor(r.Context(), full); err != nil {
			s.log.Warn("author lookup failed", "author", full.Name, "err", err)
		}
		full, _ = s.store.GetAuthor(r.Context(), a.UserID, a.ID)
	}
	writeJSON(w, http.StatusCreated, full)
}

// handleUpdateAuthor merges the fields present in the body. Changing bio or
// dates marks the author as edited, so lookups no longer overwrite them.
func (s *Server) handleUpdateAuthor(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "request too large")
		return
	}
	a := *cur
	if err := json.Unmarshal(body, &a); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	a.ID, a.UserID = cur.ID, cur.UserID
	a.Name = strings.Join(strings.Fields(a.Name), " ")
	if a.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.Contains(a.Name, ",") {
		writeErr(w, http.StatusBadRequest, "an author name cannot contain a comma (commas separate authors on a book)")
		return
	}
	if a.Bio != cur.Bio || a.BirthDate != cur.BirthDate || a.DeathDate != cur.DeathDate {
		a.Edited = true
		if a.Bio != cur.Bio {
			a.BioSource, a.BioFull = "", "" // the hand-written text replaces both
		}
	}
	if err := s.store.UpdateAuthor(r.Context(), &a); err != nil {
		s.fail(w, err, "update author")
		return
	}
	saved, err := s.store.GetAuthor(r.Context(), me(r), a.ID) // a.ID changes when renaming merged two authors
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleRefreshAuthor looks an author up again, optionally as another
// Open Library author ({"key": "OL31353A"}) when the first match was wrong.
func (s *Server) handleRefreshAuthor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	_ = decode(r, &in)
	id := idParam(r, "id")
	if err := s.store.ResetEnrichment(r.Context(), me(r), id, strings.TrimPrefix(in.Key, "/authors/")); err != nil {
		s.fail(w, err, "refresh author")
		return
	}
	a, err := s.store.GetAuthor(r.Context(), me(r), id)
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	if in.Key != "" {
		s.removeCoverFile(a.Photo)
		_ = s.store.SetAuthorPhoto(r.Context(), a.ID, "")
		a.Photo = ""
	}
	if err := s.enrichAuthor(r.Context(), a); err != nil {
		writeErr(w, http.StatusBadGateway, "Open Library did not answer, try again in a moment")
		return
	}
	a, _ = s.store.GetAuthor(r.Context(), me(r), id)
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteAuthor(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	if err := s.store.DeleteAuthor(r.Context(), a.UserID, a.ID); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "this author still has books on your shelf")
			return
		}
		s.fail(w, err, "delete author")
		return
	}
	s.removeCoverFile(a.Photo)
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthorWorks lists the author's bibliography from Open Library,
// marking works that are already on the shelf.
func (s *Server) handleAuthorWorks(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	if a.OLKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"works": []any{}, "linked": false})
		return
	}
	works, err := s.meta.AuthorWorks(r.Context(), a.OLKey, 80)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"works": []any{}, "linked": true, "unavailable": true})
		return
	}
	keys, books, _ := s.store.AuthorWorkKeys(r.Context(), a.ID)
	type work struct {
		metadata.Candidate
		OnShelf int64 `json:"on_shelf,omitempty"`
	}
	out := make([]work, 0, len(works))
	for _, c := range works {
		wk := work{Candidate: c, OnShelf: keys[c.SourceID]}
		if wk.OnShelf == 0 {
			for _, b := range books {
				if sameTitle(b.Title, c.Title) {
					wk.OnShelf = b.ID
					break
				}
			}
		}
		out = append(out, wk)
	}
	writeJSON(w, http.StatusOK, map[string]any{"works": out, "linked": true})
}

// ---- photos ----

// handleAuthorPhotoSuggestions lists pictures of the author to choose from.
func (s *Server) handleAuthorPhotoSuggestions(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	photos := s.meta.AuthorPhotoSuggestions(r.Context(), a.OLKey, a.Name, a.Wikipedia)
	if photos == nil {
		photos = []metadata.CoverOption{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"photos": photos})
}

func (s *Server) handleGetAuthorPhoto(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil || a.Photo == "" {
		writeErr(w, http.StatusNotFound, "no photo")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeFile(w, r, filepath.Join(s.cfg.CoverDir, filepath.Base(a.Photo)))
}

func (s *Server) handleSetAuthorPhoto(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	var data []byte
	var ct string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxCoverBytes+1<<20)
		if err := r.ParseMultipartForm(s.cfg.MaxCoverBytes); err != nil {
			writeErr(w, http.StatusBadRequest, "photo too large or malformed upload")
			return
		}
		f, _, err := r.FormFile("photo")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "form field 'photo' is required")
			return
		}
		defer f.Close()
		data, _ = io.ReadAll(io.LimitReader(f, s.cfg.MaxCoverBytes+1))
		ct = http.DetectContentType(data)
		if _, ok := metadata.CoverExt[ct]; !ok || int64(len(data)) > s.cfg.MaxCoverBytes {
			writeErr(w, http.StatusBadRequest, "photo must be a JPEG, PNG, WebP or GIF image under the size limit")
			return
		}
	} else {
		var in struct {
			URL string `json:"url"`
		}
		if err := decode(r, &in); err != nil || in.URL == "" {
			writeErr(w, http.StatusBadRequest, "send a multipart 'photo' file or {\"url\": \"…\"}")
			return
		}
		if data, ct, err = s.meta.FetchCover(r.Context(), in.URL, s.cfg.MaxCoverBytes); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.writeAuthorPhoto(r.Context(), a, data, ct); err != nil {
		s.fail(w, err, "save photo")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteAuthorPhoto(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAuthor(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get author")
		return
	}
	s.removeCoverFile(a.Photo)
	_ = s.store.SetAuthorPhoto(r.Context(), a.ID, "")
	w.WriteHeader(http.StatusNoContent)
}
