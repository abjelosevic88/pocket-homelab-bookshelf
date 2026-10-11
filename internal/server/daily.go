package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// Daily highlights: every morning a few highlights are picked (long-unseen
// ones first, favourites more often) and sent to the phone via Apprise, ntfy
// or a webhook; the Daily page shows them with favourite / hide.

func (s *Server) handleDaily(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = store.Today()
	}
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	ds, _ := s.store.GetDailySettings(r.Context(), me(r))
	set, err := s.store.DailySetFor(r.Context(), me(r), date, ds.Count)
	if err != nil {
		s.fail(w, err, "daily")
		return
	}
	days, _ := s.store.DailyDays(r.Context(), me(r), 400)
	writeJSON(w, http.StatusOK, map[string]any{"set": set, "days": days, "today": store.Today()})
}

// handleDailyShuffle picks a new set for today.
func (s *Server) handleDailyShuffle(w http.ResponseWriter, r *http.Request) {
	ds, _ := s.store.GetDailySettings(r.Context(), me(r))
	set, err := s.store.NewDailySet(r.Context(), me(r), ds.Count)
	if err != nil {
		s.fail(w, err, "daily")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"set": set})
}

// handleDailySend sends today's set now (also used as "Send a test").
func (s *Server) handleDailySend(w http.ResponseWriter, r *http.Request) {
	if err := s.sendDaily(r.Context(), me(r)); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetDailySettings(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDailySettings(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "daily settings")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handlePutDailySettings(w http.ResponseWriter, r *http.Request) {
	var d store.DailySettings
	if err := decode(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if d.Count < 1 || d.Count > 10 {
		d.Count = 5
	}
	if _, err := time.Parse("15:04", d.Time); err != nil {
		writeErr(w, http.StatusBadRequest, "time must be HH:MM")
		return
	}
	if d.Channel != "ntfy" && d.Channel != "webhook" {
		d.Channel = "apprise"
	}
	d.URL = strings.TrimSpace(d.URL)
	if d.Enabled && !strings.HasPrefix(d.URL, "http://") && !strings.HasPrefix(d.URL, "https://") {
		writeErr(w, http.StatusBadRequest, "add the address to send to (http://… or https://…)")
		return
	}
	if err := s.store.SaveDailySettings(r.Context(), me(r), d); err != nil {
		s.fail(w, err, "daily settings")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleQuoteFlags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Favorite *bool `json:"favorite"`
		Hidden   *bool `json:"hidden"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.SetQuoteFlags(r.Context(), me(r), idParam(r, "id"), in.Favorite, in.Hidden); err != nil {
		s.fail(w, err, "quote")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- delivery ----

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		cut := string(r[:n])
		if i := strings.LastIndex(cut, " "); i > n/2 {
			cut = cut[:i]
		}
		return cut + "…"
	}
	return s
}

// dailyMessage renders a set as a short plain-text notification.
func dailyMessage(set *store.DailySet) string {
	var b strings.Builder
	for i, q := range set.Quotes {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "“%s”\n— %s", clip(q.Text, 300), q.BookTitle)
		if q.BookAuthors != "" {
			fmt.Fprintf(&b, ", %s", strings.Split(q.BookAuthors, ",")[0])
		}
	}
	return b.String()
}

func (s *Server) sendDaily(ctx context.Context, userID int64) error {
	ds, err := s.store.GetDailySettings(ctx, userID)
	if err != nil {
		return err
	}
	if ds.URL == "" {
		return fmt.Errorf("no address to send to: set it under Settings → Daily highlights")
	}
	set, err := s.store.DailySetFor(ctx, userID, store.Today(), ds.Count)
	if err != nil {
		return err
	}
	if len(set.Quotes) == 0 {
		return fmt.Errorf("no highlights to send yet")
	}
	link := strings.TrimRight(s.cfg.BaseURL, "/") + "/daily"
	title := fmt.Sprintf("📚 Your highlights · %s", time.Now().Format("Mon 2 Jan"))
	body := dailyMessage(set)
	var req *http.Request
	switch ds.Channel {
	case "ntfy":
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, ds.URL, strings.NewReader(body))
		if err == nil {
			req.Header.Set("Title", title)
			req.Header.Set("Tags", "books")
			if s.cfg.BaseURL != "" {
				req.Header.Set("Click", link)
			}
		}
	case "webhook":
		payload, _ := json.Marshal(map[string]any{"title": title, "body": body, "url": link, "date": set.Date, "quotes": set.Quotes})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, ds.URL, bytes.NewReader(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	default: // Apprise API: POST /notify/<key> {title, body, type}
		if s.cfg.BaseURL != "" {
			body += "\n\n" + link
		}
		payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "type": "info", "format": "text"})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, ds.URL, bytes.NewReader(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if err != nil {
		return err
	}
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", ds.URL, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s answered HTTP %d: %s", ds.Channel, res.StatusCode, strings.TrimSpace(string(msg)))
	}
	return s.store.MarkDailySent(ctx, userID, set.Date)
}

// dailyScheduler checks every minute whether someone's daily highlights are due.
func (s *Server) dailyScheduler(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		users, err := s.store.EnabledDailyUsers(ctx)
		if err != nil {
			continue
		}
		now := time.Now().Format("15:04")
		for _, uid := range users {
			ds, err := s.store.GetDailySettings(ctx, uid)
			if err != nil || now < ds.Time {
				continue
			}
			set, err := s.store.DailySetFor(ctx, uid, store.Today(), ds.Count)
			if err != nil || set.SentAt != "" || len(set.Quotes) == 0 {
				continue
			}
			if err := s.sendDaily(ctx, uid); err != nil {
				s.log.Warn("daily highlights not sent", "user", uid, "err", err)
				_ = s.store.MarkDailySent(ctx, uid, set.Date) // one try a day; the Daily page still has them
				continue
			}
			s.log.Info("daily highlights sent", "user", uid, "count", len(set.Quotes))
		}
	}
}
