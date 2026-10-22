-- A short, spoiler-free "what it is about" paragraph per book and language,
-- written by the person's AI (Pocket AI or OpenAI). Kept apart from the
-- catalogue description so it can be written again without touching it.
CREATE TABLE IF NOT EXISTS book_summaries (
  book_id    INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  lang       TEXT NOT NULL,
  status     TEXT NOT NULL,            -- ok | unknown (the AI could not tell which book it is)
  text       TEXT NOT NULL DEFAULT '',
  source     TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (book_id, lang)
);
