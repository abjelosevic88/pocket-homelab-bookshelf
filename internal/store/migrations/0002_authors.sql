-- Authors of the books on a user's shelf, plus authors added by hand (followed).
-- Books keep their display string in books.authors; book_authors links them.
-- name_key is the folded name ("j r r tolkien" → "jrr tolkien") for matching.
CREATE TABLE authors (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  name_key    TEXT NOT NULL,
  ol_key      TEXT NOT NULL DEFAULT '',  -- "OL31353A"
  bio         TEXT NOT NULL DEFAULT '',
  bio_source  TEXT NOT NULL DEFAULT '',  -- URL the biography came from
  birth_date  TEXT NOT NULL DEFAULT '',
  death_date  TEXT NOT NULL DEFAULT '',
  photo       TEXT NOT NULL DEFAULT '',  -- file name in COVER_DIR
  wikipedia   TEXT NOT NULL DEFAULT '',
  followed    INTEGER NOT NULL DEFAULT 0, -- added by hand: kept without books
  edited      INTEGER NOT NULL DEFAULT 0, -- edited by hand: refresh does not overwrite
  enriched_at TEXT NOT NULL DEFAULT '',  -- '' = not looked up yet
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, name_key)
);

CREATE TABLE book_authors (
  book_id   INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  author_id INTEGER NOT NULL REFERENCES authors(id) ON DELETE CASCADE,
  position  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (book_id, author_id)
);
CREATE INDEX book_authors_author ON book_authors(author_id);
