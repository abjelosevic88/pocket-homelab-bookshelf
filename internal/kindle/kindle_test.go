package kindle

import (
	"strings"
	"testing"
)

const sample = "\ufeffThe Power of Now (Tolle, Eckhart)\r\n" +
	"- Your Highlight on page 21 | Location 312-314 | Added on Sunday, 3 May 2021 21:14:02\r\n" +
	"\r\n" +
	"Realize deeply that the present moment\r\n" +
	"==========\r\n" +
	"The Power of Now (Tolle, Eckhart)\r\n" +
	"- Your Highlight on page 21 | Location 312-315 | Added on Sunday, 3 May 2021 21:14:40\r\n" +
	"\r\n" +
	"Realize deeply that the present moment is all you ever have.\r\n" +
	"==========\r\n" +
	"The Power of Now (Tolle, Eckhart)\r\n" +
	"- Your Note on page 21 | Location 315 | Added on Sunday, 3 May 2021 21:15:00\r\n" +
	"\r\n" +
	"Core idea of the book\r\n" +
	"==========\r\n" +
	"The Power of Now (Tolle, Eckhart)\r\n" +
	"- Your Bookmark on page 40 | Location 600 | Added on Sunday, 3 May 2021 22:00:00\r\n" +
	"\r\n" +
	"\r\n" +
	"==========\r\n" +
	"Sapiens: A Brief History of Humankind (Yuval Noah Harari)\r\n" +
	"- Your Highlight at location 1201-1203 | Added on Monday, June 6, 2022 9:05:11 PM\r\n" +
	"\r\n" +
	"How do you cause people to believe in an imagined order?\r\n" +
	"==========\r\n" +
	"Sapiens: A Brief History of Humankind (Yuval Noah Harari)\r\n" +
	"- Your Highlight at location 900-901 | Added on Monday, June 6, 2022 9:00:00 PM\r\n" +
	"\r\n" +
	"<You have reached the clipping limit for this item>\r\n" +
	"==========\r\n" +
	"Sapiens: A Brief History of Humankind (Yuval Noah Harari)\r\n" +
	"- Your Note at location 2000 | Added on Monday, June 6, 2022 9:10:00 PM\r\n" +
	"\r\n" +
	"Check the footnote later\r\n" +
	"==========\r\n"

func TestParseAndGroup(t *testing.T) {
	clips, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 7 {
		t.Fatalf("want 7 clippings, got %d", len(clips))
	}
	c := clips[0]
	if c.Title != "The Power of Now" || c.Author != "Eckhart Tolle" || c.Kind != Highlight || c.Page != "21" || c.LocStart != 312 || c.LocEnd != 314 || c.Added == "" {
		t.Fatalf("first clipping: %+v", c)
	}
	if clips[4].Added == "" || clips[4].Page != "" || clips[4].LocStart != 1201 {
		t.Fatalf("US date / location-only clipping: %+v", clips[4])
	}
	books := Group(clips)
	if len(books) != 2 {
		t.Fatalf("want 2 books, got %d", len(books))
	}
	now := books[0]
	if len(now.Entries) != 1 {
		t.Fatalf("the extended highlight should replace the first: %+v", now.Entries)
	}
	if e := now.Entries[0]; e.Text != "Realize deeply that the present moment is all you ever have." || e.Note != "Core idea of the book" || e.Where() != "21" {
		t.Fatalf("power of now entry: %+v", e)
	}
	sap := books[1]
	if sap.Title != "Sapiens: A Brief History of Humankind" || sap.Author != "Yuval Noah Harari" || len(sap.Entries) != 2 {
		t.Fatalf("sapiens: %+v", sap)
	}
	if sap.Entries[0].Where() != "loc. 1201" || !sap.Entries[1].NoteOnly || sap.Entries[1].Text != "Check the footnote later" {
		t.Fatalf("sapiens entries: %+v", sap.Entries)
	}
}

func TestSplitTitle(t *testing.T) {
	for in, want := range map[string][2]string{
		"Metro 2033 (Glukhovsky, Dmitry)":                                  {"Metro 2033", "Dmitry Glukhovsky"},
		"The Eye of the World (The Wheel of Time, Book 1) (Robert Jordan)": {"The Eye of the World (The Wheel of Time, Book 1)", "Robert Jordan"},
		"Clean Code (Robert C. Martin;Dean Wampler)":                       {"Clean Code", "Robert C. Martin, Dean Wampler"},
		"Untitled document":                                                {"Untitled document", ""},
	} {
		ti, au := splitTitle(in)
		if ti != want[0] || au != want[1] {
			t.Errorf("%q → %q / %q", in, ti, au)
		}
	}
}
