-- Daily highlights: favourites and hidden quotes, when each was last shown,
-- the set picked for each day, and per-user delivery settings.
ALTER TABLE quotes ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0;
ALTER TABLE quotes ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
ALTER TABLE quotes ADD COLUMN last_shown TEXT NOT NULL DEFAULT '';
ALTER TABLE quotes ADD COLUMN shown_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE daily_sets (
  id        INTEGER PRIMARY KEY,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date      TEXT NOT NULL,             -- YYYY-MM-DD, local time
  quote_ids TEXT NOT NULL DEFAULT '',  -- "12,7,300" in display order
  sent_at   TEXT NOT NULL DEFAULT '',
  UNIQUE (user_id, date)
);

CREATE TABLE daily_settings (
  user_id   INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  enabled   INTEGER NOT NULL DEFAULT 0,
  count     INTEGER NOT NULL DEFAULT 5,
  time      TEXT NOT NULL DEFAULT '07:30', -- HH:MM, local time
  channel   TEXT NOT NULL DEFAULT 'apprise', -- apprise | ntfy | webhook
  url       TEXT NOT NULL DEFAULT ''
);
