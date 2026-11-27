package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// The four routes behind `capabilities` (links.go): /api/v1/pocket/search,
// /day/{date}, /widget and /capture, for Pocket Search, Pocket Day and Pocket
// Hub. Each runs as the request's person and sees only their shelf; the SDK's
// handlers check and cap what goes in and out. The first three read what
// /books, /reading/sessions and /dashboard read; only the capture adds
// something, one book on Want to read.

// bookURL opens a book in the app.
func bookURL(id int64) string { return "/books/" + strconv.FormatInt(id, 10) }

// dots joins the parts of a line that are there: "Frank Herbert · read, 4 stars".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// upperFirst starts a line with a capital letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// count is a number with its word: "1 page", "42 pages".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// minutesWords is a length of time as the pages show it: "50 minutes", "1 h 20 min".
func minutesWords(m int) string {
	switch h, r := m/60, m%60; {
	case h == 0:
		return count(m, "minute", "minutes")
	case r == 0:
		return strconv.Itoa(h) + " h"
	default:
		return fmt.Sprintf("%d h %d min", h, r)
	}
}

// pageWords is where the person is in a book with a page count: "page 120 of 340".
func pageWords(b *store.Book) string {
	if b.Pages > 0 && b.CurrentPage > 0 {
		return fmt.Sprintf("page %d of %d", min(b.CurrentPage, b.Pages), b.Pages)
	}
	return ""
}

// percentWords is how far through a book the person is, rounded as the pages
// round it: "35%". Nothing to go by, or nothing read yet, says nothing.
func percentWords(b *store.Book) string {
	if p, ok := b.Progress(); ok && p > 0 {
		if pct := int(math.Round(p * 100)); pct > 0 {
			return strconv.Itoa(pct) + "%"
		}
	}
	return ""
}

// statusWords says where a book stands: "want to read", "reading, page 120
// of 340", "read, 4 stars", "abandoned".
func statusWords(b *store.Book) string {
	switch b.Status {
	case store.StatusReading:
		if at := firstNonEmpty(pageWords(b), percentWords(b)); at != "" {
			return "reading, " + at
		}
		return "reading"
	case store.StatusRead:
		if b.Rating > 0 {
			stars := "stars"
			if b.Rating == 1 {
				stars = "star"
			}
			return "read, " + strconv.FormatFloat(b.Rating, 'f', -1, 64) + " " + stars
		}
		return "read"
	case store.StatusAbandoned:
		return "abandoned"
	}
	return "want to read"
}

// ---------- search ----------

