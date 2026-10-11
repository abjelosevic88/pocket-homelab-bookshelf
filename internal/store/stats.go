package store

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MonthStat is what was finished in one month.
type MonthStat struct {
	Books int `json:"books"`
	Pages int `json:"pages"`
}

// Totals summarise a set of finished reads.
type Totals struct {
	Books     int     `json:"books"`
	Pages     int     `json:"pages"`
	Rated     int     `json:"rated"`
	AvgRating float64 `json:"avg_rating"`
}

// Stats is the reading summary of one year (plus all-time totals).
type Stats struct {
	Year       int           `json:"year"`
	Years      []int         `json:"years"`
	Goal       int           `json:"goal"`
	Totals     Totals        `json:"totals"`
	Abandoned  int           `json:"abandoned"`
	Months     [12]MonthStat `json:"months"`
	Ratings    [10]int       `json:"ratings"` // index 0 = ½ star … 9 = 5 stars
	TopAuthors []TagCount    `json:"top_authors"`
	TopTags    []TagCount    `json:"top_tags"`
	Formats    []TagCount    `json:"formats"`
	Finished   []BookLine    `json:"finished"`
	Longest    *BookLine     `json:"longest"`
	AllTime    Totals        `json:"all_time"`
	Quotes     int           `json:"quotes"`
}

type finishedRow struct {
	id                           int64
	title, authors, tags, format string
	pages                        int
	rating                       float64
	date                         string
	abandoned                    bool
}

func (s *Store) finishedReads(ctx context.Context, userID int64) ([]finishedRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.title, b.authors, b.tags, b.format, b.pages, b.rating, r.finished_on, r.abandoned
		FROM reads r JOIN books b ON b.id = r.book_id WHERE b.user_id = ? AND r.finished_on <> '' ORDER BY r.finished_on, r.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []finishedRow
	for rows.Next() {
		var f finishedRow
		var ab int
		if err := rows.Scan(&f.id, &f.title, &f.authors, &f.tags, &f.format, &f.pages, &f.rating, &f.date, &ab); err != nil {
			return nil, err
		}
		f.abandoned = ab == 1
		out = append(out, f)
	}
	return out, rows.Err()
}

func addTotals(t *Totals, f finishedRow, sum *float64) {
	t.Books++
	t.Pages += f.pages
	if f.rating > 0 {
		t.Rated++
		*sum += f.rating
	}
}

func avg(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return math.Round(sum/float64(n)*100) / 100
}

// GetStats computes the summary for one year.
func (s *Store) GetStats(ctx context.Context, userID int64, year, goal int) (*Stats, error) {
	if year <= 0 {
		year = time.Now().Year()
	}
	rows, err := s.finishedReads(ctx, userID)
	if err != nil {
		return nil, err
	}
	st := &Stats{Year: year, Goal: goal, Finished: []BookLine{}}
	years := map[int]bool{time.Now().Year(): true}
	authors, tags, formats := map[string]int{}, map[string]int{}, map[string]int{}
	names := map[string]string{}
	var sumYear, sumAll float64
	ystr := strconv.Itoa(year)
	for _, f := range rows {
		if y, err := strconv.Atoi(f.date[:min(4, len(f.date))]); err == nil {
			years[y] = true
		}
		if f.abandoned {
			if strings.HasPrefix(f.date, ystr) {
				st.Abandoned++
			}
			continue
		}
		addTotals(&st.AllTime, f, &sumAll)
		if !strings.HasPrefix(f.date, ystr) {
			continue
		}
		addTotals(&st.Totals, f, &sumYear)
		if m, err := strconv.Atoi(f.date[5:min(7, len(f.date))]); err == nil && m >= 1 && m <= 12 {
			st.Months[m-1].Books++
			st.Months[m-1].Pages += f.pages
		}
		if f.rating > 0 {
			if i := int(math.Round(f.rating*2)) - 1; i >= 0 && i < 10 {
				st.Ratings[i]++
			}
		}
		for _, a := range strings.Split(f.authors, ",") {
			if a = strings.TrimSpace(a); a != "" {
				authors[strings.ToLower(a)]++
				names[strings.ToLower(a)] = a
			}
		}
		for _, t := range strings.Split(f.tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags[strings.ToLower(t)]++
				names[strings.ToLower(t)] = t
			}
		}
		fm := f.format
		if fm == "" {
			fm = "unknown"
		}
		formats[fm]++
		names[fm] = fm
		line := BookLine{ID: f.id, Title: f.title, Authors: f.authors, Rating: f.rating, Pages: f.pages, Date: f.date}
		st.Finished = append(st.Finished, line)
		if f.pages > 0 && (st.Longest == nil || f.pages > st.Longest.Pages) {
			l := line
			st.Longest = &l
		}
	}
	st.Totals.AvgRating = avg(sumYear, st.Totals.Rated)
	st.AllTime.AvgRating = avg(sumAll, st.AllTime.Rated)
	// Books marked read without a date still count all-time.
	var undated int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM books b WHERE b.user_id = ? AND b.status = 'read'
		AND NOT EXISTS (SELECT 1 FROM reads r WHERE r.book_id = b.id AND r.finished_on <> '' AND r.abandoned = 0)`, userID).Scan(&undated)
	st.AllTime.Books += undated
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM quotes q JOIN books b ON b.id = q.book_id WHERE b.user_id = ?`, userID).Scan(&st.Quotes)
	for y := range years {
		st.Years = append(st.Years, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(st.Years)))
	st.TopAuthors = sortedCounts(authors, names, 8)
	st.TopTags = sortedCounts(tags, names, 10)
	st.Formats = sortedCounts(formats, names, 0)
	// newest first reads better in the list
	for i, j := 0, len(st.Finished)-1; i < j; i, j = i+1, j-1 {
		st.Finished[i], st.Finished[j] = st.Finished[j], st.Finished[i]
	}
	return st, nil
}

