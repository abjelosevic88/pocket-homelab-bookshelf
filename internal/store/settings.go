package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
)

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Bookshelf"

// Box-wide settings live in settings (title, setup_complete); everything a
// person chooses lives in user_settings under their id (see 0008).

// GetSettings loads the box settings plus the preferences of userID, with
// defaults applied. userID 0 gives the box settings with default preferences.
func (s *Store) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	st := Settings{Title: DefaultTitle, Landing: "now", SearchSource: "all", Languages: "en", ThoughtTemplates: DefaultThoughtTemplates}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings WHERE key IN ('title', 'setup_complete')
		UNION ALL SELECT key, value FROM user_settings WHERE user_id = ?`, userID)
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

// SaveSettings stores the personal preferences of userID (not the box title).
func (s *Store) SaveSettings(ctx context.Context, userID int64, st Settings) error {
	tpl, _ := json.Marshal(st.ThoughtTemplates)
	return s.putUserSettings(ctx, userID, map[string]string{
		"thought_templates": string(tpl),
		"yearly_goal":       strconv.Itoa(st.YearlyGoal),
		"landing":           st.Landing,
		"search_source":     st.SearchSource,
		"google_books_key":  st.GoogleBooksKey,
		"languages":         st.Languages,
	})
}

// SaveBoxSettings stores the settings of the box itself: title and setup state.
func (s *Store) SaveBoxSettings(ctx context.Context, st Settings) error {
	kv := map[string]string{"title": st.Title, "setup_complete": map[bool]string{true: "1", false: "0"}[st.SetupComplete]}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) putUserSettings(ctx context.Context, userID int64, kv map[string]string) error {
	if userID <= 0 {
		return ErrNotFound
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?)
			ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value`, userID, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OwnerID is the account the box was set up with (the first user): keys
// given to the whole box through the environment are theirs.
func (s *Store) OwnerID(ctx context.Context) int64 {
	var id sql.NullInt64
	_ = s.DB.QueryRowContext(ctx, `SELECT MIN(id) FROM users`).Scan(&id)
	return id.Int64
}

// UsersWithSetting lists the users who saved a non-empty value for key.
func (s *Store) UsersWithSetting(ctx context.Context, key string) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM user_settings WHERE key = ? AND value <> '' ORDER BY user_id`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AISettings configure writing characters with OpenAI; each person has their own.
type AISettings struct {
	AIProvider string `json:"ai_provider"`
	OpenAIKey  string
	Model      string
	WebSearch  bool
}

// GetAISettings loads the AI settings of userID (web search on by default).
func (s *Store) GetAISettings(ctx context.Context, userID int64) (AISettings, error) {
	a := AISettings{WebSearch: true}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM user_settings WHERE user_id = ? AND key IN ('ai_provider', 'ai_openai_key', 'ai_model', 'ai_web_search')`, userID)
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
		case "ai_provider":
			a.AIProvider = v
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

// SaveAISettings stores the AI settings of userID.
func (s *Store) SaveAISettings(ctx context.Context, userID int64, a AISettings) error {
	return s.putUserSettings(ctx, userID, map[string]string{"ai_provider": a.AIProvider, "ai_openai_key": a.OpenAIKey, "ai_model": a.Model, "ai_web_search": map[bool]string{true: "1", false: "0"}[a.WebSearch]})
}