// pocketSearch answers Pocket Search with the books GET /books?q= finds (by
// title, author, tag, thoughts and ISBN) and the ones in a series of that
// name: a title that has the words first, then an author or a series, then
// the rest, each group latest change first.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	f := store.BookFilter{UserID: me(r), Query: q, InSeries: true}
	// SQLite's LOWER leaves "Č" as it is, so words with such letters are
	// looked for here, on the whole shelf (as findDuplicate does)
	own := !isASCII(q)
	if own {
		f.Query = ""
	}
	books, err := s.store.ListBooks(r.Context(), f)
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	has := func(v ...string) bool { return strings.Contains(strings.ToLower(strings.Join(v, "\n")), lq) }
	out := make([]core.SearchItem, 0, len(books))
	for i := range books {
		b := &books[i]
		it := core.SearchItem{ID: strconv.FormatInt(b.ID, 10), Title: b.Title, Kind: "book", Date: b.UpdatedAt, URL: bookURL(b.ID), Score: 1}
		if b.Status == store.StatusRead && b.FinishedOn != "" {
			it.Date = b.FinishedOn
		}
		// where the words were found, when the line does not show it
		where := ""
		switch {
		case has(b.Title, b.Subtitle):
			it.Score = 3
		case has(b.Authors):
			it.Score = 2
		case has(b.Series):
			it.Score, where = 2, b.Series
			if b.SeriesNo != "" {
				where += " #" + b.SeriesNo
			}
		case has(b.Tags):
			where = b.Tags
		case own && !has(b.Review):
			continue
		}
		it.Snippet = dots(b.Authors, statusWords(b), where)
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// dayBook is one book on a day: what was read of it and what happened to it.
type dayBook struct {
	id             int64
	title, authors string
	minutes, pages int
	start, end     int64 // of the reading and listening; 0 when only pages were logged
	read, listened bool
	mark           string // finished | abandoned | started
}

// sat reports whether the person sat down with the book that day.
func (d *dayBook) sat() bool { return d.read || d.listened }

// pocketDay answers Pocket Day from the reading log (what GET
// /reading/sessions lists): one line for each book read or listened to that
// day, with its minutes and pages, and the books started, finished or given
// up that day. A book shows once. The log's days and times are worked out in
// the caller's time zone when it gives one; started and finished are the plain
// dates the shelf keeps.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx, uid := r.Context(), me(r)
	loc := time.Local
	if tz != nil {
		loc = tz
	}
	all, _, err := s.readingSessionsIn(ctx, uid, loc)
	if err != nil {
		return nil, err
	}
	marks, err := s.store.ReadsOn(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	var books []*dayBook
	byID := map[int64]*dayBook{}
	book := func(id int64, title, authors string) *dayBook {
		d := byID[id]
		if d == nil {
			d = &dayBook{id: id, title: title, authors: authors}
			byID[id] = d
			books = append(books, d)
		}
		return d
	}
	for _, x := range all {
		if x.Day != date {
			continue
		}
		d := book(x.BookID, x.Title, x.Authors)
		d.minutes += x.Minutes
		d.pages += x.Pages
		d.listened = d.listened || x.Kind == "listening"
		d.read = d.read || x.Kind != "listening"
		if x.Kind != "logged" { // a page typed in says when it was typed, not when it was read
			if d.start == 0 || x.start < d.start {
				d.start = x.start
			}
			d.end = max(d.end, x.end)
		}
	}
	for _, m := range marks {
		d := book(m.BookID, m.Title, m.Authors)
		switch {
		case m.Finished:
			d.mark = "finished"
		case m.Abandoned && d.mark != "finished":
			d.mark = "abandoned"
		case m.Started && d.mark == "":
			d.mark = "started"
		}
	}
	if len(books) == 0 {
		return nil, nil
	}

	day := &core.Day{Date: date}
	var sat []*dayBook
	marked := map[string][]*dayBook{}
	minutes, pages, read, listened := 0, 0, false, false
	for _, d := range books {
		it := core.DayItem{ID: strconv.FormatInt(d.id, 10), Title: d.title, Sub: d.authors, Kind: "read", Minutes: float64(d.minutes), Qty: float64(d.pages), URL: bookURL(d.id)}
		if d.listened && !d.read {
			it.Kind = "listened"
		}
		if d.mark != "" {
			it.Kind, it.Done = d.mark, d.mark == "finished"
			marked[d.mark] = append(marked[d.mark], d)
		}
		if d.start > 0 {
			it.Time, it.End = time.Unix(d.start, 0).In(loc).Format("15:04"), time.Unix(d.end, 0).In(loc).Format("15:04")
		}
		day.Items = append(day.Items, it)
		if d.sat() {
			sat = append(sat, d)
			minutes, pages = minutes+d.minutes, pages+d.pages
			read, listened = read || d.read, listened || d.listened
		}
	}

	var parts []string
	if n := len(sat); n > 0 {
		verb := "read"
		switch {
		case listened && !read:
			verb = "listened to"
		case listened:
			verb = "read and listened to"
		}
		switch {
		case n == 1 && pages > 0:
			parts = append(parts, fmt.Sprintf("read %s of %s", count(pages, "page", "pages"), sat[0].title))
		case n == 1:
			parts = append(parts, fmt.Sprintf("%s %s for %s", verb, sat[0].title, minutesWords(minutes)))
		case minutes > 0:
			parts = append(parts, fmt.Sprintf("%s %d books for %s", verb, n, minutesWords(minutes)))
		default:
			parts = append(parts, fmt.Sprintf("read %s in %d books", count(pages, "page", "pages"), n))
		}
	}
	for _, mark := range []string{"finished", "started", "abandoned"} {
		switch of := marked[mark]; {
		case len(of) == 1 && len(sat) == 1 && of[0] == sat[0]:
			parts = append(parts, mark+" it")
		case len(of) == 1:
			parts = append(parts, mark+" "+of[0].title)
		case len(of) > 1:
			parts = append(parts, fmt.Sprintf("%s %d books", mark, len(of)))
		}
	}
	// "Read 42 pages of Dune and finished it", "Read 2 books for 50 minutes, finished Dune and started Emma"
	if n := len(parts); n > 1 {
		parts = []string{strings.Join(parts[:n-1], ", ") + " and " + parts[n-1]}
	}
	day.Summary = upperFirst(parts[0])
	if pages > 0 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Pages", Value: strconv.Itoa(pages)})
	}
	if minutes > 0 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Time", Value: minutesWords(minutes)})
	}
	return day, nil
}

// ---------- widget ----------

