package server

// More from CrossPoint Sync than positions: the highlights made on the device
// (as quotes), the pages read each day (for streaks, pace and "days left"),
// and statuses set here sent back so both sides agree. Also the "finished?"
// question when a device reaches the end of a book.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

const finishAt = 0.98 // CrossPoint Sync counts a book finished from here

// crossPointDo sends a request to the person's CrossPoint Sync account.
func crossPointDo(ctx context.Context, p *store.KosyncPrefs, method, path string, body any) (*http.Response, error) {
	var rd *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(raw)
	}
	var req *http.Request
	var err error
	if rd != nil {
		req, err = http.NewRequestWithContext(ctx, method, p.RemoteURL+path, rd)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, p.RemoteURL+path, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-User", p.RemoteUser)
	req.Header.Set("X-Auth-Key", p.RemoteKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PocketBookshelf/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized {
		res.Body.Close()
		return nil, fmt.Errorf("CrossPoint Sync did not accept the username or password")
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("CrossPoint Sync answered %s", res.Status)
	}
	return res, nil
}

// pullClippings brings the device's highlights in as quotes of the linked
// books. Safe to repeat: quotes already there are skipped.
func (s *Server) pullClippings(ctx context.Context, uid int64, p *store.KosyncPrefs) (int, error) {
	res, err := crossPointDo(ctx, p, http.MethodGet, "/api/v1/clippings", nil)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	var list struct {
		Items []struct {
			Document  string `json:"document"`
			Chapter   string `json:"chapter"`
			Text      string `json:"text"`
			Note      string `json:"note"`
			CreatedAt int64  `json:"created_at"`
		} `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 32<<20)).Decode(&list); err != nil {
		return 0, err
	}
	byDoc := map[string][]store.Quote{}
	for _, c := range list.Items {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		q := store.Quote{Text: strings.TrimSpace(c.Text), Page: clip(strings.TrimSpace(c.Chapter), 60), Note: strings.TrimSpace(c.Note)}
		if c.CreatedAt > 0 {
			q.CreatedAt = time.Unix(c.CreatedAt, 0).UTC().Format(time.RFC3339)
		}
		byDoc[c.Document] = append(byDoc[c.Document], q)
	}
	n := 0
	for doc, quotes := range byDoc {
		d, err := s.store.GetKosyncDocument(ctx, uid, doc)
		if err != nil || d.BookID == nil || d.Ignored {
			continue // not linked yet: they come in once it is
		}
		added, updated, _, err := s.store.ImportQuotes(ctx, uid, *d.BookID, quotes)
		if err != nil {
			return n, err
		}
		n += added + updated
	}
	return n, nil
}

// pullActivity keeps the pages read each day, as CrossPoint Sync logs them.
func (s *Server) pullActivity(ctx context.Context, uid int64, p *store.KosyncPrefs) error {
	_, off := time.Now().Zone()
	res, err := crossPointDo(ctx, p, http.MethodGet, "/api/v1/stats/activity?tz="+strconv.Itoa(-off/60), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var act struct {
		Days []struct {
			Day   string `json:"day"`
			Books []struct {
				Document string  `json:"document"`
				Pages    int     `json:"pages"`
				Syncs    int     `json:"syncs"`
				From     float64 `json:"from"`
				To       float64 `json:"to"`
			} `json:"books"`
		} `json:"days"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 32<<20)).Decode(&act); err != nil {
		return err
	}
	var rows []store.ActivityRow
	for _, d := range act.Days {
		if _, err := time.Parse("2006-01-02", d.Day); err != nil {
			continue
		}
		for _, b := range d.Books {
			if kosyncDocumentRE.MatchString(b.Document) {
				rows = append(rows, store.ActivityRow{Day: d.Day, Document: b.Document, Pages: max(b.Pages, 0), Syncs: max(b.Syncs, 0), From: b.From, To: b.To})
			}
		}
	}
	return s.store.ReplaceKosyncActivity(ctx, uid, rows)
}

