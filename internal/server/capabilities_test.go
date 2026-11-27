package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/config"
	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// addBook puts a book on a person's shelf through POST /books.
func (h *shelfHarness) addBook(tok string, body map[string]any) int64 {
	h.t.Helper()
	var c struct{ Book store.Book }
	if code := h.json(tok, "POST", "/books", body, &c); code != 201 {
		h.t.Fatalf("create %v: %d", body["title"], code)
	}
	return c.Book.ID
}

func (h *shelfHarness) search(tok, q string) []core.SearchItem {
	h.t.Helper()
	var res core.SearchResults
	if code := h.json(tok, "GET", "/pocket/search?"+q, nil, &res); code != 200 || res.Items == nil {
		h.t.Fatalf("search %s = %d %+v", q, code, res)
	}
	return res.Items
}

func titles(items []core.SearchItem) string {
	var t []string
	for _, it := range items {
		t = append(t, it.Title)
	}
	return strings.Join(t, " | ")
}

// Pocket Search finds a person's books by title, author, series and tag, the
// title first, and says where each stands.
func TestPocketSearch(t *testing.T) {
	h := newShelfHarness(t)
	h.anaShelf()
	emma := h.addBook(h.bobTok, map[string]any{"title": "Emma", "authors": "Jane Austen", "tags": "classic, book club", "status": "read", "rating": 4, "finished_on": "2026-10-01"})
	h.addBook(h.bobTok, map[string]any{"title": "Čarobnjak iz Oza", "authors": "L. Frank Baum"})
	h.addBook(h.bobTok, map[string]any{"title": "Soul Catcher", "authors": "Frank Herbert", "status": "read", "rating": 1, "finished_on": "2025-02-03"})
	h.addBook(h.bobTok, map[string]any{"title": "Children of Dune", "authors": "Frank Herbert", "series": "Dune Chronicles", "series_no": "3"})
	h.addBook(h.bobTok, map[string]any{"title": "Herbert's Garden", "authors": "Someone Else", "status": "abandoned"})
	dune := h.addBook(h.bobTok, map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "reading", "pages": 340})
	if code, _ := h.do(h.bobTok, "POST", fmt.Sprintf("/books/%d/progress", dune), map[string]any{"page": 120}); code != 200 {
		t.Fatalf("progress: %d", code)
	}

	// by title: the latest change first, with where the book stands
	got := h.search(h.bobTok, "q=dune")
	if titles(got) != "Dune | Children of Dune" {
		t.Fatalf("dune = %s", titles(got))
	}
	if it := got[0]; it.ID != strconv.FormatInt(dune, 10) || it.Kind != "book" || it.URL != fmt.Sprintf("/books/%d", dune) ||
		it.Snippet != "Frank Herbert · reading, page 120 of 340" || it.Date == "" {
		t.Errorf("Dune = %+v", it)
	}
	if got[1].Snippet != "Frank Herbert · want to read" {
		t.Errorf("Children of Dune = %+v", got[1])
	}
	// a title that has the words comes before an author who has them
	if got := h.search(h.bobTok, "q=Herbert"); len(got) != 4 || got[0].Title != "Herbert's Garden" || got[0].Snippet != "Someone Else · abandoned" {
		t.Errorf("herbert = %s", titles(got))
	}
	// by author: a finished book carries the day it was finished and its stars
	if got := h.search(h.bobTok, "q=austen"); len(got) != 1 || got[0].ID != strconv.FormatInt(emma, 10) || got[0].Snippet != "Jane Austen · read, 4 stars" || got[0].Date != "2026-10-01" {
		t.Errorf("austen = %+v", got)
	}
	if got := h.search(h.bobTok, "q=soul+catcher"); len(got) != 1 || got[0].Snippet != "Frank Herbert · read, 1 star" {
		t.Errorf("soul catcher = %+v", got)
	}
	// by series and by tag: the line says where the words were found
	if got := h.search(h.bobTok, "q=chronicles"); len(got) != 1 || got[0].Snippet != "Frank Herbert · want to read · Dune Chronicles #3" {
		t.Errorf("chronicles = %+v", got)
	}
	if got := h.search(h.bobTok, "q=book+club"); len(got) != 1 || got[0].Snippet != "Jane Austen · read, 4 stars · classic, book club" {
		t.Errorf("book club = %+v", got)
	}
	// letters SQLite does not fold are found whatever the capitals
	for _, q := range []string{"q=čarobnjak", "q=ČAROBNJAK+iz"} {
		if got := h.search(h.bobTok, q); len(got) != 1 || got[0].Title != "Čarobnjak iz Oza" {
			t.Errorf("%s = %+v", q, got)
		}
	}
	if got := h.search(h.bobTok, "q=ćuprija"); len(got) != 0 {
		t.Errorf("ćuprija = %+v", got)
	}
	// the limit, and nothing asked
	if got := h.search(h.bobTok, "q=herbert&limit=2"); len(got) != 2 {
		t.Errorf("limit 2 = %s", titles(got))
	}
	if got := h.search(h.bobTok, "q=+"); len(got) != 0 {
		t.Errorf("empty = %+v", got)
	}
	// each person finds only their own books
	for _, q := range []string{"q=secret", "q=diary", "q=ana", "q=private", "q=only+mine", "q=e"} {
		for _, it := range h.search(h.bobTok, q) {
			if it.Title == "Secret Diary" {
				t.Errorf("Bob found Ana's book with %s", q)
			}
		}
	}
	if got := h.search(h.anaTok, "q=private"); len(got) != 1 || got[0].Title != "Secret Diary" || got[0].Snippet != "Ana Author · read, 4.5 stars · private" || got[0].Date != "2026-03-01" {
		t.Errorf("Ana's own = %+v", got)
	}
	if got := h.search(h.anaTok, "q=dune"); len(got) != 0 {
		t.Errorf("Ana found Bob's books: %s", titles(got))
	}
	// GET /books?q= is as it was: the series' name is not looked in there
	var books []store.Book
	if code := h.json(h.bobTok, "GET", "/books?q=chronicles", nil, &books); code != 200 || len(books) != 0 {
		t.Errorf("/books?q=chronicles = %d %d books", code, len(books))
	}
}