func sortedCounts(counts map[string]int, names map[string]string, limit int) []TagCount {
	out := make([]TagCount, 0, len(counts))
	for k, n := range counts {
		name := names[k]
		if name == "" {
			name = k
		}
		out = append(out, TagCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func progressPct(cur, pages int) int {
	if pages <= 0 || cur <= 0 {
		return 0
	}
	return min(100, cur*100/pages)
}

// GetDashboard returns the compact status for widgets.
func (s *Store) GetDashboard(ctx context.Context, userID int64, goal int) (*Dashboard, error) {
	d := &Dashboard{YearlyGoal: goal, CurrentlyRead: []BookLine{}}
	q := func(dst *int, query string, args ...any) {
		_ = s.DB.QueryRowContext(ctx, query, args...).Scan(dst)
	}
	q(&d.Books, `SELECT COUNT(1) FROM books WHERE user_id = ?`, userID)
	q(&d.Reading, `SELECT COUNT(1) FROM books WHERE user_id = ? AND status = 'reading'`, userID)
	q(&d.Want, `SELECT COUNT(1) FROM books WHERE user_id = ? AND status = 'want'`, userID)
	q(&d.ReadTotal, `SELECT COUNT(1) FROM books WHERE user_id = ? AND status = 'read'`, userID)
	q(&d.Quotes, `SELECT COUNT(1) FROM quotes q JOIN books b ON b.id = q.book_id WHERE b.user_id = ?`, userID)
	year := strconv.Itoa(time.Now().Year())
	q(&d.ReadThisYear, `SELECT COUNT(1) FROM reads r JOIN books b ON b.id = r.book_id WHERE b.user_id = ? AND r.abandoned = 0 AND substr(r.finished_on, 1, 4) = ?`, userID, year)
	q(&d.PagesThisYear, `SELECT COALESCE(SUM(b.pages), 0) FROM reads r JOIN books b ON b.id = r.book_id WHERE b.user_id = ? AND r.abandoned = 0 AND substr(r.finished_on, 1, 4) = ?`, userID, year)
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(ROUND(AVG(rating), 2), 0) FROM books WHERE user_id = ? AND rating > 0`, userID).Scan(&d.AvgRating)
	if goal > 0 {
		d.GoalPercent = min(100, d.ReadThisYear*100/goal)
	}
	reading, err := s.ListBooks(ctx, BookFilter{UserID: userID, Status: StatusReading, Sort: "recent"})
	if err != nil {
		return nil, err
	}
	for _, b := range reading {
		d.CurrentlyRead = append(d.CurrentlyRead, BookLine{ID: b.ID, Title: b.Title, Authors: b.Authors, Progress: progressPct(b.CurrentPage, b.Pages), Date: b.StartedOn})
	}
	if last, _ := s.ListBooks(ctx, BookFilter{UserID: userID, Status: StatusRead, Sort: "finished", Limit: 1}); len(last) > 0 && last[0].FinishedOn != "" {
		b := last[0]
		d.LastFinished = &BookLine{ID: b.ID, Title: b.Title, Authors: b.Authors, Rating: b.Rating, Date: b.FinishedOn}
	}
	if next, _ := s.ListBooks(ctx, BookFilter{UserID: userID, Status: StatusWant, Sort: "queue", Limit: 1}); len(next) > 0 {
		b := next[0]
		d.UpNext = &BookLine{ID: b.ID, Title: b.Title, Authors: b.Authors, Date: b.PlannedFor}
	}
	return d, nil
}
