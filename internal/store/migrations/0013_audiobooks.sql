-- Audiobooks from Pocket Audiobooks (through Pocket Core, listening:read):
-- which book on the person's shelves an audiobook is, and which finishes and
-- starts were already taken in, so a sync never adds them twice and a book the
-- person removed is never added again.
CREATE TABLE IF NOT EXISTS audiobook_links (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id    TEXT NOT NULL,
  book_id    INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, item_id)
);
CREATE TABLE IF NOT EXISTS audiobook_events (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id TEXT NOT NULL,
  kind    TEXT NOT NULL,  -- finished | started
  day     TEXT NOT NULL,
  PRIMARY KEY (user_id, item_id, kind, day)
);
