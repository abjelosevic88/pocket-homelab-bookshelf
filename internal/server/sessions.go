package server

import (
	"context"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Reading sessions are worked out from the e-reader's progress updates.
// KOReader (and CrossPoint) ask for the position when a book is opened or the
// device wakes, and send it on sleep, on close and every few pages. Between
// two positions the pages read are the share of the book times its page
// count; the minutes run from the device opening the book (or the previous
// update, when it came only a little earlier) to the update. When neither is
// known the minutes are estimated from the person's own pace and marked so.
const (
	sessionCarryOver = 45 * time.Minute // an update this soon after the last one continues the reading
	sessionJoin      = 15 * time.Minute // a break this short keeps it one session
	sessionMaxPerPg  = 6.0              // minutes a page; more means the device lay open
	sessionDefaultPg = 1.5              // minutes a page before the person's own pace is known
)

// ReadingSession is one sitting with a book on an e-reader.
type ReadingSession struct {
	BookID    int64   `json:"book_id"`
	Title     string  `json:"title"`
	Authors   string  `json:"authors"`
	Day       string  `json:"day"` // local day it started
	Start     string  `json:"start"`
	End       string  `json:"end"`
	Minutes   int     `json:"minutes"`
	Pages     int     `json:"pages"` // 0 when the book has no page count
	From      float64 `json:"from"`  // share of the book, 0–1
	To        float64 `json:"to"`
	FromPage  int     `json:"from_page,omitempty"`
	ToPage    int     `json:"to_page,omitempty"`
	Device    string  `json:"device"`
	Estimated bool    `json:"estimated"` // minutes guessed from the pace for part of it
	Updates   int     `json:"updates"`

	start, end int64
	pages      float64
	minutes    float64
}

type sessionSeg struct {
	doc        string
	start, end int64
	from, to   float64
	pages      float64
	minutes    float64
	estimated  bool
	known      bool
	e          store.ReaderEvent
}

// buildSessions turns progress updates (oldest first) into sessions, oldest
// first, and returns the minutes a page it used for estimates.
func buildSessions(events []store.ReaderEvent, loc *time.Location) ([]ReadingSession, float64) {
	byDoc := map[string][]store.ReaderEvent{}
	var docs []string
	for _, e := range events {
		if _, ok := byDoc[e.Document]; !ok {
			docs = append(docs, e.Document)
		}
		byDoc[e.Document] = append(byDoc[e.Document], e)
	}
	// First pass: the forward steps, with their minutes where they are known.
	var segs []sessionSeg
	for _, doc := range docs {
		var prev *store.ReaderEvent
		var lastOpen int64
		for i := range byDoc[doc] {
			e := byDoc[doc][i]
			if e.Source == "open" {
				lastOpen = e.Timestamp
				continue
			}
			if prev == nil {
				prev = &e
				continue
			}
			if e.Percentage <= prev.Percentage {
				// The same place again, or a jump back: nothing read; carry on from here.
				prev = &e
				continue
			}
			sg := sessionSeg{doc: doc, end: e.Timestamp, from: prev.Percentage, to: e.Percentage, e: e}
			if e.BookPages > 0 {
				sg.pages = (e.Percentage - prev.Percentage) * float64(e.BookPages)
			}
			gap := time.Duration(e.Timestamp-prev.Timestamp) * time.Second
			switch {
			case lastOpen > prev.Timestamp && lastOpen < e.Timestamp:
				sg.start, sg.known = lastOpen, true
			case gap <= sessionCarryOver:
				sg.start, sg.known = prev.Timestamp, true
			}
			if sg.known {
				sg.minutes = float64(e.Timestamp-sg.start) / 60
				if sg.pages > 0 && sg.minutes/sg.pages > sessionMaxPerPg {
					sg.known = false // the device lay open
				}
			}
			segs = append(segs, sg)
			prev = &e
		}
	}
	// The person's pace from the steps with known minutes.
	var kp, km float64
	for _, sg := range segs {
		if sg.known && sg.pages > 0 {
			kp += sg.pages
			km += sg.minutes
		}
	}
	perPage := sessionDefaultPg
	if kp >= 20 && km > 0 {
		perPage = math.Min(5, math.Max(0.3, km/kp))
	}
	for i := range segs {
		sg := &segs[i]
		if !sg.known {
			sg.estimated = true
			sg.minutes = math.Max(1, sg.pages*perPage)
			if sg.pages == 0 {
				sg.minutes = math.Max(1, (sg.to-sg.from)*300*perPage) // no page count: a 300-page book
			}
			sg.start = sg.end - int64(sg.minutes*60)
		}
	}
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].start < segs[j].start })
	// Second pass: steps of one document close together are one session.
	var out []ReadingSession
	open := map[string]int{} // document → index in out of its latest session
	for _, sg := range segs {
		if i, ok := open[sg.doc]; ok && sg.start <= out[i].end+int64(sessionJoin/time.Second) {
			s := &out[i]
			s.end = max(s.end, sg.end)
			s.To = sg.to
			s.pages += sg.pages
			s.minutes += sg.minutes
			s.Estimated = s.Estimated || sg.estimated
			s.Updates++
			continue
		}
		out = append(out, ReadingSession{BookID: sg.e.BookID, Title: sg.e.BookTitle, Authors: sg.e.BookAuth, From: sg.from, To: sg.to,
			Device: sg.e.Device, Estimated: sg.estimated, Updates: 1, start: sg.start, end: sg.end, pages: sg.pages, minutes: sg.minutes})
		open[sg.doc] = len(out) - 1
	}
	for i := range out {
		s := &out[i]
		s.To = math.Max(s.To, s.From)
		st, en := time.Unix(s.start, 0).In(loc), time.Unix(s.end, 0).In(loc)
		s.Day, s.Start, s.End = st.Format("2006-01-02"), st.Format(time.RFC3339), en.Format(time.RFC3339)
		s.Pages = int(math.Round(s.pages))
		s.Minutes = max(1, int(math.Round(s.minutes)))
		s.From, s.To = math.Round(s.From*10000)/10000, math.Round(s.To*10000)/10000
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out, perPage
}

