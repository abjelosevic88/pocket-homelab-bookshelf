package server

import "testing"

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
