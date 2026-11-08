-- Reading positions that reach a book another way than this server's KOSync:
-- Pocket Library's reading feed (its own e-reader sync, its web reader). Kept
-- next to kosync_events so a book lists every update, whichever way it came.
CREATE TABLE IF NOT EXISTS progress_events (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id     INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  source      TEXT NOT NULL,              -- library
  device      TEXT NOT NULL DEFAULT '',
  percentage  REAL NOT NULL DEFAULT 0,
  timestamp   INTEGER NOT NULL,           -- when the reader was there (unix seconds)
  received_at TEXT NOT NULL,
  UNIQUE (user_id, book_id, source, timestamp)
);
CREATE INDEX IF NOT EXISTS progress_events_book ON progress_events(book_id, timestamp);
