package store

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func openTest(t *testing.T) (*Store, int64) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u := &User{Email: "me@example.com", Name: "Me", Role: "admin"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return s, u.ID
}

func TestStatusFlowKeepsReadsInStep(t *testing.T) {
	s, uid := openTest(t)
	ctx := context.Background()
	b := &Book{UserID: uid, Title: "Dune", Authors: "Frank Herbert", Pages: 600, Tags: "sci-fi, Sci-Fi ,classics", Status: StatusWant}
	if err := s.CreateBook(ctx, b, "", ""); err != nil {
		t.Fatal(err)
	}
	if b.QueuePos != 1 || b.Tags != "sci-fi, classics" {
		t.Fatalf("queue %d tags %q", b.QueuePos, b.Tags)
	}
	got, err := s.SetStatus(ctx, uid, b.ID, StatusReading, "2026-01-02")
	if err != nil {
		t.Fatal(err)
	}
	if got.StartedOn != "2026-01-02" || got.FinishedOn != "" {
		t.Fatalf("after start: %+v", got)
	}
	// Starting again does not open a second read.
	if _, err := s.SetStatus(ctx, uid, b.ID, StatusReading, "2026-01-05"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.SetStatus(ctx, uid, b.ID, StatusRead, "2026-01-20")
	if got.FinishedOn != "2026-01-20" || got.ReadCount != 1 || got.CurrentPage != 600 {
		t.Fatalf("after finish: %+v", got)
	}
	reads, _ := s.ListReads(ctx, b.ID)
	if len(reads) != 1 || reads[0].StartedOn != "2026-01-02" {
		t.Fatalf("reads: %+v", reads)
	}
	// Re-read, then give up on it: the first read still counts.
	_, _ = s.SetStatus(ctx, uid, b.ID, StatusReading, "2026-06-01")
	got, _ = s.SetStatus(ctx, uid, b.ID, StatusAbandoned, "2026-06-10")
	if got.ReadCount != 1 || got.FinishedOn != "2026-01-20" {
		t.Fatalf("after abandon: %+v", got)
	}
	// Back to the plan drops nothing that was finished.
	got, _ = s.SetStatus(ctx, uid, b.ID, StatusWant, "")
	if got.Status != StatusWant || got.QueuePos == 0 {
		t.Fatalf("after want: %+v", got)
	}
	if reads, _ = s.ListReads(ctx, b.ID); len(reads) != 2 {
		t.Fatalf("want 2 reads, got %d", len(reads))
	}
}

func TestBooksArePerUser(t *testing.T) {
	s, uid := openTest(t)
	ctx := context.Background()
	other := &User{Email: "other@example.com", Role: "member"}
	_ = s.CreateUser(ctx, other)
	b := &Book{UserID: uid, Title: "Mine", Status: StatusRead}
	if err := s.CreateBook(ctx, b, "", "2026-03-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBook(ctx, other.ID, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user can see my book: %v", err)
	}
	if err := s.CreateQuote(ctx, other.ID, &Quote{BookID: b.ID, Text: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user can quote my book: %v", err)
	}
	if list, _ := s.ListBooks(ctx, BookFilter{UserID: other.ID}); len(list) != 0 {
		t.Fatalf("other user lists %d books", len(list))
	}
}

func TestStatsAndDashboard(t *testing.T) {
	s, uid := openTest(t)
	ctx := context.Background()
	y := time.Now().Year()
	ys := strconv.Itoa(y)
	add := func(title string, pages int, rating float64, finished string, status string) {
		b := &Book{UserID: uid, Title: title, Authors: "A. Author", Pages: pages, Rating: rating, Status: status, Tags: "fiction"}
		if err := s.CreateBook(ctx, b, "", finished); err != nil {
			t.Fatal(err)
		}
	}
	add("One", 100, 4, ys+"-02-10", StatusRead)
	add("Two", 300, 5, ys+"-02-20", StatusRead)
	add("Three", 200, 0, strconv.Itoa(y-1)+"-12-31", StatusRead)
	add("Quit", 500, 1, ys+"-03-01", StatusAbandoned)
	add("Undated", 50, 3, "", StatusRead)
	add("Next", 0, 0, "", StatusWant)
	st, err := s.GetStats(ctx, uid, y, 10)
	if err != nil {
		t.Fatal(err)
	}
	if st.Totals.Books != 2 || st.Totals.Pages != 400 || st.Totals.AvgRating != 4.5 || st.Abandoned != 1 {
		t.Fatalf("totals: %+v abandoned %d", st.Totals, st.Abandoned)
	}
	if st.Months[1].Books != 2 || st.Ratings[7] != 1 || st.Ratings[9] != 1 {
		t.Fatalf("months/ratings: %+v %+v", st.Months, st.Ratings)
	}
	if st.AllTime.Books != 4 { // One, Two, Three + Undated
		t.Fatalf("all time: %+v", st.AllTime)
	}
	if len(st.Years) != 2 || st.Years[0] != y {
		t.Fatalf("years: %v", st.Years)
	}
	if st.Longest == nil || st.Longest.Title != "Two" {
		t.Fatalf("longest: %+v", st.Longest)
	}
	d, err := s.GetDashboard(ctx, uid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if d.ReadThisYear != 2 || d.GoalPercent != 20 || d.Want != 1 || d.LastFinished == nil || d.LastFinished.Title != "Two" || d.UpNext == nil {
		t.Fatalf("dashboard: %+v", d)
	}
	if books, _ := s.ListBooks(ctx, BookFilter{UserID: uid, Year: y - 1}); len(books) != 1 || books[0].Title != "Three" {
		t.Fatalf("year filter: %+v", books)
	}
	if books, _ := s.ListBooks(ctx, BookFilter{UserID: uid, Tag: "FICTION"}); len(books) != 6 {
		t.Fatalf("tag filter: %d", len(books))
	}
}

func TestReorderQueue(t *testing.T) {
	s, uid := openTest(t)
	ctx := context.Background()
	var ids []int64
	for _, title := range []string{"A", "B", "C"} {
		b := &Book{UserID: uid, Title: title, Status: StatusWant}
		_ = s.CreateBook(ctx, b, "", "")
		ids = append(ids, b.ID)
	}
	if err := s.ReorderQueue(ctx, uid, []int64{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListBooks(ctx, BookFilter{UserID: uid, Status: StatusWant, Sort: "queue"})
	if list[0].Title != "C" || list[1].Title != "A" || list[2].Title != "B" {
		t.Fatalf("order: %s %s %s", list[0].Title, list[1].Title, list[2].Title)
	}
}
