package store

import (
	"context"
	"encoding/json"
	"strconv"
)

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Bookshelf"

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := Settings{Title: DefaultTitle, Landing: "now", SearchSource: "all", Languages: "en", ThoughtTemplates: DefaultThoughtTemplates}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "yearly_goal":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				st.YearlyGoal = n
			}
		case "landing":
			if v == "now" || v == "library" || v == "planner" {
				st.Landing = v
			}
		case "search_source":
			if v == "all" || v == "openlibrary" || v == "google" {
				st.SearchSource = v
			}
		case "google_books_key":
			st.GoogleBooksKey = v
		case "languages":
			st.Languages = v
		case "thought_templates":
			var t []ThoughtTemplate
			if json.Unmarshal([]byte(v), &t) == nil {
				st.ThoughtTemplates = t
			}
		case "setup_complete":
			st.SetupComplete = v == "1"
		}
	}
	return st, rows.Err()
}

// SaveSettings persists all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	tpl, _ := json.Marshal(st.ThoughtTemplates)
	kv := map[string]string{
		"thought_templates": string(tpl),
		"title":             st.Title,
		"yearly_goal":       strconv.Itoa(st.YearlyGoal),
		"landing":           st.Landing,
		"search_source":     st.SearchSource,
		"google_books_key":  st.GoogleBooksKey,
		"languages":         st.Languages,
		"setup_complete":    map[bool]string{true: "1", false: "0"}[st.SetupComplete],
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// AISettings configure writing characters with OpenAI.
type AISettings struct {
	OpenAIKey string
	Model     string
	WebSearch bool
}

// GetAISettings loads the AI settings (web search on by default).
func (s *Store) GetAISettings(ctx context.Context) (AISettings, error) {
	a := AISettings{WebSearch: true}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings WHERE key IN ('ai_openai_key', 'ai_model', 'ai_web_search')`)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return a, err
		}
		switch k {
		case "ai_openai_key":
			a.OpenAIKey = v
		case "ai_model":
			a.Model = v
		case "ai_web_search":
			a.WebSearch = v != "0"
		}
	}
	return a, rows.Err()
}

// SaveAISettings persists the AI settings.
func (s *Store) SaveAISettings(ctx context.Context, a AISettings) error {
	kv := map[string]string{"ai_openai_key": a.OpenAIKey, "ai_model": a.Model, "ai_web_search": map[bool]string{true: "1", false: "0"}[a.WebSearch]}
	for k, v := range kv {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return nil
}
