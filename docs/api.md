# API

Base path `/api/v1`. JSON in, JSON out. Authenticate with the session cookie (browser) or a personal token:

```
Authorization: Bearer pb_xxxxxxxx…
```

Create tokens under **Settings → API tokens**. A token acts as the user who created it and sees only their shelf. Dates are `YYYY-MM-DD`, months `YYYY-MM`, ratings `0.5`–`5` in half steps (`0` = not rated).

### Pocket Core app links

With Pocket Core on the box, another Pocket app can act for a person with a five-minute `pcx_` token Core issues (`Authorization: Bearer pcx_…`). The token resolves to that person's account and sees only their shelf, and only these routes are open to it:

| Scope | Name | Routes |
|---|---|---|
| (any link) | | `GET /auth/me` |
| `books:read` | See your books | `GET /books` (all its filters), `GET /books/{id}` |

Anything else answers `403` ("this Pocket Core link does not cover …"). Links are switched on and off in Core → **Links** (box-wide) and **Settings → Apps acting for me** (per person).

## Books

| Method | Path | Notes |
|---|---|---|
| `GET` | `/books` | Your shelf. Filters: `status=want\|reading\|read\|abandoned`, `q=` (title, author, tags, review, ISBN), `tag=`, `author=`, `year=` (finished that year), `favorite=1`, `isbn=`, `sort=recent\|added\|title\|author\|rating\|finished\|queue`, `limit=`. |
| `POST` | `/books` | Add a book (body below). `409 {"book_id"}` if it is already on the shelf, unless `"allow_duplicate": true`. Returns `{"book", "warning"?}`. |
| `GET` | `/books/{id}` | `{"book", "reads": […], "quotes": […]}` |
| `PUT` | `/books/{id}` | **Partial**: send only what changes, e.g. `{"rating": 4.5}` or `{"review": "…"}`. A `"cover_url"` downloads a new cover. A changed `"status"` is applied like the status call with today's date. |
| `DELETE` | `/books/{id}` | Book, cover, reads and quotes. |
| `POST` | `/books/{id}/status` | `{"status": "reading"}` · `{"status": "read", "date": "2026-10-11", "rating": 4.5, "review": "…"}` · `{"status": "abandoned"}` · `{"status": "want"}`. Keeps the reading history in step (see below). |
| `POST` | `/books/{id}/progress` | `{"page": 142}` or `{"percent": 40}` (needs a page count). Starts a want-to-read book. |
| `GET` | `/books/{id}/covers` | Covers to choose from: `{"covers": [{url, thumb, source, label, language}]}` — every edition of the book on Open Library (+ Google Books). Set one with `POST /books/{id}/cover {"url"}`. |
| `GET` · `POST` · `DELETE` | `/books/{id}/cover` | Image / set (`multipart` field `cover`, or `{"url": "https://…"}`) / remove. |
| `POST` | `/books/{id}/reads` | Record a past read: `{"started_on", "finished_on", "abandoned"}` (finish date required). |
| `PUT` · `DELETE` | `/reads/{id}` | Edit / remove a read. |
| `PUT` | `/planner/order` | `{"ids": [8, 3, 5]}` — the want-to-read order. |
| `GET` | `/tags` | `[{"name", "count"}]`, most used first. |

### Book body

```json
{
  "title": "The Left Hand of Darkness", "subtitle": "",
  "authors": "Ursula K. Le Guin",          // comma separated
  "isbn13": "9780441478125", "isbn10": "", "pages": 304, "published": "1969",
  "publisher": "Ace", "language": "eng", "description": "…",
  "format": "paper",                       // paper | ebook | audio | ""
  "tags": "sci-fi, classics",
  "status": "read",                        // want (default) | reading | read | abandoned
  "rating": 5, "review": "Slow start, then…", "favorite": true,
  "current_page": 0,
  "planned_for": "2026-11", "plan_note": "Recommended by Marko",
  "source": "openlibrary", "source_id": "/works/OL59800W", "source_url": "https://openlibrary.org/works/OL59800W",
  "cover_url": "https://covers.openlibrary.org/b/id/10618463-L.jpg",   // create/update only: downloaded
  "started_on": "2026-03-03", "finished_on": "2026-04-20"             // create only: dates of the read
}
```

### How status and reads fit together

A **read** is one time through a book; re-reading adds another. The status call keeps them in step:

| To | What happens |
|---|---|
| `reading` | Opens a read starting today (or `date`), unless one is already open. Re-reading a finished book resets the page. |
| `read` | Closes the open read on `date`, or records a read that finished on `date`. Page jumps to the end. |
| `abandoned` | Same as `read`, marked abandoned (not counted as read in stats). |
| `want` | Drops an open, unfinished read and puts the book at the end of the plan. Finished reads stay. |

