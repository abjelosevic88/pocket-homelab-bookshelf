package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-bookshelf/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// "Ask about this book": every book can have Pocket AI chats of its own.
// Bookshelf starts them through Pocket Core (link chats:write on Pocket AI)
// with the book's details as context — where the reader is (so nothing past
// their page gets spoiled), their thoughts and quotes — and lists them on the
// book's page. The chat itself runs and is read in Pocket AI (embedded in the
// page). The browser never sees the link token.

var aiUse = core.Use{App: "ai", Scopes: []string{"chats:write"},
	Reason: "Ask about a book: start a Pocket AI chat about it with its details and your place in it, and list the book's chats on its page"}

var aiHTTP = &http.Client{Timeout: 20 * time.Second}

// bookChat is one chat about a book, as Pocket AI reports it.
type bookChat struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // idle | running | waiting | error
	Preview   string `json:"preview"`
	Model     string `json:"model"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// chatStarter is a ready question for one book: the label on the chip and
// the question with the book's details filled in.
type chatStarter struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// chatsBox is what the book page shows under "Ask about this book".
type chatsBox struct {
	Available bool          `json:"available"`
	Missing   bool          `json:"missing,omitempty"` // no Pocket AI (or Core) here: the page shows no box
	Reason    string        `json:"reason,omitempty"`
	Chats     []bookChat    `json:"chats"`
	Starters  []chatStarter `json:"starters"`
	Model     string        `json:"model"`
}

// aiClient reaches the person's Pocket AI with a fresh link token.
type aiClient struct{ base, token string }

func (c *aiClient) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := aiHTTP.Do(req)
	if err != nil {
		return errors.New("Pocket AI does not answer right now")
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error = strings.TrimSpace(e.Error); e.Error == "" {
			e.Error = fmt.Sprintf("Pocket AI answered HTTP %d", res.StatusCode)
		} else {
			e.Error = "Pocket AI: " + e.Error
		}
		return errors.New(e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// aiFor is the person's Pocket AI, or why it can't be reached (missing: there
// is nothing to switch on).
func (s *Server) aiFor(ctx context.Context, email string) (c *aiClient, missing bool, reason string) {
	if !s.core.Configured() || s.cfg.BaseURL == "" {
		return nil, true, "Chats about a book run in Pocket AI through Pocket Core, which this Bookshelf is not joined to."
	}
	lt, err := s.core.ActAs(ctx, email, "ai", "chats:write")
	if err != nil {
		var le *core.LinkError
		if !errors.As(err, &le) {
			return nil, false, "Pocket Core does not answer right now, so Pocket AI cannot be reached."
		}
		switch le.Reason {
		case "target_missing":
			return nil, true, "Pocket AI is not on this box."
		case "off_person":
			return nil, false, "Switched off for you: turn on “Pocket Bookshelf → Pocket AI” in Pocket Core, Settings → Apps acting for me."
		case "off_admin":
			return nil, false, "Switched off for this box: an admin turns on “Pocket Bookshelf → Pocket AI” in Pocket Core, Links."
		case "not_in_audience":
			return nil, false, "You have no Pocket AI: an admin can give you access in Pocket Core."
		case "not_offered", "not_declared":
			return nil, false, "This Pocket AI or Bookshelf is too old to link the two; update both."
		case "unknown_user":
			return nil, false, "Your account is not in Pocket Core."
		}
		return nil, false, "Pocket AI: " + le.Message
	}
	return &aiClient{base: strings.TrimRight(lt.URL, "/"), token: lt.AccessToken}, false, ""
}

// bookItemURL is the book's address here: Pocket AI keys a book's chats on it.
func (s *Server) bookItemURL(b *store.Book) string {
	return s.cfg.BaseURL + "/books/" + strconv.FormatInt(b.ID, 10)
}

// GET /api/v1/books/{id}/chats
func (s *Server) handleBookChats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	b, err := s.store.GetBook(ctx, me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "book chats")
		return
	}
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "sign in")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	box := chatsBox{Chats: []bookChat{}, Starters: startersFor(b, st.AskStarters), Model: st.AskModel}
	c, missing, reason := s.aiFor(ctx, u.Email)
	if c == nil {
		box.Missing, box.Reason = missing, reason
		writeJSON(w, http.StatusOK, box)
		return
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/link/chats?url="+url.QueryEscape(s.bookItemURL(b)), nil, &box.Chats); err != nil {
		box.Reason = err.Error()
		writeJSON(w, http.StatusOK, box)
		return
	}
	box.Available = true
	writeJSON(w, http.StatusOK, box)
}

// POST /api/v1/books/{id}/chats {text, name?, model?} → the new chat
func (s *Server) handleStartBookChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	b, err := s.store.GetBook(ctx, me(r), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "book chat")
		return
	}
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "sign in")
		return
	}
	var in struct {
		Text  string `json:"text"`
		Name  string `json:"name"` // the chat's name, e.g. the starter's label; else the question
		Model string `json:"model"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "write a question")
		return
	}
	if len(in.Text) > 20000 {
		writeErr(w, http.StatusBadRequest, "the question is too long")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	model := in.Model
	if model != "sonnet" && model != "opus" && model != "haiku" {
		model = st.AskModel
	}
	c, _, reason := s.aiFor(ctx, u.Email)
	if c == nil {
		writeErr(w, http.StatusConflict, reason)
		return
	}
	quotes, _ := s.store.ListQuotes(ctx, store.QuoteFilter{UserID: b.UserID, BookID: b.ID, Limit: 25})
	var chat bookChat
	err = c.do(ctx, http.MethodPost, "/api/v1/link/chats", map[string]any{
		"text": fillStarter(strings.TrimSpace(in.Text), b), "title": b.Title, "url": s.bookItemURL(b),
		"context": bookContext(b, quotes, s.bookItemURL(b)), "model": model, "name": clipText(strings.TrimSpace(in.Name), 100),
	}, &chat)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, chat)
}