// dayOf reads a person's day in UTC, so the test does not depend on the
// machine's time zone.
func (h *shelfHarness) dayOf(tok, date, tz string) core.Day {
	h.t.Helper()
	var d core.Day
	if code := h.json(tok, "GET", "/pocket/day/"+date+"?tz="+tz, nil, &d); code != 200 || d.Date != date || d.Items == nil {
		h.t.Fatalf("day %s = %d %+v", date, code, d)
	}
	return d
}

func dayItem(d core.Day, title string) core.DayItem {
	for _, it := range d.Items {
		if it.Title == title {
			return it
		}
	}
	return core.DayItem{}
}

// Pocket Day gets one line for each book read or listened to that day, from
// every way reading comes in, and the books started and finished.
func TestPocketDay(t *testing.T) {
	h := newShelfHarness(t)
	ctx := context.Background()
	at := func(day, clock string) int64 {
		ts, err := time.Parse("2006-01-02 15:04", day+" "+clock)
		if err != nil {
			t.Fatal(err)
		}
		return ts.Unix()
	}
	stamp := func(sec int64) string { return time.Unix(sec, 0).UTC().Format(time.RFC3339) }
	const day, next = "2026-10-05", "2026-10-06"

	ebook := h.addBook(h.anaTok, map[string]any{"title": "Library Book", "authors": "Lib Author", "status": "reading", "pages": 300, "started_on": "2026-09-20"})
	audio := h.addBook(h.anaTok, map[string]any{"title": "Audio Book", "authors": "Aud Author", "status": "reading", "started_on": "2026-09-20"})
	paper := h.addBook(h.anaTok, map[string]any{"title": "Paper Book", "authors": "Pap Author", "status": "reading", "pages": 400, "started_on": day})
	h.addBook(h.anaTok, map[string]any{"title": "Old Book", "authors": "Old Author", "status": "read", "started_on": "2026-09-01", "finished_on": day})
	h.addBook(h.anaTok, map[string]any{"title": "Dull Book", "authors": "Dull Author", "status": "abandoned", "finished_on": "2026-09-02"})

	// Pocket Library: 20% → 25% of 300 pages in ten minutes; the next day 25% → 39%
	for _, e := range []struct {
		pct float64
		ts  int64
	}{{0.20, at(day, "12:00")}, {0.25, at(day, "12:10")}, {0.25, at(next, "09:00")}, {0.39, at(next, "09:30")}} {
		if err := h.st.AddProgressEvent(ctx, h.ana.ID, ebook, "library", "Kobo", e.pct, e.ts); err != nil {
			t.Fatal(err)
		}
	}
	// by hand, late in the evening: 0 → 45
	if err := h.st.AddPageLog(ctx, h.ana.ID, paper, 0, 45, 400, at(day, "21:00")); err != nil {
		t.Fatal(err)
	}
	// Pocket Audiobooks: forty minutes in the morning
	if err := h.st.SetAudiobookLink(ctx, h.ana.ID, "li-a", audio); err != nil {
		t.Fatal(err)
	}
	if err := h.st.SaveListeningSessions(ctx, h.ana.ID, day, []store.ListeningSession{
		{SessionID: "1", ItemID: "li-a", Day: day, StartedAt: stamp(at(day, "08:00")), UpdatedAt: stamp(at(day, "08:40")), Seconds: 2400, StartPos: 600, EndPos: 3000, Duration: 36000, Device: "iPhone"},
	}, true); err != nil {
		t.Fatal(err)
	}

	d := h.dayOf(h.anaTok, day, "UTC")
	if d.Summary != "Read and listened to 3 books for 50 minutes, finished Old Book and started Paper Book" || len(d.Items) != 4 {
		t.Fatalf("day = %q %+v", d.Summary, d.Items)
	}
	if it := dayItem(d, "Library Book"); it.ID != strconv.FormatInt(ebook, 10) || it.Kind != "read" || it.Sub != "Lib Author" || it.Minutes != 10 || it.Qty != 15 ||
		it.Time != "12:00" || it.End != "12:10" || it.Done || it.URL != fmt.Sprintf("/books/%d", ebook) {
		t.Errorf("e-reader = %+v", it)
	}
	if it := dayItem(d, "Audio Book"); it.Kind != "listened" || it.Minutes != 40 || it.Qty != 0 || it.Time != "08:00" || it.End != "08:40" {
		t.Errorf("listening = %+v", it)
	}
	// pages typed in have no time of day; the book was started that day
	if it := dayItem(d, "Paper Book"); it.Kind != "started" || it.Qty != 45 || it.Minutes != 0 || it.Time != "" || it.Done {
		t.Errorf("by hand = %+v", it)
	}
	if it := dayItem(d, "Old Book"); it.Kind != "finished" || !it.Done || it.Sub != "Old Author" || it.Qty != 0 {
		t.Errorf("finished = %+v", it)
	}
	if len(d.Metrics) != 2 || d.Metrics[0] != (core.Metric{Label: "Pages", Value: "60"}) || d.Metrics[1] != (core.Metric{Label: "Time", Value: "50 minutes"}) {
		t.Errorf("metrics = %+v", d.Metrics)
	}

	// one book, read and finished: one line, in one sentence
	if code, body := h.do(h.anaTok, "POST", fmt.Sprintf("/books/%d/status", ebook), map[string]any{"status": "read", "date": next}); code != 200 {
		t.Fatalf("finish: %d %s", code, body)
	}
	d = h.dayOf(h.anaTok, next, "UTC")
	if d.Summary != "Read 42 pages of Library Book and finished it" || len(d.Items) != 1 {
		t.Fatalf("next day = %q %+v", d.Summary, d.Items)
	}
	if it := d.Items[0]; it.Kind != "finished" || !it.Done || it.Qty != 42 || it.Minutes != 30 || it.Time != "09:00" {
		t.Errorf("read and finished = %+v", it)
	}

	// the caller's time zone decides which day the reading falls on: 21:00 UTC
	// is the next morning in Tokyo. Started and finished stay the shelf's dates.
	d = h.dayOf(h.anaTok, day, "Asia/Tokyo")
	if it := dayItem(d, "Paper Book"); it.Kind != "started" || it.Qty != 0 {
		t.Errorf("Tokyo, the day = %+v", it)
	}
	if it := dayItem(d, "Library Book"); it.Time != "21:00" || it.Qty != 15 {
		t.Errorf("Tokyo, the e-reader = %+v", it)
	}
	if it := dayItem(h.dayOf(h.anaTok, next, "Asia/Tokyo"), "Paper Book"); it.Kind != "read" || it.Qty != 45 {
		t.Errorf("Tokyo, the next day = %+v", it)
	}

	// a day with one book and no pages says how long; a book given up shows too
	d = h.dayOf(h.anaTok, "2026-09-02", "UTC")
	if d.Summary != "Abandoned Dull Book" || len(d.Items) != 1 || d.Items[0].Kind != "abandoned" || d.Items[0].Done {
		t.Errorf("given up = %q %+v", d.Summary, d.Items)
	}
	if err := h.st.SaveListeningSessions(ctx, h.ana.ID, "2026-10-08", []store.ListeningSession{
		{SessionID: "2", ItemID: "li-a", Day: "2026-10-08", StartedAt: stamp(at("2026-10-08", "07:00")), UpdatedAt: stamp(at("2026-10-08", "08:20")), Seconds: 4800, StartPos: 3000, EndPos: 7800, Duration: 36000},
	}, false); err != nil {
		t.Fatal(err)
	}
	if d = h.dayOf(h.anaTok, "2026-10-08", "UTC"); d.Summary != "Listened to Audio Book for 1 h 20 min" {
		t.Errorf("listening only = %q", d.Summary)
	}

	// nothing that day, and nothing of Ana's on Bob's day
	if d = h.dayOf(h.anaTok, "2026-01-01", "UTC"); d.Summary != "" || len(d.Items) != 0 || len(d.Metrics) != 0 {
		t.Errorf("empty day = %+v", d)
	}
	if d = h.dayOf(h.bobTok, day, "UTC"); d.Summary != "" || len(d.Items) != 0 {
		t.Errorf("Bob's day = %+v", d)
	}
	if code, _ := h.do(h.anaTok, "GET", "/pocket/day/yesterday", nil); code != 400 {
		t.Errorf("bad date = %d, want 400", code)
	}
}