// sessionPages fills the page numbers from the books' page counts.
func sessionPages(ss []ReadingSession, events []store.ReaderEvent) {
	pages := map[int64]int{}
	for _, e := range events {
		pages[e.BookID] = e.BookPages
	}
	for i := range ss {
		if n := pages[ss[i].BookID]; n > 0 {
			ss[i].FromPage = int(math.Round(ss[i].From * float64(n)))
			ss[i].ToPage = int(math.Round(ss[i].To * float64(n)))
		}
	}
}

func (s *Server) readingSessions(ctx context.Context, uid int64) ([]ReadingSession, float64, error) {
	events, err := s.store.ReaderEvents(ctx, uid, 0)
	if err != nil {
		return nil, 0, err
	}
	ss, perPage := buildSessions(events, time.Local)
	sessionPages(ss, events)
	return ss, perPage, nil
}

// SessionTotals add up a list of sessions.
type SessionTotals struct {
	Sessions     int     `json:"sessions"`
	Minutes      int     `json:"minutes"`
	Pages        int     `json:"pages"`
	PagesPerHour float64 `json:"pages_per_hour"`
}

func totals(ss []ReadingSession) SessionTotals {
	t := SessionTotals{Sessions: len(ss)}
	for _, s := range ss {
		t.Minutes += s.Minutes
		t.Pages += s.Pages
	}
	if t.Minutes > 0 && t.Pages > 0 {
		t.PagesPerHour = math.Round(float64(t.Pages)/float64(t.Minutes)*600) / 10
	}
	return t
}

func newestFirst(ss []ReadingSession) []ReadingSession {
	out := make([]ReadingSession, len(ss))
	for i, s := range ss {
		out[len(ss)-1-i] = s
	}
	return out
}

// handleBookSessions is a book's reading log: every session, newest first.
func (s *Server) handleBookSessions(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBook(r.Context(), me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get book")
		return
	}
	all, perPage, err := s.readingSessions(r.Context(), b.UserID)
	if err != nil {
		s.fail(w, err, "sessions")
		return
	}
	ss := []ReadingSession{}
	for _, x := range all {
		if x.BookID == b.ID {
			ss = append(ss, x)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": newestFirst(ss), "totals": totals(ss), "minutes_per_page": math.Round(perPage*100) / 100})
}

// handleReadingSessions lists the sessions from one day to another
// (YYYY-MM-DD, both included; today when left out), newest first, for Pocket Day.
func (s *Server) handleReadingSessions(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	today := time.Now().Format("2006-01-02")
	if _, err := time.Parse("2006-01-02", from); err != nil {
		from = today
	}
	if _, err := time.Parse("2006-01-02", to); err != nil {
		to = from
	}
	all, perPage, err := s.readingSessions(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "sessions")
		return
	}
	ss := []ReadingSession{}
	for _, x := range all {
		if x.Day >= from && x.Day <= to {
			ss = append(ss, x)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "sessions": newestFirst(ss), "totals": totals(ss), "minutes_per_page": math.Round(perPage*100) / 100})
}
