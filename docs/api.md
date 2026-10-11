# API

Base path `/api/v1`. JSON in, JSON out. Authenticate with the session cookie (browser) or a personal token:

```
Authorization: Bearer pb_xxxxxxxx…
```

Create tokens under **Settings → API tokens**. A token acts as the user who created it and sees only their shelf. Dates are `YYYY-MM-DD`, months `YYYY-MM`, ratings `0.5`–`5` in half steps (`0` = not rated).

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
| `GET` · `PUT` | `/settings` | `{title, yearly_goal, landing, search_source, google_books_key}` (+ read-only `sources`, `google_books_env`) |
| `GET` | `/settings/system` | Version, paths, sizes, counts. |
| `GET` · `POST` · `PUT` · `DELETE` | `/users`, `/users/{id}` | Listing for everyone; changes admin only. |
| `GET` · `POST` · `DELETE` | `/tokens` | Personal tokens. The plaintext is returned once on create. |
| `GET` | `/backup` | Consistent SQLite copy. |
| `GET` | `/export.json` · `/export.csv` | Your shelf: complete / one row per book. |
| `POST` | `/import` | The JSON export. Books already on the shelf are skipped. |
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