// Pocket Hub's widget is the Reading now page in a few lines.
func TestPocketWidget(t *testing.T) {
	h := newShelfHarness(t)
	h.anaShelf()
	widget := func(tok string) core.WidgetData {
		t.Helper()
		var w core.WidgetData
		if code := h.json(tok, "GET", "/pocket/widget", nil, &w); code != 200 || w.Items == nil {
			t.Fatalf("widget = %d %+v", code, w)
		}
		return w
	}
	// an empty shelf has nothing to show
	if w := widget(h.bobTok); w.Line != "" || len(w.Items) != 0 || len(w.Metrics) != 0 {
		t.Errorf("empty shelf = %+v", w)
	}
	// books, but none being read: the numbers, no line
	h.addBook(h.bobTok, map[string]any{"title": "Later", "authors": "Some Author"})
	h.addBook(h.bobTok, map[string]any{"title": "Done", "authors": "Some Author", "status": "read", "finished_on": time.Now().Format("2006-01-02")})
	w := widget(h.bobTok)
	if w.Line != "" || len(w.Items) != 0 || w.URL != "/now" || len(w.Metrics) != 2 ||
		w.Metrics[0] != (core.Metric{Label: "Read this year", Value: "1"}) || w.Metrics[1] != (core.Metric{Label: "Want to read", Value: "1"}) {
		t.Errorf("nothing being read = %+v", w)
	}

	// one book with a page: the line and the row say how far
	dune := h.addBook(h.bobTok, map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "reading", "pages": 340})
	if code, _ := h.do(h.bobTok, "POST", fmt.Sprintf("/books/%d/progress", dune), map[string]any{"page": 120}); code != 200 {
		t.Fatalf("progress: %d", code)
	}
	w = widget(h.bobTok)
	if w.Line != "Reading Dune, 35%" || len(w.Items) != 1 {
		t.Fatalf("one book = %+v", w)
	}
	if it := w.Items[0]; it.ID != strconv.FormatInt(dune, 10) || it.Title != "Dune" || it.Detail != "page 120 of 340 · 35%" || it.URL != fmt.Sprintf("/books/%d", dune) || it.Done {
		t.Errorf("row = %+v", it)
	}

	// more books: the one touched last leads, a book without a page shows its
	// author, an e-reader's position its percent; the goal sits next to the count
	emma := h.addBook(h.bobTok, map[string]any{"title": "Emma", "authors": "Jane Austen", "status": "reading"})
	kobo := h.addBook(h.bobTok, map[string]any{"title": "On the Kobo", "authors": "E. Reader", "status": "reading"})
	if err := h.st.AddProgressEvent(t.Context(), h.bob.ID, kobo, "library", "Kobo", 0.5, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	st, _ := h.st.GetSettings(t.Context(), h.bob.ID)
	st.YearlyGoal = 12
	if err := h.st.SaveSettings(t.Context(), h.bob.ID, st); err != nil {
		t.Fatal(err)
	}
	w = widget(h.bobTok)
	if w.Line != "Reading On the Kobo, 50%, and 2 more" || len(w.Items) != 3 || w.Metrics[0].Value != "1 of 12" {
		t.Fatalf("three books = %+v", w)
	}
	if w.Items[0].Detail != "50%" || w.Items[1].ID != strconv.FormatInt(emma, 10) || w.Items[1].Detail != "Jane Austen" || w.Items[2].Title != "Dune" {
		t.Errorf("rows = %+v", w.Items)
	}

	// Ana's widget has only her own shelf
	if w := widget(h.anaTok); w.Line != "" || len(w.Items) != 0 || w.Metrics[0].Value != "0" && w.Metrics[0].Value != "1" || w.Metrics[1].Value != "0" {
		t.Errorf("Ana = %+v", w)
	}
}