Stats count finished, not-abandoned reads by their finish date. A book marked read without a date (e.g. an import) counts all-time but in no year.

## Quotes

| Method | Path | Notes |
|---|---|---|
| `GET` | `/quotes` | Newest first. `q=`, `book=<id>`, `random=1` (one at random), `limit=`. Each quote carries `book_title`, `book_authors`, `book_cover`. |
| `POST` | `/books/{id}/quotes` | `{"text", "page", "note"}` |
| `PUT` · `DELETE` | `/quotes/{id}` | |

## Authors

Authors are created from the books' `authors` strings and looked up on Open Library in the background.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/authors?q=&sort=read\|name\|rating\|recent\|added` | With shelf stats: `books, read, reading, want, avg_rating, pages, last_read`. `enriched_at` is `""` until the lookup ran. |
| `POST` | `/authors` | Follow an author: `{"name"}` and/or `{"key": "OL31353A"}`. Looked up immediately. |
| `GET` | `/authors/search?q=` | Open Library author search; results already on your list carry `author_id`. |
| `GET` | `/authors/resolve?name=` | `{"id"}` of the author with that name. |
| `GET` | `/authors/{id}` | `{"author", "books": […]}`. `bio` is the short biography, `bio_full` the long one (`## Section` / `### Subsection` lines) or `""`. |
| `PUT` | `/authors/{id}` | Partial: `name` (renames it in every book; renaming onto an existing author merges), `bio`, `birth_date`, `death_date`. Edited fields survive refreshes. |
| `DELETE` | `/authors/{id}` | Only without books on the shelf (409 otherwise). |
| `GET` | `/authors/{id}/works` | Bibliography from Open Library, most published first; `on_shelf` = your book id; `original_title` when `title` is an English edition's. |
| `POST` | `/authors/{id}/refresh` | Look up again; `{"key": "OL…A"}` links a different Open Library author. |
| `GET` | `/authors/{id}/photos` | Pictures to choose from: `{"photos": [{url, thumb, source, label}]}` — Open Library photos, the Wikidata portrait, images from the Wikipedia article, other Open Library records with the name. Set one with `POST /authors/{id}/photo {"url"}`. |
| `GET` · `POST` · `DELETE` | `/authors/{id}/photo` | Image / set (`multipart` field `photo`, or `{"url"}`) / remove. |

## Characters

One set per book and language: `{"status": "ok" | "none" | "unknown", "characters": [{"name", "role": "main" | "supporting", "description"}], "source", "updated_at"}`. `none` = no characters (non-fiction), `unknown` = the writer could not name them reliably. Written by `scripts/characters.py` (see the README).

| Method | Path | Notes |
|---|---|---|
| `GET` | `/books/{id}/characters` | `{"langs": ["en", "sr"], "sets": {"en": {…}, "sr": {…}}}` — `langs` are the preferred languages. |
| `PUT` | `/books/{id}/characters` | `{"source": "claude sonnet", "sets": {"en": {"status": "ok", "characters": […]}}}` — replaces the given languages. Up to 40 characters. Only your own books (admins included); another user's book is a 404. |
| `DELETE` | `/books/{id}/characters` | Forget them, so they are written again. |
| `GET` | `/characters/status` | `{"writer": "app" \| "external", "running", "paused", "waiting", "written", "failed", "last_error", "last_run"}` — `app` when the app has an OpenAI key. |

### Summary

A short spoiler-free "what it is about" paragraph per book and preferred language, written in the background by the person's AI (Pocket AI or their OpenAI key) after the characters, and shown first under *About the book*. `{"status": "ok" | "unknown", "text", "source", "updated_at"}`.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/books/{id}/summary` | `{"langs": ["en", "sr"], "summaries": {"en": {…}}, "writer": true}` — `writer` is false without an AI. |
| `DELETE` | `/books/{id}/summary` | Forget it, so it is written again. |
| `GET` | `/books/{id}/covers/ai` | Covers found with the person's AI, each checked by downloading it: `{"covers": [{url, thumb, source, label}], "hints": {original_title, original_authors, isbns, publisher, year, cover_urls}}`. Up to ~2 minutes. |
| `POST` | `/characters/run` | Write what is missing now (also un-pauses and retries failed books). |
| `GET` · `PUT` | `/settings/ai` | `{"key_set", "key_hint", "key_env", "model", "default_model", "web_search"}`. Your own key: everyone has their own. PUT: `{"key": "sk-…"` (omit to keep)`, "clear_key": true, "model", "web_search"}`. `key_env` is true only for the owner (first account), whose fallback `OPENAI_API_KEY` is. |
| `POST` | `/settings/ai/test` | `{"key"}` (or your saved key) → `{"models": [...]}` the key can use. |
| `GET` | `/characters/missing?limit=20` | Books missing a preferred language: `{"langs", "books": [{id, title, subtitle, work_title, authors, published, series, series_no, description, source_url, missing: ["sr"]}]}`. Only the token owner's books. `"writer": "app"` means the app writes them itself with that person's key (external writers should stand aside). |

