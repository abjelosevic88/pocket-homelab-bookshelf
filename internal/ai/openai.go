package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAI calls the OpenAI Responses API.
type OpenAI struct {
	Key       string
	Model     string
	WebSearch bool   // let the model look books up on the web
	BaseURL   string // default https://api.openai.com/v1
	HTTP      *http.Client
}

// DefaultModel is used when none is chosen.
const DefaultModel = "gpt-5-mini"

// APIError is an error answer from OpenAI.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("OpenAI HTTP %d: %s", e.Status, e.Message) }

// Fatal reports whether retrying cannot help until the settings change (bad key, no access, no money).
func (e *APIError) Fatal() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden ||
		e.Status == http.StatusNotFound || strings.Contains(e.Message, "insufficient_quota") || strings.Contains(e.Message, "exceeded your current quota")
}

func (o *OpenAI) base() string {
	if o.BaseURL != "" {
		return strings.TrimRight(o.BaseURL, "/")
	}
	return "https://api.openai.com/v1"
}

func (o *OpenAI) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (o *OpenAI) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.base()+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+o.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := o.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		msg := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
			if e.Error.Code != "" {
				msg += " (" + e.Error.Code + ")"
			}
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return &APIError{Status: res.StatusCode, Message: msg}
	}
	return json.Unmarshal(raw, out)
}

// Models lists the chat models the key can use, newest first.
func (o *OpenAI) Models(ctx context.Context) ([]string, error) {
	var r struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := o.do(ctx, http.MethodGet, "/models", nil, &r); err != nil {
		return nil, err
	}
	sort.Slice(r.Data, func(i, j int) bool { return r.Data[i].Created > r.Data[j].Created })
	var out []string
	for _, m := range r.Data {
		id := strings.ToLower(m.ID)
		chat := strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "chatgpt-") || (len(id) > 1 && id[0] == 'o' && id[1] >= '0' && id[1] <= '9')
		for _, skip := range []string{"audio", "realtime", "tts", "transcribe", "image", "embedding", "search", "instruct", "moderation", "codex", "computer", "deep-research"} {
			if strings.Contains(id, skip) {
				chat = false
			}
		}
		if chat {
			out = append(out, m.ID)
		}
	}
	return out, nil
}

// Characters asks the model for a book's characters in langs.
func (o *OpenAI) Characters(ctx context.Context, b Book, langs []string) (*Result, error) {
	model := o.Model
	if model == "" {
		model = DefaultModel
	}
	body := map[string]any{
		"model": model,
		"input": Prompt(b, langs),
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "book_characters", "strict": true, "schema": Schema(langs),
		}},
	}
	// Web search: the current tool name, the older preview one, then none (models without it).
	tools := []string{""}
	if o.WebSearch {
		tools = []string{"web_search", "web_search_preview", ""}
	}
	var r responsesReply
	var err error
	for _, t := range tools {
		if t != "" {
			body["tools"] = []map[string]string{{"type": t}}
		} else {
			delete(body, "tools")
		}
		err = o.do(ctx, http.MethodPost, "/responses", body, &r)
		var ae *APIError
		if err == nil || !errors.As(err, &ae) || ae.Status != http.StatusBadRequest || !strings.Contains(strings.ToLower(ae.Message), "tool") && !strings.Contains(strings.ToLower(ae.Message), "web_search") {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	text := r.text()
	if text == "" {
		reason := r.Status
		if r.Incomplete != nil && r.Incomplete.Reason != "" {
			reason = r.Incomplete.Reason
		}
		return nil, fmt.Errorf("OpenAI returned no answer (%s)", reason)
	}
	var res Result
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		return nil, fmt.Errorf("OpenAI answer is not valid JSON: %w", err)
	}
	return &res, nil
}

type responsesReply struct {
	Status     string `json:"status"`
	Incomplete *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
}

func (r responsesReply) text() string {
	var sb strings.Builder
	for _, o := range r.Output {
		if o.Type != "message" {
			continue
		}
		for _, c := range o.Content {
			if c.Type == "output_text" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
