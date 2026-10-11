package store

import (
	"context"
	"database/sql"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DailySettings say when and where a user's daily highlights go.
type DailySettings struct {
	Enabled bool   `json:"enabled"`
	Count   int    `json:"count"`
	Time    string `json:"time"`    // HH:MM local
	Channel string `json:"channel"` // apprise | ntfy | webhook
	URL     string `json:"url"`
}

// DailySet is the highlights picked for one day.
type DailySet struct {
	Date     string  `json:"date"`
	QuoteIDs []int64 `json:"-"`
	Quotes   []Quote `json:"quotes"`
	SentAt   string  `json:"sent_at"`
}

// GetDailySettings returns a user's settings (defaults when never saved).
func (s *Store) GetDailySettings(ctx context.Context, userID int64) (DailySettings, error) {
	d := DailySettings{Count: 5, Time: "07:30", Channel: "apprise"}
	var en int
	err := s.DB.QueryRowContext(ctx, `SELECT enabled, count, time, channel, url FROM daily_settings WHERE user_id = ?`, userID).Scan(&en, &d.Count, &d.Time, &d.Channel, &d.URL)
	if err != nil && err != sql.ErrNoRows {
		return d, err
	}
	d.Enabled = en == 1
	return d, nil
}

// SaveDailySettings stores a user's settings.
func (s *Store) SaveDailySettings(ctx context.Context, userID int64, d DailySettings) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO daily_settings (user_id, enabled, count, time, channel, url) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET enabled = excluded.enabled, count = excluded.count, time = excluded.time, channel = excluded.channel, url = excluded.url`,
		userID, boolInt(d.Enabled), d.Count, d.Time, d.Channel, d.URL)
	return err
}

// EnabledDailyUsers lists users with the daily review switched on.
func (s *Store) EnabledDailyUsers(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM daily_settings WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type candidate struct {
	id, book int64
	fav      bool
	last     string
}

// pickQuotes chooses n highlights: never-shown and long-unseen ones are
// likelier, favourites count double, and no book appears twice.
func pickQuotes(cands []candidate, n int, today time.Time, rnd *rand.Rand) []int64 {
	type scored struct {
		c candidate
		w float64
	}
	var pool []scored
	for _, c := range cands {
		days := 400.0 // never shown
		if c.last != "" {
			if t, err := time.Parse("2006-01-02", c.last[:min(10, len(c.last))]); err == nil {
				days = math.Max(0, today.Sub(t).Hours()/24)
			}
		}
		if days < 7 { // just seen: rest for a week unless nothing else is left
			days = 0.05
		}
		w := math.Min(days, 400)
		if c.fav {
			w *= 2
		}
		pool = append(pool, scored{c, w})
	}
	var out []int64
	usedBook := map[int64]bool{}
	for len(out) < n && len(pool) > 0 {
		total := 0.0
		for _, p := range pool {
			if !usedBook[p.c.book] {
				total += p.w
			}
		}
		if total == 0 { // only repeats of used books are left
			usedBook = map[int64]bool{}
			for _, p := range pool {
				total += p.w
			}
		}
		r := rnd.Float64() * total
		pick, last := -1, -1
		for i, p := range pool {
			if usedBook[p.c.book] {
				continue
			}
			last = i
			if r -= p.w; r <= 0 {
				pick = i
				break
			}
		}
		if pick < 0 { // rounding left a sliver: take the last eligible one
			pick = last
		}
		if pick < 0 {
			break
		}
		out = append(out, pool[pick].c.id)
		usedBook[pool[pick].c.book] = true
		pool = append(pool[:pick], pool[pick+1:]...)
	}
	return out
}

// DailySetFor returns the highlights of a day, picking (and recording as
// shown) a fresh set for today when there is none yet. Past days without a
// set return an empty one.
func (s *Store) DailySetFor(ctx context.Context, userID int64, date string, count int) (*DailySet, error) {
	set, err := s.loadDailySet(ctx, userID, date)
	if err != nil || set != nil || date != Today() {
		if set == nil && err == nil {
			set = &DailySet{Date: date, Quotes: []Quote{}}
		}
		return set, err
	}
	return s.NewDailySet(ctx, userID, count)
}

// NewDailySet picks today's highlights again (replacing any set for today).
func (s *Store) NewDailySet(ctx context.Context, userID int64, count int) (*DailySet, error) {
	if count < 1 || count > 10 {
		count = 5
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT q.id, q.book_id, q.favorite, q.last_shown FROM quotes q JOIN books b ON b.id = q.book_id
		WHERE b.user_id = ? AND q.hidden = 0 AND length(q.text) >= 20`, userID)
	if err != nil {
		return nil, err
	}
	var cands []candidate
	for rows.Next() {
		var c candidate
		var fav int
		if err := rows.Scan(&c.id, &c.book, &fav, &c.last); err != nil {
			rows.Close()
			return nil, err
		}
		c.fav = fav == 1
		cands = append(cands, c)
	}
	rows.Close()
	today := Today()
	t, _ := time.ParseInLocation("2006-01-02", today, time.Local)
	ids := pickQuotes(cands, count, t, rand.New(rand.NewSource(time.Now().UnixNano())))
	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = strconv.FormatInt(id, 10)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `INSERT INTO daily_sets (user_id, date, quote_ids) VALUES (?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET quote_ids = excluded.quote_ids`, userID, today, strings.Join(strIDs, ",")); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE quotes SET last_shown = ?, shown_count = shown_count + 1 WHERE id = ?`, today, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.loadDailySet(ctx, userID, today)
}

func (s *Store) loadDailySet(ctx context.Context, userID int64, date string) (*DailySet, error) {
	var ids, sent string
	err := s.DB.QueryRowContext(ctx, `SELECT quote_ids, sent_at FROM daily_sets WHERE user_id = ? AND date = ?`, userID, date).Scan(&ids, &sent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	set := &DailySet{Date: date, SentAt: sent, Quotes: []Quote{}}
	all, err := s.ListQuotes(ctx, QuoteFilter{UserID: userID})
	if err != nil {
		return nil, err
	}
	byID := map[int64]Quote{}
	for _, q := range all {
		byID[q.ID] = q
	}
	for _, p := range strings.Split(ids, ",") {
		id, _ := strconv.ParseInt(p, 10, 64)
		if q, ok := byID[id]; ok { // a deleted quote just drops out
			set.QuoteIDs = append(set.QuoteIDs, id)
			set.Quotes = append(set.Quotes, q)
		}
	}
	return set, nil
}

// MarkDailySent records that a day's set went out.
func (s *Store) MarkDailySent(ctx context.Context, userID int64, date string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE daily_sets SET sent_at = ? WHERE user_id = ? AND date = ?`, Now(), userID, date)
	return err
}

// DailyDays lists the days that have a set, newest first.
func (s *Store) DailyDays(ctx context.Context, userID int64, limit int) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date FROM daily_sets WHERE user_id = ? ORDER BY date DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, rows.Err()
}

// SetQuoteFlags marks a quote favourite and/or hidden.
func (s *Store) SetQuoteFlags(ctx context.Context, userID, id int64, favorite, hidden *bool) error {
	sets, args := []string{}, []any{}
	if favorite != nil {
		sets, args = append(sets, "favorite = ?"), append(args, boolInt(*favorite))
	}
	if hidden != nil {
		sets, args = append(sets, "hidden = ?"), append(args, boolInt(*hidden))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id, userID)
	res, err := s.DB.ExecContext(ctx, `UPDATE quotes SET `+strings.Join(sets, ", ")+` WHERE id = ? AND book_id IN (SELECT id FROM books WHERE user_id = ?)`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