// bookContext is what the agent learns about the book and the reader's place
// in it (≤ 16 KB, Pocket AI's limit).
func bookContext(b *store.Book, quotes []store.Quote, itemURL string) string {
	var sb strings.Builder
	line := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			fmt.Fprintf(&sb, "%s: %s\n", k, v)
		}
	}
	sb.WriteString("A book on the reader's shelf in Pocket Bookshelf. They are asking about it.\n\n")
	line("Title", b.Title)
	line("Subtitle", b.Subtitle)
	if b.OriginalTitle != "" && b.OriginalTitle != b.Title {
		line("Original title", b.OriginalTitle)
	}
	line("Author", b.Authors)
	if b.Series != "" {
		line("Series", strings.TrimSpace(b.Series+" #"+b.SeriesNo))
	}
	line("Published", b.Published)
	if b.Pages > 0 {
		line("Pages", strconv.Itoa(b.Pages))
	}
	line("Format", b.Format)
	line("Tags", b.Tags)
	line("Page in Bookshelf", itemURL)
	sb.WriteString("\n")
	switch b.Status {
	case "reading":
		where := "They are reading it now"
		if b.CurrentPage > 0 {
			where += fmt.Sprintf(", currently on page %d", b.CurrentPage)
			if b.Pages > 0 {
				where += fmt.Sprintf(" of %d (%d%%)", b.Pages, b.CurrentPage*100/b.Pages)
			}
		} else if b.ListenDuration > 0 && b.ListenPosition > 0 {
			where += fmt.Sprintf(" as an audiobook, %s into %s (%d%%)", hoursMinutes(b.ListenPosition), hoursMinutes(b.ListenDuration), b.ListenPosition*100/b.ListenDuration)
		}
		if b.StartedOn != "" {
			where += ", started " + b.StartedOn
		}
		sb.WriteString(where + ".\n")
		sb.WriteString("Spoilers: they have not finished it. Do not reveal plot, twists, deaths or the ending beyond where they are. If a question can't be answered without it, say so and ask before spoiling.\n")
	case "read":
		sb.WriteString("They have finished it")
		if b.FinishedOn != "" {
			sb.WriteString(" (" + b.FinishedOn + ")")
		}
		sb.WriteString(". Spoilers are fine.\n")
	case "abandoned":
		sb.WriteString("They stopped reading it before the end")
		if b.CurrentPage > 0 {
			fmt.Fprintf(&sb, ", around page %d", b.CurrentPage)
		} else if b.ListenDuration > 0 && b.ListenPosition > 0 {
			fmt.Fprintf(&sb, ", %d%% into the audiobook", b.ListenPosition*100/b.ListenDuration)
		}
		sb.WriteString(". Ask before spoiling what comes after.\n")
	default:
		sb.WriteString("They have not started it yet (it is on their want-to-read list). Do not spoil it.\n")
	}
	if b.Rating > 0 {
		fmt.Fprintf(&sb, "Their rating: %s / 5\n", strconv.FormatFloat(b.Rating, 'f', -1, 64))
	}
	if d := strings.TrimSpace(b.Description); d != "" {
		sb.WriteString("\nDescription:\n" + clipText(d, 2500) + "\n")
	}
	if rv := strings.TrimSpace(b.Review); rv != "" {
		sb.WriteString("\nTheir thoughts so far (their own words):\n" + clipText(rv, 6000) + "\n")
	}
	if len(quotes) > 0 {
		sb.WriteString("\nPassages they saved:\n")
		for _, q := range quotes {
			if sb.Len() > 15000 {
				break
			}
			t := "- “" + clipText(strings.Join(strings.Fields(q.Text), " "), 400) + "”"
			if q.Page != "" {
				t += " (p. " + q.Page + ")"
			}
			if q.Note != "" {
				t += " — their note: " + clipText(q.Note, 200)
			}
			sb.WriteString(t + "\n")
		}
	}
	sb.WriteString("\nTheir other books, quotes and reading history are reachable with the pocket_bookshelf tools.\n")
	return clipText(sb.String(), 16000)
}

