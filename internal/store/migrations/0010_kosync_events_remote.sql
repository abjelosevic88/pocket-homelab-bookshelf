-- Every progress update as it arrived: the raw body a device (or the
-- CrossPoint Sync server) sent, kept as a reading history and so nothing a
-- device says is lost when the summary in kosync_documents moves on.
CREATE TABLE IF NOT EXISTS kosync_events (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  document    TEXT NOT NULL,
  source      TEXT NOT NULL,            -- device (a PUT here) | crosspoint (pulled)
  device      TEXT NOT NULL DEFAULT '',
  device_id   TEXT NOT NULL DEFAULT '',
  percentage  REAL NOT NULL DEFAULT 0,
  timestamp   INTEGER NOT NULL DEFAULT 0, -- when the reader was there (unix seconds)
  body        TEXT NOT NULL,            -- raw JSON
  received_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS kosync_events_doc ON kosync_events(user_id, document, id);

-- What CrossPoint Sync knows about a document: its cover and print page count.
ALTER TABLE kosync_documents ADD COLUMN cover_url TEXT NOT NULL DEFAULT '';
ALTER TABLE kosync_documents ADD COLUMN page_count INTEGER NOT NULL DEFAULT 0;

-- Per person: whether books read on a device that are not on the shelf are
-- added, and the CrossPoint Sync account titles, authors and covers are read
-- from (its mirror only forwards the file hash). remote_key is the MD5 of the
-- password, which is what that server checks.
CREATE TABLE IF NOT EXISTS kosync_prefs (
  user_id     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  add_missing INTEGER NOT NULL DEFAULT 1,
  remote_url  TEXT NOT NULL DEFAULT '',
  remote_user TEXT NOT NULL DEFAULT '',
  remote_key  TEXT NOT NULL DEFAULT '',
  last_pull   TEXT NOT NULL DEFAULT '',
  last_error  TEXT NOT NULL DEFAULT ''
);
