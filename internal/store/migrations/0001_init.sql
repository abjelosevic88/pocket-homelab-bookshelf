CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'admin',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- One row per book on someone's shelf. Every user has their own shelf: the
-- book, where it stands (status), what they thought of it (rating, review)
-- and, while it waits, its place in the reading plan.
--   status:  want | reading | read | abandoned
--   rating:  0 = not rated, otherwise 0.5 … 5 in half stars
--   queue_pos / planned_for / plan_note only matter while status = want.
CREATE TABLE books (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title        TEXT NOT NULL,
  subtitle     TEXT NOT NULL DEFAULT '',
  authors      TEXT NOT NULL DEFAULT '',  -- "Ursula K. Le Guin, Someone Else"
  isbn13       TEXT NOT NULL DEFAULT '',
  isbn10       TEXT NOT NULL DEFAULT '',
  pages        INTEGER NOT NULL DEFAULT 0,
  published    TEXT NOT NULL DEFAULT '',  -- year, or whatever the source gave
  publisher    TEXT NOT NULL DEFAULT '',
  language     TEXT NOT NULL DEFAULT '',
  description  TEXT NOT NULL DEFAULT '',
  format       TEXT NOT NULL DEFAULT '',  -- paper | ebook | audio
  tags         TEXT NOT NULL DEFAULT '',
  cover        TEXT NOT NULL DEFAULT '',  -- file name in COVER_DIR
  source       TEXT NOT NULL DEFAULT '',  -- openlibrary | google | manual | import
  source_id    TEXT NOT NULL DEFAULT '',
  source_url   TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'want',
  rating       REAL NOT NULL DEFAULT 0,
  review       TEXT NOT NULL DEFAULT '',
  favorite     INTEGER NOT NULL DEFAULT 0,
  current_page INTEGER NOT NULL DEFAULT 0,
  queue_pos    INTEGER NOT NULL DEFAULT 0,
  planned_for  TEXT NOT NULL DEFAULT '',  -- YYYY-MM, '' = up next
  plan_note    TEXT NOT NULL DEFAULT '',  -- why / who recommended it
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX books_user_status ON books(user_id, status);
CREATE INDEX books_isbn13 ON books(isbn13);

-- Each time through a book. A re-read is a second row, so it counts again in
-- that year's stats. finished_on = '' while the read is still open.
CREATE TABLE reads (
  id          INTEGER PRIMARY KEY,
  book_id     INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  started_on  TEXT NOT NULL DEFAULT '',
  finished_on TEXT NOT NULL DEFAULT '',
  abandoned   INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX reads_book ON reads(book_id);
CREATE INDEX reads_finished ON reads(finished_on);

-- Passages worth keeping.
CREATE TABLE quotes (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  text       TEXT NOT NULL,
  page       TEXT NOT NULL DEFAULT '',  -- "142", "ch. 3", "47%"
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX quotes_book ON quotes(book_id);