// clipText shortens s to n runes, keeping its lines (clip in daily.go joins them).
func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// startersFor are the person's starters that suit where the book stands, filled in.
func startersFor(b *store.Book, all []store.AskStarter) []chatStarter {
	out := []chatStarter{}
	for _, st := range all {
		if st.When != "" && st.When != "any" && !strings.Contains(","+st.When+",", ","+b.Status+",") {
			continue
		}
		out = append(out, chatStarter{Label: st.Label, Text: fillStarter(st.Text, b)})
	}
	return out
}

// fillStarter puts the book's details in a question: {title} {author} {page} {pages} {percent}.
func fillStarter(text string, b *store.Book) string {
	page, pct := "where I am", ""
	if b.CurrentPage > 0 {
		page = strconv.Itoa(b.CurrentPage)
		if b.Pages > 0 {
			pct = strconv.Itoa(b.CurrentPage*100/b.Pages) + "%"
		}
	} else if b.ListenDuration > 0 && b.ListenPosition > 0 {
		// an audiobook: "up to page {page} … beyond that page" becomes a point in time
		pct = strconv.Itoa(b.ListenPosition*100/b.ListenDuration) + "%"
		at := hoursMinutes(b.ListenPosition) + " of " + hoursMinutes(b.ListenDuration) + " into the audiobook (" + pct + ")"
		text = strings.NewReplacer("page {page}", at, "{page}", at, "that page", "that point").Replace(text)
	}
	pages := ""
	if b.Pages > 0 {
		pages = strconv.Itoa(b.Pages)
	}
	return strings.NewReplacer("{title}", b.Title, "{author}", b.Authors, "{page}", page, "{pages}", pages, "{percent}", pct).Replace(text)
}

// GET /api/v1/ai → {"url"}: Pocket AI's public address for the app's chat
// window ("" without Core or Pocket AI). Cached a minute.
func (s *Server) handleAIInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"url": s.aiPublicURL(r.Context())})
}

func (s *Server) aiPublicURL(ctx context.Context) string {
	if !s.core.Configured() {
		return ""
	}
	const key = "ai#public"
	s.coreMu.Lock()
	if r, ok := s.resolved[key]; ok && time.Since(r.at) < time.Minute {
		s.coreMu.Unlock()
		return r.url
	}
	s.coreMu.Unlock()
	u := ""
	if res, err := s.core.Resolve(ctx, "ai"); err == nil && res.Status != "missing" {
		u = strings.TrimRight(res.PublicURL, "/")
	}
	s.coreMu.Lock()
	s.resolved[key] = resolvedURL{url: u, at: time.Now()}
	s.coreMu.Unlock()
	return u
}

// hoursMinutes is a length of time for people: "20h 26m", "45m".
func hoursMinutes(seconds int) string {
	m := seconds / 60
	if m < 60 {
		return strconv.Itoa(m) + "m"
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}
