package metadata

import (
	"encoding/json"
	"testing"
)

func TestISBN(t *testing.T) {
	if got := NormalizeISBN("978-0-441-47812-5"); got != "9780441478125" {
		t.Fatal(got)
	}
	if got := NormalizeISBN("0-8044-2957-X"); got != "080442957X" {
		t.Fatal(got)
	}
	if NormalizeISBN("12345") != "" {
		t.Fatal("short accepted")
	}
	if got := ISBN10to13("0441478123"); got != "9780441478125" {
		t.Fatal(got)
	}
	for q, want := range map[string]bool{"9780441478125": true, "ISBN 978-0-441-47812-5": true, "isbn:0441478123": true, "dune 1965": false, "the 9780441478125": false} {
		if LooksLikeISBN(q) != want {
			t.Errorf("LooksLikeISBN(%q) != %v", q, want)
		}
	}
}

func TestOpenLibraryCandidate(t *testing.T) {
	var d olDoc
	_ = json.Unmarshal([]byte(`{"key":"/works/OL59800W","title":"The Left Hand of Darkness","author_name":["Ursula K. Le Guin"],
		"first_publish_year":1969,"isbn":["0441478069","9788445009314","9780441478125"],"cover_i":10618463,"number_of_pages_median":286,
		"publisher":["Ace"],"language":["eng","spa"],"subject":["Science fiction","Fiction, science fiction, general","nyt:x","Gender"]}`), &d)
	c := olCandidate(d, "9780441478125")
	if c.ISBN13 != "9780441478125" || c.Published != "1969" || c.Pages != 286 || c.Language != "" {
		t.Fatalf("%+v", c)
	}
	if c.CoverURL != "https://covers.openlibrary.org/b/id/10618463-L.jpg" || c.SourceURL != "https://openlibrary.org/works/OL59800W" {
		t.Fatalf("%+v", c)
	}
	if len(c.Subjects) != 2 || c.Subjects[0] != "Science fiction" {
		t.Fatalf("subjects %v", c.Subjects)
	}
	if olText(json.RawMessage(`{"type":"/type/text","value":"hi"}`)) != "hi" || olText(json.RawMessage(`"plain"`)) != "plain" {
		t.Fatal("olText")
	}
}

func TestGoogleCandidate(t *testing.T) {
	var v gbVolume
	_ = json.Unmarshal([]byte(`{"id":"abc","volumeInfo":{"title":"Dune","authors":["Frank Herbert"],"publishedDate":"1990-09-01",
		"description":"<p>Spice &amp; sand.</p><p>More</p>","pageCount":535,"categories":["Fiction / Science Fiction / General"],"language":"en",
		"industryIdentifiers":[{"type":"ISBN_10","identifier":"0441172717"}],
		"imageLinks":{"thumbnail":"http://books.google.com/books/content?id=abc&printsec=frontcover&img=1&zoom=1&edge=curl&source=gbs_api"}}}`), &v)
	c := gbCandidate(v)
	if c.Published != "1990" || c.ISBN13 != "9780441172719" || c.Description != "Spice & sand.\nMore" {
		t.Fatalf("%+v", c)
	}
	if c.CoverURL != "https://books.google.com/books/content?id=abc&printsec=frontcover&img=1&zoom=1&source=gbs_api" {
		t.Fatal(c.CoverURL)
	}
	if len(c.Subjects) != 2 || c.Subjects[1] != "Science Fiction" {
		t.Fatalf("subjects %v", c.Subjects)
	}
}

func TestMergeDedupesByISBN(t *testing.T) {
	a := []Candidate{{Source: "openlibrary", Title: "Dune", ISBN13: "1"}, {Source: "openlibrary", Title: "Other"}}
	b := []Candidate{{Source: "google", Title: "Dune", ISBN13: "1", Pages: 500, Description: "d"}, {Source: "google", Title: "Third"}}
	m := Merge(a, b, 0)
	if len(m) != 3 || m[0].Pages != 500 || m[0].Source != "openlibrary" || m[0].Description != "d" {
		t.Fatalf("%+v", m)
	}
}

func TestPublicOnly(t *testing.T) {
	for addr, blocked := range map[string]bool{"127.0.0.1:80": true, "10.0.0.5:443": true, "192.168.1.1:80": true, "100.111.226.27:443": true,
		"[::1]:80": true, "169.254.169.254:80": true, "207.241.224.2:443": false} {
		err := publicOnly("tcp", addr, nil)
		if (err != nil) != blocked {
			t.Errorf("%s: blocked=%v err=%v", addr, blocked, err)
		}
	}
}