// pushReaderStatus tells CrossPoint Sync a status set here: finished and
// did-not-finish as such; anything else hands the status back to the device's
// progress (a manual "reading" there would stop it from ever finishing).
func (s *Server) pushReaderStatus(uid, bookID int64, status string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		p, err := s.store.GetKosyncPrefs(ctx, uid)
		if err != nil || !p.Connected() {
			return
		}
		docs, err := s.store.KosyncDocumentsForBook(ctx, uid, bookID)
		if err != nil || len(docs) == 0 {
			return
		}
		var want any
		switch status {
		case store.StatusRead:
			want = "finished"
		case store.StatusAbandoned:
			want = "dnf"
		}
		for _, d := range docs {
			res, err := crossPointDo(ctx, p, http.MethodPut, "/api/v1/documents/"+url.PathEscape(d)+"/status", map[string]any{"status": want})
			if err != nil {
				s.log.Warn("crosspoint status", "user", uid, "book", bookID, "document", d, "err", err)
				continue
			}
			res.Body.Close()
			s.log.Info("crosspoint status", "user", uid, "book", bookID, "document", d, "status", want)
		}
	}()
}

// askFinished asks once, when a device reaches the end of a book still being
// read here, whether it is finished: the book page and Now show the question,
// and the phone gets it too when Daily highlights has an address.
func (s *Server) askFinished(ctx context.Context, uid int64, b *store.Book, d *store.KosyncDocument) {
	if d.Percentage < finishAt || b.Status != store.StatusReading {
		return
	}
	if first, err := s.store.MarkKosyncFinishAsked(ctx, uid, d.Document); err != nil || !first {
		return
	}
	link := strings.TrimRight(s.cfg.BaseURL, "/") + fmt.Sprintf("/books/%d", b.ID)
	title := "📖 Finished " + b.Title + "?"
	body := fmt.Sprintf("Your e-reader is at %d%% of %s. Mark it read and rate it in Bookshelf.", int(math.Round(d.Percentage*100)), b.Title)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.notify(ctx, uid, title, body, link); err != nil {
			s.log.Info("finish question not sent to the phone", "user", uid, "book", b.ID, "reason", err)
		}
	}()
}

// Pace is how a book is going on the e-reader.
type Pace struct {
	Days        []store.BookActivity `json:"days"`
	ReadingDays int                  `json:"reading_days"`  // days with pages, last 30 days
	PagesPerDay float64              `json:"pages_per_day"` // on those days
	PagesToday  int                  `json:"pages_today"`
	PagesLeft   int                  `json:"pages_left"`
	DaysLeft    int                  `json:"days_left"` // 0 when unknown
	FinishOn    string               `json:"finish_on"`
}

func (s *Server) bookPace(ctx context.Context, uid int64, b *store.Book) (*Pace, error) {
	days, err := s.store.ActivityForBook(ctx, uid, b.ID)
	if err != nil {
		return nil, err
	}
	p := &Pace{Days: days}
	today := time.Now().Format("2006-01-02")
	since := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	sum := 0
	for _, d := range days {
		if d.Day == today {
			p.PagesToday = d.Pages
		}
		if d.Day >= since && d.Pages > 0 {
			p.ReadingDays++
			sum += d.Pages
		}
	}
	if p.ReadingDays > 0 {
		p.PagesPerDay = math.Round(float64(sum)/float64(p.ReadingDays)*10) / 10
	}
	if b.Pages > 0 {
		p.PagesLeft = max(b.Pages-b.CurrentPage, 0)
	}
	if b.Status == store.StatusReading && p.PagesPerDay > 0 && p.PagesLeft > 0 {
		p.DaysLeft = int(math.Ceil(float64(p.PagesLeft) / p.PagesPerDay))
		p.FinishOn = time.Now().AddDate(0, 0, p.DaysLeft).Format("2006-01-02")
	}
	return p, nil
}