func (h *shelfHarness) capture(tok string, body map[string]any) (int, core.CaptureResult, string) {
	h.t.Helper()
	var res struct {
		core.CaptureResult
		Error string `json:"error"`
	}
	code := h.json(tok, "POST", "/pocket/capture", body, &res)
	return code, res.CaptureResult, res.Error
}

// A quick line becomes a book on Want to read, once.
func TestPocketCapture(t *testing.T) {
	h := newShelfHarness(t)
	ctx := context.Background()
	h.anaShelf()
	first := map[string]any{"request_id": "hub-0000000000000001", "kind": "book", "text": "  Dune \nMarko says it is the one to start with"}
	code, res, _ := h.capture(h.bobTok, first)
	if code != 201 || res.ID == "" || res.Title != "Dune" || res.URL != "/books/"+res.ID {
		t.Fatalf("capture = %d %+v", code, res)
	}
	id, _ := strconv.ParseInt(res.ID, 10, 64)
	b, err := h.st.GetBook(ctx, h.bob.ID, id)
	if err != nil || b.Title != "Dune" || b.Status != store.StatusWant || b.PlanNote != "Marko says it is the one to start with" || b.Source != "manual" ||
		b.Authors != "" || b.QueuePos == 0 || b.StartedOn != "" {
		t.Fatalf("the book = %v %+v", err, b)
	}
	// it is on the Planner's list like any book added by hand
	var want []store.Book
	if code := h.json(h.bobTok, "GET", "/books?status=want&sort=queue", nil, &want); code != 200 || len(want) != 1 || want[0].ID != id {
		t.Errorf("want to read = %d %+v", code, want)
	}

	// the same request again answers the same book and adds nothing
	if code, again, _ := h.capture(h.bobTok, first); code != 201 || again != res || h.count(h.bobTok) != 1 {
		t.Errorf("retry = %d %+v, %d books", code, again, h.count(h.bobTok))
	}
	// the same request id with other words is refused
	if code, _, msg := h.capture(h.bobTok, map[string]any{"request_id": "hub-0000000000000001", "text": "Emma"}); code != 400 || msg != "this was already kept with other words" {
		t.Errorf("other words = %d %q", code, msg)
	}
	// a title that is on the shelf is not added twice, whatever the capitals
	if code, _, msg := h.capture(h.bobTok, map[string]any{"request_id": "hub-0000000000000002", "title": "DUNE"}); code != 400 || msg != "“DUNE” is already on your shelf" {
		t.Errorf("twice = %d %q", code, msg)
	}
	h.addBook(h.bobTok, map[string]any{"title": "Čarobnjak iz Oza", "authors": "L. Frank Baum", "status": "read"})
	if code, _, msg := h.capture(h.bobTok, map[string]any{"request_id": "hub-0000000000000003", "text": "čarobnjak iz oza"}); code != 400 || !strings.Contains(msg, "already on your shelf") {
		t.Errorf("twice, other letters = %d %q", code, msg)
	}
	// Ana has the same title on her own shelf: that is no reason to refuse Bob
	if code, res, _ := h.capture(h.bobTok, map[string]any{"request_id": "hub-0000000000000004", "title": "Secret Diary", "text": "the one Ana keeps talking about"}); code != 201 || res.Title != "Secret Diary" {
		t.Errorf("same title as Ana's = %d %+v", code, res)
	}
	if h.count(h.bobTok) != 3 || h.count(h.anaTok) != 1 {
		t.Errorf("books: Bob %d, Ana %d", h.count(h.bobTok), h.count(h.anaTok))
	}

	// through Pocket Hub's link the book remembers where it came from
	code, res, _ = h.capture("pcx_cap_bob_capture", map[string]any{"request_id": "hub-0000000000000005", "title": "Emma"})
	if code != 201 || res.Title != "Emma" {
		t.Fatalf("linked capture = %d %+v", code, res)
	}
	id, _ = strconv.ParseInt(res.ID, 10, 64)
	if b, err := h.st.GetBook(ctx, h.bob.ID, id); err != nil || b.Source != "hub" || b.Status != store.StatusWant {
		t.Errorf("linked book = %v %+v", err, b)
	}
	if code, again, _ := h.capture("pcx_cap_bob_capture", map[string]any{"request_id": "hub-0000000000000005", "title": "Emma"}); code != 201 || again != res {
		t.Errorf("linked retry = %d %+v", code, again)
	}

	// what the route does not take
	for name, body := range map[string]map[string]any{
		"another kind":   {"request_id": "hub-0000000000000006", "kind": "quote", "text": "x"},
		"no request id":  {"text": "x"},
		"nothing to add": {"request_id": "hub-0000000000000007", "text": "   "},
	} {
		if code, _, msg := h.capture(h.bobTok, body); code != 400 || msg == "" {
			t.Errorf("%s = %d %q", name, code, msg)
		}
	}
	if h.count(h.bobTok) != 4 {
		t.Errorf("Bob has %d books", h.count(h.bobTok))
	}

	// the same line sent twice at the same moment is kept once
	var wg sync.WaitGroup
	ids := make([]string, 4)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, res, _ := h.capture(h.bobTok, map[string]any{"request_id": "hub-0000000000000008", "title": "Persuasion"})
			ids[i] = res.ID
		}()
	}
	wg.Wait()
	for _, got := range ids {
		if got == "" || got != ids[0] {
			t.Errorf("at once = %v", ids)
			break
		}
	}
	if h.count(h.bobTok) != 5 {
		t.Errorf("Bob has %d books", h.count(h.bobTok))
	}
}

