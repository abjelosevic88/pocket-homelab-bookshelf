-- Characters of a book, one row per language: a short, spoiler-free note on
-- each. Written by a language model through the API (see scripts/characters.py)
-- or by hand. status: ok | none (no characters — non-fiction) | unknown (the
-- writer did not know the book well enough to name them reliably).
CREATE TABLE book_characters (
    book_id    INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    lang       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'ok',
    characters TEXT NOT NULL DEFAULT '[]', -- JSON [{name, role, description}]
    source     TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (book_id, lang)
);
