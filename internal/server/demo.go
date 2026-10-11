package server

import (
	"context"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

// seedDemo fills an empty shelf with a believable reading life so screenshots
// and first impressions work. No network: books have no covers.
func (s *Server) seedDemo(ctx context.Context) error {
	users, _ := s.store.ListUsers(ctx)
	if len(users) == 0 {
		u := &store.User{Email: "admin@localhost", Name: "Admin", Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		users = []store.User{*u}
	}
	uid := users[0].ID
	if books, _ := s.store.ListBooks(ctx, store.BookFilter{UserID: uid, Limit: 1}); len(books) > 0 {
		return nil
	}
	now := time.Now()
	day := func(monthsAgo, d int) string {
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -monthsAgo, d-1).Format("2006-01-02")
	}
	type row struct {
		title, authors, isbn, tags, format, status, review, plan string
		pages, year                                              int
		rating                                                   float64
		started, finished                                        string
		page                                                     int
		fav                                                      bool
	}
	rows := []row{
		{"The Left Hand of Darkness", "Ursula K. Le Guin", "9780441478125", "sci-fi, classics", "paper", "read",
			"Slow to start, then it takes over. The ice crossing is one of the best things I have read — two people learning to trust each other with nothing else around.", "", 304, 1969, 5, day(7, 3), day(6, 20), 0, true},
		{"Project Hail Mary", "Andy Weir", "9780593135204", "sci-fi", "audio", "read",
			"Pure fun. Rocky alone is worth it. The science-as-puzzle structure works better here than in The Martian.", "", 496, 2021, 4.5, day(5, 2), day(5, 18), 0, false},
		{"Thinking, Fast and Slow", "Daniel Kahneman", "9780374533557", "psychology, non-fiction", "ebook", "read",
			"Important ideas, but long. The first third would have been enough for me.", "", 499, 2011, 3.5, day(4, 1), day(3, 12), 0, false},
		{"Piranesi", "Susanna Clarke", "9781635575637", "fantasy", "paper", "read",
			"Strange and gentle. The house will stay with me for a long time.", "", 272, 2020, 4.5, day(2, 5), day(2, 15), 0, true},
		{"Deep Work", "Cal Newport", "9781455586691", "productivity, non-fiction", "ebook", "abandoned",
			"Got the point after 80 pages; the rest repeats it.", "", 296, 2016, 2, day(1, 2), day(1, 9), 0, false},
		{"The Dispossessed", "Ursula K. Le Guin", "9780061054884", "sci-fi, classics", "paper", "reading", "", "", 387, 1974, 0, day(0, 1), "", 142, false},
		{"Atomic Habits", "James Clear", "9780735211292", "productivity, non-fiction", "audio", "reading", "", "", 320, 2018, 0, day(0, 3), "", 0, false},
		{"Dune", "Frank Herbert", "9780441172719", "sci-fi, classics", "paper", "want", "", "Before the films — Marko keeps telling me to", 688, 1965, 0, "", "", 0, false},
		{"The Name of the Wind", "Patrick Rothfuss", "9780756404741", "fantasy", "ebook", "want", "", "", 662, 2007, 0, "", "", 0, false},
		{"Sapiens", "Yuval Noah Harari", "9780062316097", "history, non-fiction", "paper", "want", "", "Holiday read", 443, 2011, 0, "", "", 0, false},
		{"The Midnight Library", "Matt Haig", "9780525559474", "fiction", "ebook", "want", "", "", 304, 2020, 0, "", "", 0, false},
	}
	nextMonth := now.AddDate(0, 1, 0).Format("2006-01")
	for _, r := range rows {
		b := &store.Book{UserID: uid, Title: r.title, Authors: r.authors, ISBN13: r.isbn, Tags: r.tags, Format: r.format, Status: r.status,
			Review: r.review, PlanNote: r.plan, Pages: r.pages, Rating: r.rating, CurrentPage: r.page, Favorite: r.fav, Source: "manual"}
		if r.year > 0 {
			b.Published = itoa(r.year)
		}
		if r.title == "Sapiens" {
			b.PlannedFor = nextMonth
		}
		if r.status == "read" {
			b.CurrentPage = r.pages
		}
		if err := s.store.CreateBook(ctx, b, r.started, r.finished); err != nil {
			return err
		}
	}
	quotes := []struct{ book, text, page string }{
		{"The Left Hand of Darkness", "Light is the left hand of darkness and darkness the right hand of light.", "233"},
		{"The Left Hand of Darkness", "It is good to have an end to journey toward; but it is the journey that matters, in the end.", "220"},
		{"Piranesi", "The Beauty of the House is immeasurable; its Kindness infinite.", "5"},
		{"Project Hail Mary", "Fist my bump.", ""},
	}
	books, _ := s.store.ListBooks(ctx, store.BookFilter{UserID: uid})
	byTitle := map[string]int64{}
	for _, b := range books {
		byTitle[b.Title] = b.ID
	}
	for _, q := range quotes {
		_ = s.store.CreateQuote(ctx, uid, &store.Quote{BookID: byTitle[q.book], Text: q.text, Page: q.page})
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
