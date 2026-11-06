package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The iPhone app signs in through /app-signin: sign in, Allow, a one-time
// code to pocketbookshelf://, traded with the PKCE verifier for a token.
func TestAppSignin(t *testing.T) {
	h := newShelfHarness(t)
	h.cfg.BaseURL = h.ts.URL
	sid, err := h.st.CreateSession(context.Background(), h.ana.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(cookie bool) *http.Response {
		req, _ := http.NewRequest("GET", h.ts.URL+"/app-signin?"+url.Values{"challenge": {challenge}, "device": {"Ana's iPhone"}}.Encode(), nil)
		if cookie {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
		}
		res, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	allow := func(origin string) string {
		req, _ := http.NewRequest("POST", h.ts.URL+"/app-signin", strings.NewReader(url.Values{"challenge": {challenge}, "device": {"Ana's iPhone"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
		res, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther {
			return res.Status
		}
		return res.Header.Get("Location")
	}

	// signed out: to the sign-in page, coming back here after it
	res := get(false)
	if loc := res.Header.Get("Location"); res.StatusCode != 302 || !strings.HasPrefix(loc, "/login?next=%2Fapp-signin%3F") {
		t.Fatalf("signed out: %d %s", res.StatusCode, loc)
	}
	// signed in: the question, naming the phone and the person
	res = get(true)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "Allow") || !strings.Contains(string(body), "Ana&#39;s iPhone") {
		t.Fatalf("question: %d %s", res.StatusCode, body)
	}
	// another site can't press Allow
	if got := allow("https://evil.example"); !strings.Contains(got, "403") {
		t.Fatalf("cross-site allow: %s", got)
	}
	code := func(loc string) string {
		u, err := url.Parse(loc)
		if err != nil || u.Scheme != "pocketbookshelf" || u.Query().Get("code") == "" {
			t.Fatalf("callback: %s", loc)
		}
		return u.Query().Get("code")
	}
	exchange := func(c, v string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", h.ts.URL+"/api/v1/auth/app/exchange", strings.NewReader(`{"code":"`+c+`","verifier":"`+v+`"}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	// the wrong verifier gets nothing, and the code is spent
	c1 := code(allow(h.ts.URL))
	if st, _ := exchange(c1, strings.Repeat("x", 50)); st != 401 {
		t.Fatalf("wrong verifier: %d", st)
	}
	if st, _ := exchange(c1, verifier); st != 401 {
		t.Fatalf("spent code: %d", st)
	}
	// the right one: a token for Ana, named after the phone
	st, out := exchange(code(allow(h.ts.URL)), verifier)
	tok, _ := out["token"].(string)
	if st != 200 || tok == "" || !strings.Contains(out["info"].(map[string]any)["name"].(string), "Ana's iPhone") {
		t.Fatalf("exchange: %d %v", st, out)
	}
	var me struct{ Email string }
	if code := h.json(tok, "GET", "/auth/me", nil, &me); code != 200 || me.Email != h.ana.Email {
		t.Fatalf("token: %d %+v", code, me)
	}
}
