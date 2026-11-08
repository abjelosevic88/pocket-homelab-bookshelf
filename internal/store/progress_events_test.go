package store

import (
	"context"
	"testing"
)

func TestProgressEventsJoinKosyncHistory(t *testing.T) {
	s, uid := openTest(t)
	ctx := context.Background()
	b := &Book{UserID: uid, Title: "Nebeski oganj", Authors: "Robert Jordan", Pages: 894, Status: StatusReading}
	if err := s.CreateBook(ctx, b, "2026-10-22", ""); err != nil {
		t.Fatal(err)
	}
	doc := "01a01cdb62f88f29ecbdbe3808822849"
	if err := s.SaveKosyncProgress(ctx, uid, &KosyncDocument{Document: doc, Percentage: 0.0646, Device: "CrossPoint Sync", Timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkKosyncDocument(ctx, uid, doc, &b.ID, false); err != nil {
		t.Fatal(err)
	}
	_ = s.AddKosyncEvent(ctx, uid, &KosyncEvent{Document: doc, Source: "device", Device: "CrossPoint Sync", Percentage: 0.0646, Timestamp: 1000, Body: []byte(`{}`)})

	// the same position through Library a minute later is not a second update
	_ = s.AddProgressEvent(ctx, uid, b.ID, "library", "CrossPoint", 0.0647, 1060)
	// the Kindle, later, through Library; twice from two polls, kept once
	_ = s.AddProgressEvent(ctx, uid, b.ID, "library", "KindlePaperWhite6", 0.0793, 5000)
	_ = s.AddProgressEvent(ctx, uid, b.ID, "library", "KindlePaperWhite6", 0.0793, 5000)

	ev, err := s.ListBookKosyncEvents(ctx, uid, b.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 || ev[0].Source != "library" || ev[0].Device != "KindlePaperWhite6" || ev[1].Source != "device" {
		t.Fatalf("events %+v", ev)
	}
	got, err := s.GetBook(ctx, uid, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReaderPercent == nil || *got.ReaderPercent != 0.0793 || got.ReaderSyncedAt != 5000 || got.ReaderDevice != "KindlePaperWhite6" {
		t.Fatalf("reader %v at %d from %q", got.ReaderPercent, got.ReaderSyncedAt, got.ReaderDevice)
	}
}