func (s *Server) handleBookPace(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	p, err := s.bookPace(r.Context(), b.UserID, b)
	if err != nil {
		s.fail(w, err, "pace")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Activity is the reading on e-readers, for Stats.
type Activity struct {
	Days           []store.ReadingDay `json:"days"` // the last 365 days with reading
	Streak         int                `json:"streak"`
	LongestStreak  int                `json:"longest_streak"`
	ReadingDaysYTD int                `json:"reading_days_year"`
	Pages7         int                `json:"pages_7"`
	Pages30        int                `json:"pages_30"`
	PagesYTD       int                `json:"pages_year"`
	Connected      bool               `json:"connected"`
}

func (s *Server) readingActivity(ctx context.Context, uid int64) (*Activity, error) {
	now := time.Now()
	days, err := s.store.ReadingDays(ctx, uid, now.AddDate(-1, 0, 0).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	a := &Activity{Days: days}
	if p, err := s.store.GetKosyncPrefs(ctx, uid); err == nil {
		a.Connected = p.Connected()
	}
	set := map[string]bool{}
	year := now.Format("2006")
	d7, d30 := now.AddDate(0, 0, -6).Format("2006-01-02"), now.AddDate(0, 0, -29).Format("2006-01-02")
	for _, d := range days {
		set[d.Day] = true
		if strings.HasPrefix(d.Day, year) {
			a.ReadingDaysYTD++
			a.PagesYTD += d.Pages
		}
		if d.Day >= d7 {
			a.Pages7 += d.Pages
		}
		if d.Day >= d30 {
			a.Pages30 += d.Pages
		}
	}
	// The streak runs up to today, or up to yesterday while today is still open.
	day := now
	if !set[day.Format("2006-01-02")] {
		day = day.AddDate(0, 0, -1)
	}
	for set[day.Format("2006-01-02")] {
		a.Streak++
		day = day.AddDate(0, 0, -1)
	}
	sorted := make([]string, 0, len(set))
	for d := range set {
		sorted = append(sorted, d)
	}
	sort.Strings(sorted)
	run := 0
	var prev time.Time
	for _, d := range sorted {
		t, _ := time.Parse("2006-01-02", d)
		if run > 0 && t.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		prev = t
		a.LongestStreak = max(a.LongestStreak, run)
	}
	return a, nil
}

func (s *Server) handleReadingActivity(w http.ResponseWriter, r *http.Request) {
	a, err := s.readingActivity(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "activity")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleReadingToday is the day's reading for a briefing: the books being
// read with progress, today's pages and when each will be finished.
func (s *Server) handleReadingToday(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), me(r)
	books, err := s.store.ListBooks(ctx, store.BookFilter{UserID: uid, Status: store.StatusReading, Sort: "recent"})
	if err != nil {
		s.fail(w, err, "list books")
		return
	}
	type line struct {
		ID         int64   `json:"id"`
		Title      string  `json:"title"`
		Authors    string  `json:"authors"`
		Percent    int     `json:"percent"`
		Page       int     `json:"page,omitempty"`
		Pages      int     `json:"pages,omitempty"`
		PagesToday int     `json:"pages_today"`
		PerDay     float64 `json:"pages_per_day,omitempty"`
		DaysLeft   int     `json:"days_left,omitempty"`
		FinishOn   string  `json:"finish_on,omitempty"`
		Finished   bool    `json:"device_says_finished,omitempty"`
		URL        string  `json:"item_url"`
	}
	out := []line{}
	total := 0
	for i := range books {
		b := &books[i]
		l := line{ID: b.ID, Title: b.Title, Authors: b.Authors, Page: b.CurrentPage, Pages: b.Pages,
			URL: strings.TrimRight(s.cfg.BaseURL, "/") + fmt.Sprintf("/books/%d", b.ID)}
		switch {
		case b.Pages > 0:
			l.Percent = min(100, b.CurrentPage*100/b.Pages)
		case b.ReaderPercent != nil:
			l.Percent = int(math.Round(*b.ReaderPercent * 100))
		}
		l.Finished = b.ReaderPercent != nil && *b.ReaderPercent >= finishAt
		if p, err := s.bookPace(ctx, uid, b); err == nil {
			l.PagesToday, l.PerDay, l.DaysLeft, l.FinishOn = p.PagesToday, p.PagesPerDay, p.DaysLeft, p.FinishOn
			total += p.PagesToday
		}
		out = append(out, l)
	}
	resp := map[string]any{"date": time.Now().Format("2006-01-02"), "reading": out, "pages_today": total}
	if a, err := s.readingActivity(ctx, uid); err == nil {
		resp["streak"], resp["pages_7"] = a.Streak, a.Pages7
	}
	writeJSON(w, http.StatusOK, resp)
}
