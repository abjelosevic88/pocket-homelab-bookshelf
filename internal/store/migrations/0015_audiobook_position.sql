-- Where the person is in an audiobook (Pocket Audiobooks, seconds), kept up
-- to date by every sync, so a book being listened to shows its progress.
ALTER TABLE audiobook_links ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audiobook_links ADD COLUMN duration INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audiobook_links ADD COLUMN listened_on TEXT NOT NULL DEFAULT '';