// pocketWidget answers Pocket Hub with the Reading now page in a few lines:
// the books being read, the one touched last on top, each with how far the
// person is, and the dashboard's count of books read this year (of the
// yearly goal, when there is one). Someone with an empty shelf gets nothing
// to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx, uid := r.Context(), me(r)
	st, _ := s.store.GetSettings(ctx, uid)
	d, err := s.store.GetDashboard(ctx, uid, st.YearlyGoal)
	if err != nil {
		return nil, err
	}
	if d.Books == 0 {
		return nil, nil
	}
	// the dashboard's list has no page numbers
	reading, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid, Status: store.StatusReading, Sort: "recent"})
	if err != nil {
		return nil, err
	}
	w := &core.WidgetData{URL: "/now"}
	for i := range reading {
		b := &reading[i]
		w.Items = append(w.Items, core.WidgetItem{ID: strconv.FormatInt(b.ID, 10), Title: b.Title, URL: bookURL(b.ID),
			Detail: firstNonEmpty(dots(pageWords(b), percentWords(b)), b.Authors)})
	}
	if n := len(reading); n > 0 {
		w.Line = "Reading " + reading[0].Title
		if pct := percentWords(&reading[0]); pct != "" {
			w.Line += ", " + pct
		}
		if n > 1 {
			w.Line += fmt.Sprintf(", and %d more", n-1)
		}
	}
	year := strconv.Itoa(d.ReadThisYear)
	if d.YearlyGoal > 0 {
		year += " of " + strconv.Itoa(d.YearlyGoal)
	}
	w.Metrics = []core.Metric{{Label: "Read this year", Value: year}, {Label: "Want to read", Value: strconv.Itoa(d.Want)}}
	return w, nil
}

// ---------- capture ----------

// captureBook splits a quick line into a book's title and why to read it:
// the title when one was given (the text is then the why), else the text's
// first line. The why is kept as long as the Planner's own field takes.
func captureBook(in core.CaptureInput) (title, why string) {
	title, why = in.Title, in.Text
	if title == "" {
		title, why, _ = strings.Cut(in.Text, "\n")
	}
	title, why = strings.Join(strings.Fields(title), " "), strings.Join(strings.Fields(why), " ")
	return clipText(title, 300), clipText(why, 200)
}

// pocketCapture keeps a quick line as a book on Want to read, the way "Add it
// by hand" adds one from a title alone: nothing is looked up, the details are
// filled in on the book's page. A title that is already on the shelf is not
// added again. The same request_id again answers the same book and adds
// nothing: the receipt is written in the transaction that adds the book.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx, uid := r.Context(), me(r)
	op, err := core.NewOperation(fmt.Sprintf("bookshelf:user:%d:%s", uid, core.CapturePath), in.RequestID, in)
	if err != nil {
		return nil, err
	}
	var res core.CaptureResult
	replay := func() (*core.CaptureResult, error) {
		found, err := op.Replay(ctx, s.store.DB, &res)
		switch {
		case errors.Is(err, core.ErrOperationConflict):
			return nil, fmt.Errorf("%w: this was already kept with other words", core.ErrCapture)
		case err != nil || !found:
			return nil, err
		}
		return &res, nil
	}
	if kept, err := replay(); kept != nil || err != nil {
		return kept, err
	}

	b := store.Book{UserID: uid, Status: store.StatusWant, Source: firstNonEmpty(linkApp(ctx), "manual")}
	b.Title, b.PlanNote = captureBook(in)
	if clean(&b) != "" {
		return nil, fmt.Errorf("%w: a book needs a title", core.ErrCapture)
	}
	if len(s.sameTitle(ctx, uid, b.Title)) > 0 {
		// the same line sent twice at once: the other one kept it just now
		if kept, err := replay(); kept != nil || err != nil {
			return kept, err
		}
		return nil, fmt.Errorf("%w: “%s” is already on your shelf", core.ErrCapture, b.Title)
	}
	err = s.store.CreateBook(ctx, &b, "", "", func(tx *sql.Tx) error {
		res = core.CaptureResult{ID: strconv.FormatInt(b.ID, 10), Title: b.Title, URL: bookURL(b.ID)}
		return op.Save(ctx, tx, res)
	})
	if err != nil {
		// the same line sent twice at once: the other one kept it
		if kept, rerr := replay(); kept != nil || rerr != nil {
			return kept, rerr
		}
		return nil, err
	}
	s.wantLater(ctx, &b)
	return &res, nil
}

// wantLater does for a kept line what adding a book to Want to read does
// when the person chose "ask Pocket Requests for it right away" (Settings →
// Requests): it asks, but after the answer, so keeping a line never waits
// for another app. Without that choice nothing is asked.
func (s *Server) wantLater(ctx context.Context, b *store.Book) {
	u := userFrom(ctx)
	if u == nil || !s.core.Configured() || len(wantGetModes[s.store.UserKey(ctx, u.ID, "want_get")]) == 0 {
		return
	}
	saved := *b
	go func() {
		if g := s.afterWant(context.WithoutCancel(ctx), u, &saved); g != nil {
			s.log.Info("kept line: asked Pocket Requests", "book", saved.ID, "asked", g["asked"])
		}
	}()
}