// "When I add a book to Want to read: ask Pocket Requests" holds for a kept
// line too, after the answer.
func TestPocketCaptureAsksRequests(t *testing.T) {
	h := newShelfHarness(t)
	mu, posted := fakeRequests(t, h)
	asked := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(*posted)
	}
	// no choice made: nothing is asked
	if code, _, _ := h.capture(h.anaTok, map[string]any{"request_id": "hub-0000000000000011", "title": "Dune"}); code != 201 {
		t.Fatalf("capture = %d", code)
	}
	if code := h.json(h.anaTok, "PUT", "/requests/settings", map[string]any{"want_get": "ebook"}, nil); code != 200 {
		t.Fatalf("settings %d", code)
	}
	if code, _, _ := h.capture(h.anaTok, map[string]any{"request_id": "hub-0000000000000012", "title": "Emma"}); code != 201 {
		t.Fatalf("capture = %d", code)
	}
	for i := 0; i < 200 && asked() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*posted) != 1 || (*posted)[0]["title"] != "Emma" || (*posted)[0]["kind"] != "ebook" {
		t.Fatalf("asked = %v", *posted)
	}
}

// A link token with one of the pocket:* scopes reaches its one route (and
// /auth/me), as the person it acts for; the app's own scopes do not open them.
func TestPocketLinkScopes(t *testing.T) {
	h := newShelfHarness(t)
	h.anaShelf()
	dune := h.addBook(h.bobTok, map[string]any{"title": "Dune", "authors": "Frank Herbert", "status": "reading", "pages": 340, "started_on": "2026-10-05"})
	type call struct {
		m, p string
		body map[string]any
	}
	n := 0
	routes := map[string]func() call{
		"search": func() call { return call{"GET", "/pocket/search?q=e", nil} },
		"day":    func() call { return call{"GET", "/pocket/day/2026-10-05", nil} },
		"widget": func() call { return call{"GET", "/pocket/widget", nil} },
		"capture": func() call {
			n++
			return call{"POST", "/pocket/capture", map[string]any{"request_id": fmt.Sprintf("hub-00000000000001%02d", n), "title": fmt.Sprintf("Book %d", n)}}
		},
	}
	for scope := range routes {
		tok := "pcx_cap_bob_" + scope
		for name, mk := range routes {
			c := mk()
			code, body := h.do(tok, c.m, c.p, c.body)
			switch {
			case name == scope && (code/100 != 2 || strings.Contains(string(body), "Secret Diary")):
				t.Errorf("pocket:%s %s %s = %d %s", scope, c.m, c.p, code, body)
			case name == scope && scope != "capture" && !strings.Contains(string(body), "Dune"):
				t.Errorf("pocket:%s does not show Bob's book: %s", scope, body)
			case name != scope && (code != 403 || !strings.Contains(string(body), "link does not cover")):
				t.Errorf("pocket:%s on %s %s = %d %s, want 403", scope, c.m, c.p, code, body)
			}
		}
		var u store.User
		if code := h.json(tok, "GET", "/auth/me", nil, &u); code != 200 || u.Email != "bob@example.com" {
			t.Errorf("%s auth/me = %d %+v", tok, code, u)
		}
		// nothing else in the app
		for _, c := range []call{
			{"GET", "/books", nil}, {"GET", fmt.Sprintf("/books/%d", dune), nil}, {"POST", "/books", map[string]any{"title": "x"}}, {"GET", "/reading/sessions", nil},
			{"GET", "/dashboard", nil}, {"GET", "/search?q=x", nil}, {"GET", "/export.json", nil}, {"GET", "/settings", nil}, {"DELETE", fmt.Sprintf("/books/%d", dune), nil},
		} {
			if code, _ := h.do(tok, c.m, c.p, c.body); code != 403 {
				t.Errorf("%s %s %s = %d, want 403", tok, c.m, c.p, code)
			}
		}
		// and its own route in no other way (the router has no such route to refuse)
		for _, c := range []call{
			{"POST", "/pocket/search?q=e", map[string]any{}}, {"POST", "/pocket/widget", map[string]any{}}, {"GET", "/pocket/capture", nil},
			{"DELETE", "/pocket/day/2026-10-05", nil}, {"GET", "/pocket/day/2026-10-05/more", nil},
		} {
			if code, _ := h.do(tok, c.m, c.p, c.body); code != 403 && code != 404 && code != 405 {
				t.Errorf("%s %s %s = %d", tok, c.m, c.p, code)
			}
		}
	}
	// the links the app had before do not open the new routes
	for _, tok := range []string{"pcx_bob_books", "pcx_bob_libwrite", "pcx_bob_other", "pcx_move_bob_in", "pcx_move_bob_out"} {
		for _, mk := range routes {
			c := mk()
			if code, _ := h.do(tok, c.m, c.p, c.body); code != 403 {
				t.Errorf("%s %s %s = %d, want 403", tok, c.m, c.p, code)
			}
		}
	}
	// a token Core does not vouch for
	if code, _ := h.do("pcx_revoked", "GET", "/pocket/widget", nil); code != 401 {
		t.Errorf("unknown link token = %d, want 401", code)
	}
	// only the capture link added anything, each request once
	var books []store.Book
	h.json(h.bobTok, "GET", "/books", nil, &books)
	for _, b := range books {
		if b.Title != "Dune" && b.Source != "hub" {
			t.Errorf("added outside the capture link: %+v", b)
		}
	}
	if len(books) != 2 || h.count(h.anaTok) != 1 {
		t.Errorf("Bob has %d books, Ana %d", len(books), h.count(h.anaTok))
	}
}

