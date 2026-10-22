-- Reading activity from CrossPoint Sync (GET /api/v1/stats/activity): per day
-- and document, how far the reader got. Replaced on every pull.
CREATE TABLE IF NOT EXISTS kosync_activity (
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day      TEXT NOT NULL,            -- YYYY-MM-DD, the reader's local day
  document TEXT NOT NULL,
  pages    INTEGER NOT NULL DEFAULT 0, -- print pages forward, as CrossPoint Sync counts them
  syncs    INTEGER NOT NULL DEFAULT 0,
  from_pct REAL NOT NULL DEFAULT 0,
  to_pct   REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day, document)
);

-- Asked once whether a book the e-reader has (almost) finished is finished.
ALTER TABLE kosync_documents ADD COLUMN finish_asked INTEGER NOT NULL DEFAULT 0;

-- The title of the original work, for translations ("The Fires of Heaven").
ALTER TABLE books ADD COLUMN original_title TEXT NOT NULL DEFAULT '';

-- Books whose series and original title the AI was already asked about.
CREATE TABLE IF NOT EXISTS book_ai_facts (
  book_id  INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
  asked_at TEXT NOT NULL
);
