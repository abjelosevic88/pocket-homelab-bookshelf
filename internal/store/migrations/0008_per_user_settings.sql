-- Preferences and keys belong to a person, not to the box. Books, authors and
-- daily sets already carry user_id; reads, quotes, book_authors and
-- book_characters belong to a book and every query reaches them through
-- books.user_id. What was still shared lived in settings: the yearly goal,
-- landing page, search source, preferred languages, thought templates, the
-- Google Books key and the OpenAI settings. They move to user_settings,
-- owned by the account that set them up (the first user, the owner); members
-- start from the defaults and bring their own keys. settings keeps only what
-- is about the box itself (title, setup_complete).
CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

INSERT OR IGNORE INTO user_settings (user_id, key, value)
  SELECT (SELECT MIN(id) FROM users), key, value FROM settings
  WHERE key IN ('yearly_goal', 'landing', 'search_source', 'languages', 'thought_templates',
                'google_books_key', 'ai_openai_key', 'ai_model', 'ai_web_search')
    AND EXISTS (SELECT 1 FROM users);

DELETE FROM settings WHERE key IN ('yearly_goal', 'landing', 'search_source', 'languages', 'thought_templates',
                                   'google_books_key', 'ai_openai_key', 'ai_model', 'ai_web_search');

-- Child rows are looked up by their book; these make the ownership joins cheap.
CREATE INDEX IF NOT EXISTS books_user ON books(user_id, id);
CREATE INDEX IF NOT EXISTS authors_user ON authors(user_id);
