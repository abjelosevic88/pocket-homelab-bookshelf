-- Reading progress from e-readers over the KOReader sync protocol (KOSync):
-- KOReader, CrossPoint and CrossPoint Sync's "Another KOSync server" mirror.
--
-- A person picks a sync username and password in Settings → Reader sync. The
-- devices send the username and the MD5 of the password on every request;
-- only a bcrypt of that MD5 is kept.
CREATE TABLE IF NOT EXISTS kosync_accounts (
  user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  username   TEXT NOT NULL UNIQUE COLLATE NOCASE,
  key_hash   TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_seen  TEXT NOT NULL DEFAULT ''
);

-- One row per document (the KOReader file hash) a person reads: the latest
-- position from any device, what the device said the book is, and the book on
-- the shelf it was matched or linked to.
CREATE TABLE IF NOT EXISTS kosync_documents (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  document   TEXT NOT NULL,
  progress   TEXT NOT NULL DEFAULT '',
  percentage REAL NOT NULL DEFAULT 0,
  device     TEXT NOT NULL DEFAULT '',
  device_id  TEXT NOT NULL DEFAULT '',
  timestamp  INTEGER NOT NULL DEFAULT 0,
  title      TEXT NOT NULL DEFAULT '',
  authors    TEXT NOT NULL DEFAULT '',
  filename   TEXT NOT NULL DEFAULT '',
  book_id    INTEGER REFERENCES books(id) ON DELETE SET NULL,
  ignored    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, document)
);
CREATE INDEX IF NOT EXISTS kosync_documents_book ON kosync_documents(book_id);
