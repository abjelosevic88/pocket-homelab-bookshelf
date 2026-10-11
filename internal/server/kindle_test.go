package server

import (
	"testing"
	"time"
)

func TestKindleTitleMatching(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Sapiens: A Brief History of Humankind", "Sapiens", true},
		{"Moć sadašnjeg trenutka", "Moc sadasnjeg trenutka", true},
		{"The Eye of the World (The Wheel of Time, Book 1)", "The Eye of the World", true},
		{"Clean Code", "Clean Coder", false},
		{"1984", "Nineteen Eighty-Four", false},
	}
	for _, c := range cases {
		if got := sameTitle(c.a, c.b); got != c.want {
			t.Errorf("sameTitle(%q, %q) = %v", c.a, c.b, got)
		}
	}
	if !authorMatches("Eckhart Tolle", "Eckhart Tolle") || authorMatches("Jo Nesbø", "Stephen King") || !authorMatches("Jo Nesbo", "Jo Nesbø") {
		t.Error("authorMatches")
	}
}

func TestDailyDue(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, time.Local) }
	cases := []struct {
		now  time.Time
		want bool
	}{{at(7, 29), false}, {at(7, 30), true}, {at(9, 29), true}, {at(9, 31), false}, {at(23, 28), false}}
	for _, c := range cases {
		if got := dailyDue(c.now, "07:30"); got != c.want {
			t.Errorf("%s: got %v", c.now.Format("15:04"), got)
		}
	}
}