// The manifest tells Pocket Core the shelf Hub puts the app on and what
// Search, Day and Hub can ask it.
func TestManifestCapabilities(t *testing.T) {
	h := newShelfHarness(t)
	h.cfg.CoreRegistrationKey = "pcr_test"
	if err := h.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := h.manifest()
	if m["slug"] != "bookshelf" || m["category"] != "Media" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	capture, _ := m["capture"].(map[string]any)
	if search == nil || day == nil || widget == nil || capture == nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, capture)
	}
	if bangs := fmt.Sprint(search["bangs"]); bangs != "[books book]" || fmt.Sprint(search["kinds"]) != "[book]" {
		t.Errorf("search: %v", search)
	}
	if day["label"] != "Reading" || day["one"] != "book" || day["many"] != "books" || day["unit"] != "qty" || day["what"] == "" {
		t.Errorf("day: %v", day)
	}
	if widget["title"] != "Reading now" || widget["what"] == "" {
		t.Errorf("widget: %v", widget)
	}
	if k := capture["kinds"].([]any)[0].(map[string]any); k["kind"] != "book" || k["label"] != "Book" || fmt.Sprint(k["fields"]) != "[title]" {
		t.Errorf("capture: %v", capture)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"books:read", "books:write", core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture, core.ScopeMoveExport, core.ScopeMoveImport} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
}

