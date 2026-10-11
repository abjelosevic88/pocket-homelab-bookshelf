-- Series: name and number of a book in it ("0" for a prequel, "2.5" for a
-- novella between 2 and 3, "" when not known), the Wikidata item of the
-- series (to list its other books), and whether detection has run.
ALTER TABLE books ADD COLUMN series TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_no TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_qid TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_checked INTEGER NOT NULL DEFAULT 0;
CREATE INDEX books_series ON books(user_id, series);
