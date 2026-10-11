-- The longer biography (Wikipedia introduction plus life sections) behind "Read more".
ALTER TABLE authors ADD COLUMN bio_full TEXT NOT NULL DEFAULT '';
-- Look Wikipedia-backed authors up again to fetch it (hand-edited ones keep their text).
UPDATE authors SET enriched_at = '' WHERE bio_source LIKE '%wikipedia.org%' AND edited = 0;
