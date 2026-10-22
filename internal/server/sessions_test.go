package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
)

func TestBuildSessions(t *testing.T) {
	loc := time.UTC
	at := func(h, m int) int64 { return time.Date(2026, 10, 22, h, m, 0, 0, loc).Unix() }
	ev := func(src string, ts int64, pct float64) store.ReaderEvent {
		return store.ReaderEvent{Document: "d1", Source: src, Device: "KOReader", Percentage: pct, Timestamp: ts, BookID: 7, BookTitle: "Book", BookPages: 1000}
	}
	events := []store.ReaderEvent{
		ev("device", at(8, 0), 0.10), // where the book was left
		// Morning: opened 20:00 later, two page-count syncs, closed.
		ev("open", at(20, 0), 0.10),
		ev("device", at(20, 10), 0.11),
		ev("device", at(20, 20), 0.12),
		ev("device", at(20, 30), 0.13), // 30 pages in 30 minutes
		// Same place synced again: nothing read.
		ev("device", at(21, 0), 0.13),
		// Next day: no open seen, long gap → estimated from the pace (1 min/page).
		ev("device", at(32, 0), 0.15),
		// Jump back: nothing counted.
		ev("device", at(33, 0), 0.05),
	}
	ss, perPage := buildSessions(events, loc)
	if len(ss) != 2 {
		t.Fatalf("sessions = %d, want 2: %+v", len(ss), ss)
	}
	if perPage != 1 {
		t.Errorf("per page = %v, want 1", perPage)
	}
	a := ss[0]
	if a.Day != "2026-10-22" || a.Pages != 30 || a.Minutes != 30 || a.Estimated || a.Updates != 3 {
		t.Errorf("first session = %+v", a)
	}
	b := ss[1]
	if b.Day != "2026-10-23" || b.Pages != 20 || b.Minutes != 20 || !b.Estimated {
		t.Errorf("second session = %+v", b)
	}
	sessionPages(ss, events)
	if ss[0].FromPage != 100 || ss[0].ToPage != 130 {
		t.Errorf("pages = %d → %d", ss[0].FromPage, ss[0].ToPage)
	}
	if tt := totals(ss); tt.Pages != 50 || tt.Minutes != 50 || tt.PagesPerHour != 60 {
		t.Errorf("totals = %+v", tt)
	}
}

func TestBuildSessionsDeviceLeftOpen(t *testing.T) {
	loc := time.UTC
	base := time.Date(2026, 10, 22, 10, 0, 0, 0, loc).Unix()
	events := []store.ReaderEvent{
		{Document: "d", Source: "device", Percentage: 0.5, Timestamp: base, BookID: 1, BookPages: 200},
		{Document: "d", Source: "open", Percentage: 0.5, Timestamp: base + 60},
		// Two pages in three hours: the device lay open, so it is estimated.
		{Document: "d", Source: "device", Percentage: 0.51, Timestamp: base + 3*3600, BookID: 1, BookPages: 200},
	}
	ss, _ := buildSessions(events, loc)
	if len(ss) != 1 || !ss[0].Estimated || ss[0].Minutes != 3 || ss[0].Pages != 2 {
		t.Fatalf("got %+v", ss)
	}
}

func TestReadingSessionsAreThePersonsOwn(t *testing.T) {
	h := newShelfHarness(t)
	ctx := context.Background()
	anaBook, _, _, _ := h.anaShelf()
	doc := "0123456789abcdef0123456789abcdef"
	if err := h.st.SaveKosyncProgress(ctx, h.ana.ID, &store.KosyncDocument{Document: doc, Progress: "x", Percentage: 0.1, Device: "KOReader", Timestamp: 1}); err != nil {
		t.Fatal(err)
	}
	if err := h.st.LinkKosyncDocument(ctx, h.ana.ID, doc, &anaBook, false); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for _, e := range []store.KosyncEvent{
		{Document: doc, Source: "device", Percentage: 0.10, Timestamp: now - 1800},
		{Document: doc, Source: "open", Percentage: 0.10, Timestamp: now - 1200},
		{Document: doc, Source: "device", Percentage: 0.15, Timestamp: now},
	} {
		e.Body = json.RawMessage(`{}`)
		if err := h.st.AddKosyncEvent(ctx, h.ana.ID, &e); err != nil {
			t.Fatal(err)
		}
	}
	var got struct {
		Sessions []ReadingSession
		Totals   SessionTotals
	}
	if code := h.json(h.anaTok, "GET", fmt.Sprintf("/books/%d/sessions", anaBook), nil, &got); code != 200 || len(got.Sessions) != 1 ||
		got.Sessions[0].Pages != 10 || got.Sessions[0].Minutes != 20 || got.Sessions[0].Estimated {
		t.Fatalf("Ana's log = %d %+v", code, got)
	}
	if code := h.json(h.anaTok, "GET", "/reading/sessions", nil, &got); code != 200 || got.Totals.Pages != 10 || got.Totals.Minutes != 20 {
		t.Fatalf("Ana's day = %d %+v", code, got)
	}
	if code, _ := h.do(h.bobTok, "GET", fmt.Sprintf("/books/%d/sessions", anaBook), nil); code != 404 {
		t.Errorf("Bob reading Ana's log = %d, want 404", code)
	}
	for _, tok := range []string{h.bobTok, "pcx_bob_books"} {
		if code, body := h.do(tok, "GET", "/reading/sessions", nil); code != 200 || strings.Contains(string(body), "Secret Diary") {
			t.Errorf("%s day = %d %s", tok, code, body)
		}
	}
	if code, _ := h.do("pcx_bob_other", "GET", "/reading/sessions", nil); code != 403 {
		t.Errorf("link without books:read = %d, want 403", code)
	}
}