## Search the catalogues

| Method | Path | Notes |
|---|---|---|
| `GET` | `/search?q=&source=all\|openlibrary\|google&limit=` | `{"results": [candidate…], "warnings": [], "sources": ["openlibrary", "google"]}`. A candidate already on your shelf has `"on_shelf": <book id>`. ISBN queries are detected. |
| `GET` | `/search/details?source=&id=` | Full record of one result (description, subjects, bigger cover). |
| `POST` · `GET` | `/covers/fetch-missing` | Start / poll the background lookup of covers by ISBN: `{"running", "done", "found", "total"}`. |

## Summary

| Method | Path | Notes |
|---|---|---|
| `GET` | `/dashboard` | `{reading, read_this_year, pages_this_year, yearly_goal, goal_percent, want, read_total, books, quotes, avg_rating, currently_reading: [{id, title, authors, progress, date}], last_finished, up_next}` |
| `GET` | `/stats?year=` | Per-year summary: totals, books/pages per month, rating distribution, top authors and tags, formats, finished list, longest book, all-time totals. |

## Account, settings, data

| Method | Path | Notes |
|---|---|---|
| `GET` | `/auth/status` | `{needs_setup, auth_mode, authenticated, user, version, title}` |
| `POST` | `/auth/setup` · `/auth/login` · `/auth/logout` | |
| `GET` · `PUT` | `/auth/me` | Own profile, `{"name","email","current_password","new_password"}` |
| `GET` · `PUT` | `/settings` | `{title, yearly_goal, landing, search_source, google_books_key, languages: "en,sr", thought_templates: [{name, body}]}` (+ read-only `sources`, `google_books_env`). `title` is the box's and only admins change it; everything else is the caller's own. `google_books_env` (the `GOOGLE_BOOKS_API_KEY` fallback) applies to the owner only. |
| `GET` | `/settings/system` | Version, paths, sizes, and the counts of your own shelf (`users` too for admins). |
| `GET` · `POST` · `PUT` · `DELETE` | `/users`, `/users/{id}` | Admin only; a member's listing holds just their own account. |
| `GET` · `POST` · `DELETE` | `/tokens` | Personal tokens. The plaintext is returned once on create. |
| `GET` | `/backup` | Consistent SQLite copy of the whole box — admin only. Everyone can export their own shelf below. |
| `GET` | `/export.json` · `/export.csv` | Your shelf: complete / one row per book. |
| `POST` | `/import` | The JSON export. Books already on the shelf are skipped. |
| `POST` | `/import/kindle/preview` | Kindle `My Clippings.txt` (body or `multipart` field `file`) → `{"clippings", "books": [{title, author, entries: [{text, note, page, loc_start, loc_end, added, note_only}], highlights, notes, known, match: {book_id, title, by: "title"\|"openlibrary"}, candidate}]}`. Nothing is saved. |
| `POST` | `/import/kindle` | `{"books": [{"target": "book", "book_id": 17, "entries": […]}, {"target": "new", "status": "read", "title", "author", "candidate", "entries": […]}, {"target": "skip"}]}` → `{books_created, books_updated, added, updated, skipped}`. Idempotent: known highlights are skipped, longer versions replace shorter ones, a "new" book already on the shelf is reused. |
| `POST` | `/import/goodreads` | Goodreads or StoryGraph export, or this app's CSV — body or `multipart` field `file`. |

Unauthenticated: `/healthz`, `/readyz`, `/metrics` (Prometheus; disable with `METRICS_ENABLED=false` or block at the proxy).

## Homepage widget

```yaml
- Pocket Bookshelf:
    href: https://bookshelf.example.com/
    icon: mdi-bookshelf
    widget:
      type: customapi
      url: http://pocket-bookshelf:8080/api/v1/dashboard
      refreshInterval: 600000
      headers:
        Authorization: Bearer {{HOMEPAGE_VAR_POCKET_BOOKSHELF_TOKEN}}
      mappings:
        - { field: reading, label: Reading, format: text }
        - { field: read_this_year, label: This year, format: text }
        - { field: want, label: Planned, format: text }
        - { field: avg_rating, label: Avg ★, format: float }
```

## iOS Shortcut: log where you are

1. *Ask for Input* (number, "Page?") → 2. *Get Contents of URL* `POST …/books/{id}/progress` with JSON `{"page": [Provided Input]}` and the bearer header. Pin one per book you are reading, or fetch `GET /books?status=reading` first and *Choose from List*.
