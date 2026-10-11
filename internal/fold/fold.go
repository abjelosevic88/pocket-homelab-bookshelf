// Package fold normalises names and titles for comparison: lower case, no
// accents, no punctuation — "Moć sadašnjeg trenutka!" → "moc sadasnjeg trenutka".
package fold

import (
	"strings"
	"unicode"
)

var replacer = strings.NewReplacer(
	"č", "c", "ć", "c", "ž", "z", "š", "s", "đ", "dj", "á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "ă", "a", "ą", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "ě", "e", "ę", "e", "í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o",
	"ö", "o", "õ", "o", "ő", "o", "ø", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ű", "u", "ů", "u", "ñ", "n", "ń", "n",
	"ň", "n", "ý", "y", "ç", "c", "ł", "l", "ľ", "l", "ś", "s", "ź", "z", "ż", "z", "ř", "r", "ť", "t", "ď", "d", "ß", "ss",
	"ạ", "a", "ấ", "a", "ầ", "a", "ậ", "a", "ẩ", "a", "ắ", "a", "ằ", "a", "ặ", "a", "ế", "e", "ề", "e", "ệ", "e", "ể", "e",
	"ị", "i", "ọ", "o", "ố", "o", "ồ", "o", "ộ", "o", "ổ", "o", "ơ", "o", "ớ", "o", "ờ", "o", "ợ", "o", "ụ", "u", "ư", "u",
	"ứ", "u", "ừ", "u", "ự", "u", "ỳ", "y", "ō", "o", "ū", "u", "ā", "a", "ī", "i",
	"&", " and ",
)

// String folds s.
func String(s string) string {
	s = replacer.Replace(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case !space && b.Len() > 0:
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// Name folds a person's name and drops dots between initials, so
// "J.R.R. Tolkien", "J. R. R. Tolkien" and "JRR Tolkien" compare equal.
func Name(s string) string {
	f := strings.Fields(String(strings.ReplaceAll(s, ".", " ")))
	var out []string
	initials := ""
	for _, w := range f {
		if len(w) == 1 {
			initials += w
			continue
		}
		if initials != "" {
			out = append(out, initials)
			initials = ""
		}
		out = append(out, w)
	}
	if initials != "" {
		out = append(out, initials)
	}
	return strings.Join(out, " ")
}

// LastName is the folded last word of a name.
func LastName(s string) string {
	f := strings.Fields(String(s))
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}
