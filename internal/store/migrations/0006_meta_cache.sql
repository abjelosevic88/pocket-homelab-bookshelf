-- Answers from Wikidata and Open Library that pages are built from (series
-- members, covers), kept across restarts so pages open instantly; refreshed
-- in the background when old.
CREATE TABLE meta_cache (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    fetched_at INTEGER NOT NULL -- unix seconds
);
