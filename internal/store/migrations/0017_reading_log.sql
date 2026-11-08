-- The reading log beyond e-readers: every listening session from Pocket
-- Audiobooks (kept as it reports them, matched to a book through
-- audiobook_links) and every page the person logs by hand.
CREATE TABLE IF NOT EXISTS listening_sessions (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL,              -- Pocket Audiobooks' history id
  item_id    TEXT NOT NULL,
  day        TEXT NOT NULL,              -- local day it started
  started_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  seconds    INTEGER NOT NULL DEFAULT 0, -- time listened
  start_pos  INTEGER NOT NULL DEFAULT 0, -- seconds into the book
  end_pos    INTEGER NOT NULL DEFAULT 0,
  duration   INTEGER NOT NULL DEFAULT 0,
  device     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, session_id)
);
CREATE INDEX IF NOT EXISTS listening_sessions_item ON listening_sessions(user_id, item_id);
CREATE TABLE IF NOT EXISTS page_logs (
  id        INTEGER PRIMARY KEY,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id   INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  from_page INTEGER NOT NULL DEFAULT 0,
  page      INTEGER NOT NULL,
  pages     INTEGER NOT NULL DEFAULT 0,  -- the book's page count then
  at        INTEGER NOT NULL             -- unix seconds
);
CREATE INDEX IF NOT EXISTS page_logs_book ON page_logs(book_id, at);