// Without Pocket Core the routes answer the person signed in to the app, and
// a kept line is still kept once.
func TestPocketRoutesWithoutCore(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db", CoverDir: dir + "/covers", MaxCoverBytes: 1 << 20}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ana := &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	if err := st.CreateUser(t.Context(), ana); err != nil {
		t.Fatal(err)
	}
	tok, _, _ := st.CreateAPIToken(t.Context(), ana.ID, "ana")
	s := New(cfg, st, log, nil)
	if err := s.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	h := &shelfHarness{t: t, ts: ts, st: st, s: s, cfg: cfg, ana: ana, anaTok: tok}

	body := map[string]any{"request_id": "web-0000000000000001", "text": "Dune"}
	code, res, _ := h.capture(tok, body)
	if code != 201 || res.Title != "Dune" {
		t.Fatalf("capture = %d %+v", code, res)
	}
	if code, again, _ := h.capture(tok, body); code != 201 || again != res || h.count(tok) != 1 {
		t.Errorf("retry = %d %+v, %d books", code, again, h.count(tok))
	}
	if got := h.search(tok, "q=dune"); len(got) != 1 || got[0].Snippet != "want to read" {
		t.Errorf("search = %+v", got)
	}
	var w core.WidgetData
	if code := h.json(tok, "GET", "/pocket/widget", nil, &w); code != 200 || len(w.Metrics) != 2 || w.Metrics[1].Value != "1" {
		t.Errorf("widget = %d %+v", code, w)
	}
	if d := h.dayOf(tok, "2026-10-05", "UTC"); len(d.Items) != 0 {
		t.Errorf("day = %+v", d)
	}
	// a link token means nothing here
	if code, _ := h.do("pcx_cap_bob_widget", "GET", "/pocket/widget", nil); code != 401 {
		t.Errorf("link token without Core = %d, want 401", code)
	}
	if code, _ := h.do("", "GET", "/pocket/widget", nil); code != 401 {
		t.Errorf("nobody = %d, want 401", code)
	}
}
