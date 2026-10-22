<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="">
</p>
<h1 align="center">Pocket Bookshelf</h1>
<p align="center">
  A small self-hosted reading log: what you read, what you thought of it, the lines worth keeping, and what to read next.<br>
  <b>One binary · one SQLite file · one volume.</b> Book details and covers from Open Library and Google Books.
</p>
<p align="center">
  <a href="https://github.com/abjelosevic88/pocket-homelab-bookshelf/actions"><img src="https://github.com/abjelosevic88/pocket-homelab-bookshelf/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://github.com/abjelosevic88/pocket-homelab-bookshelf/pkgs/container/pocket-homelab-bookshelf"><img src="https://img.shields.io/badge/ghcr.io-amd64%20%7C%20arm64%20%7C%20armv7-blue" alt="image"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="MIT"></a>
</p>

---

Goodreads is a social network, and StoryGraph is too. This is just your notebook, built around four things:

- **Add a book in seconds.** Type a title, author or ISBN. The details and cover come from Open Library (no key) and Google Books (free key), and the cover is saved with your data. Pick *Want to read*, *Reading* or *Read*, and you're done.
- **Say what you thought.** Half-star ratings from ½ to 5, your thoughts in your own words, favourites, tags. When you finish a book, the app asks for a rating and a few words while it's still fresh.
- **Keep the good lines.** Save quotes with a page number and a note, or import every Kindle highlight in one go. A random one comes back to you on the home page.
- **Get to know the authors.** Every author gets a page with a short biography and photo, filled in automatically: how many of their books you read, your average rating, and the rest of their bibliography to plan from.
- **Plan what's next.** Want-to-read books form a queue you can reorder, pencil in for a month and start with one tap. A yearly goal tells you whether you're ahead or behind.

<p align="center"><img src="docs/screenshots/now.png" width="900" alt="Reading now: yearly goal, books in progress with page tracking, up next, recently finished, a random quote"></p>

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/library.png" alt="Library as a cover grid with status and ratings"></td>
    <td width="50%"><img src="docs/screenshots/book.png" alt="A book's page: rating, thoughts, quotes and reading history"></td>
  </tr>
  <tr>
    <td align="center"><sub>The library: covers, ratings, status, filters and search.</sub></td>
    <td align="center"><sub>A book: half-star rating, your thoughts, quotes, every time you read it.</sub></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/screenshots/add-search.png" alt="Adding a book: search results from Open Library and Google Books"></td>
    <td width="50%"><img src="docs/screenshots/add-pick.png" alt="Picking a result: details, description, status, subjects as tag suggestions"></td>
  </tr>
  <tr>
    <td align="center"><sub>Search by title, author or ISBN…</sub></td>
    <td align="center"><sub>…pick it and say where it stands. Subjects become one-tap tags.</sub></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/screenshots/authors.png" alt="Authors with photos, years and how many of their books you read"></td>
    <td width="50%"><img src="docs/screenshots/author.png" alt="An author page: biography, your stats, your books and their other books"></td>
  </tr>
  <tr>
    <td align="center"><sub>Authors, filled in from Open Library and Wikipedia.</sub></td>
    <td align="center"><sub>An author: bio, what you read, everything else they wrote.</sub></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/screenshots/planner.png" alt="Reading plan: up next queue and books planned per month"></td>
    <td width="50%"><img src="docs/screenshots/stats.png" alt="Stats: books per month, ratings, authors, tags"></td>
  </tr>
  <tr>
    <td align="center"><sub>The plan: a queue you can reorder, plus months.</sub></td>
    <td align="center"><sub>Stats per year: pace, ratings, authors, tags, formats.</sub></td>
  </tr>
</table>

<p align="center">
  <img src="docs/screenshots/mobile-now.png" width="260" alt="Reading now on a phone">&nbsp;&nbsp;
  <img src="docs/screenshots/quotes-dark.png" width="520" alt="Quotes in dark mode"><br>
  <sub>Installable PWA: a drawer and an add button on phones, light and dark themes that follow the system.</sub>
</p>

## Features

| Area | What you get |
|---|---|
| **Books** | Title, subtitle, authors, pages, year, publisher, language, ISBN-10/13, description, format (paper / e-book / audio), tags, cover. Statuses: want to read, reading, read, abandoned. |
| **Your verdict** | Half-star rating (0.5–5), free-text thoughts (start from your own templates, e.g. fiction / non-fiction sections; editable under Settings → Templates), favourite flag. A *Finished* dialog asks for the date, rating and a few words in one go. |
| **Reading history** | Every time through a book, with start and finish dates. Re-reads count again in that year, and abandoned reads are kept separately. Past reads can be added later. |
| **Progress** | Current page with a percentage for books in progress, updated inline from the home page. |
| **Quotes** | Text, page or location, your note. Searchable, with a random one on the home page and the Quotes page. |
| **Plan** | Want-to-read queue: reorder, plan for a month (overdue months are flagged), note why you want to read it or who recommended it, start reading with one tap. Yearly goal with pace (“3 behind schedule”). |
| **Authors** | Created from your books automatically, then filled in from Open Library in the background: short biography (Wikipedia summary when linked), birth/death, photo. Per author: books read, reading, planned, average rating, pages; your books; their full bibliography (English titles where an English edition exists) with one-tap *Want* / *Read it*. Find and follow new authors, edit name (renames it on every book, merges duplicates), bio, dates and photo, re-link to the right Open Library author. |
| **Characters** | Every novel's characters with a short, spoiler-free note on each, in each of your preferred languages (switch on the book page). Written in the background with your OpenAI key (Settings → AI), or without one by `scripts/characters.py` using `claude -p` / `codex exec`. See [Characters](#characters). |
| **Stats** | Per year: books, pages, average rating, books per month, rating distribution, top authors and tags, formats, longest book, the finished list. All-time totals. |
| **Metadata** | Open Library and Google Books, searched in parallel, deduplicated by ISBN. ISBN queries pin the exact edition. Covers are downloaded and stored locally, and any book can re-fetch details and cover later. See [docs/metadata.md](docs/metadata.md). |
| **Import** | Goodreads and StoryGraph CSV exports (ratings, reviews, dates, shelves); missing covers are fetched afterwards by ISBN. **Kindle highlights** from `My Clippings.txt`: matched to your books (also across translations, via Open Library), notes attached, re-imports only add what is new. |
| **People** | Multi-user, and every account has its own private shelf. Admins manage users. Login modes: local accounts, proxy header (Authelia / Authentik / Tinyauth) or none. |
| **Automation** | REST API with personal tokens: `GET /api/v1/dashboard` for Homepage/Glance widgets, `POST /api/v1/books/{id}/progress` from an iOS Shortcut, `GET /api/v1/quotes?random=1` for a quote anywhere. Prometheus `/metrics`, `/healthz`, `/readyz`. With **Pocket Core** on the box: registers itself, publishes `bookshelf.started` / `bookshelf.finished` / `bookshelf.abandoned` / `bookshelf.rated`, accepts paired-phone tokens, and offers **Sign in with Pocket Core** on the login page with no configuration (the app receives its OpenID Connect client when it registers). |
| **Ops-friendly** | Single static binary with an embedded web app, SQLite with WAL, `VACUUM INTO` backups, JSON and CSV export, multi-arch image (~15 MB), runs as non-root, no outbound calls except catalogue lookups (and OpenAI, if you give it a key). |

## Quick start

```bash
mkdir pocket-bookshelf && cd pocket-bookshelf
curl -O https://raw.githubusercontent.com/abjelosevic88/pocket-homelab-bookshelf/main/docker-compose.yml
docker compose up -d
# open http://localhost:8080 and create your account
```

Or with plain Docker:

```bash
docker run -d --name pocket-bookshelf -p 8080:8080 -v ./data:/data -e TZ=Europe/Berlin \
  ghcr.io/abjelosevic88/pocket-homelab-bookshelf:latest
```

Want to look around first? Add `-e DEMO_DATA=true -e AUTH_MODE=none` to get a sample shelf with no login.

Binaries for Linux, macOS and Windows are attached to every [release](https://github.com/abjelosevic88/pocket-homelab-bookshelf/releases), and a systemd unit is in [deploy/systemd](deploy/systemd).

## Configuration

Everything is an environment variable with a sensible default. The important ones:

| Variable | Default | Notes |
|---|---|---|
| `TZ` | `UTC` | Decides what “today” is for finish dates and the current year. Set it. |
| `BASE_URL` | – | Public URL (informational). |
| `DATA_DIR` | `/data` | Database, covers and backups live here. |
| `AUTH_MODE` | `local` | `local` (built-in login, multi-user), `proxy` (trust `Remote-User` from your SSO proxy; users auto-created), `none` (single implicit user with no login, for a trusted LAN only). |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | – | Create the first admin without the setup screen. |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS. |
| `GOOGLE_BOOKS_API_KEY` | – | Enables Google Books for the owner (everyone can also set their own in Settings → Book search). |
| `DEMO_DATA` | `false` | Seed a sample shelf into an empty database. |

The full list is in [docs/configuration.md](docs/configuration.md), and reverse-proxy snippets are in [docs/reverse-proxy.md](docs/reverse-proxy.md).

## Automation

Create a token under **Settings → API tokens**, then:

```bash
# where am I in the book? (iOS Shortcut, n8n, …)
curl -X POST https://bookshelf.example.com/api/v1/books/12/progress \
  -H "Authorization: Bearer pb_…" -H "Content-Type: application/json" -d '{"page": 142}'

# finished it
curl -X POST https://bookshelf.example.com/api/v1/books/12/status \
  -H "Authorization: Bearer pb_…" -H "Content-Type: application/json" \
  -d '{"status": "read", "rating": 4.5, "review": "Slow start, then it takes over."}'

# compact status for a Homepage / Glance widget
curl -H "Authorization: Bearer pb_…" https://bookshelf.example.com/api/v1/dashboard
# → {"reading":2,"read_this_year":14,"yearly_goal":24,"goal_percent":58,"want":9,…}
```

Everything else, including a Homepage `customapi` widget, is in [docs/api.md](docs/api.md).

**App links (Pocket Core):** another Pocket app the person allowed in Core can act for them here with a five-minute `pcx_` token. Bookshelf offers `books:read` ("See your books": `GET /books`, `GET /books/{id}`); `GET /auth/me` answers any linked app. The token acts as that person and sees only their shelf; every other route answers 403. Pocket Day uses it for the day's reading. The link can be switched off in Core → **Links** (box-wide) or under **Settings → Apps acting for me** (per person).

## Reader sync (KOReader, CrossPoint)

Bookshelf is a KOSync server, the protocol behind KOReader's "Progress sync" (sync.koreader.rocks).
Pick a sync username and password under **Settings → Reader sync**, then point a device at
`BASE_URL/kosync` (or at a dedicated port, `KOSYNC_PORT`, that serves nothing else and can be
published on its own):

- **KOReader**: Tools → Progress sync → Custom sync server, then *Login* (registration is closed).
- **CrossPoint**: KOReader Sync settings; with "send metadata" on, books match by title.
- **CrossPoint Sync**: Connections → *Another KOSync server* mirrors progress here.

Devices know a book by its file hash. A file is matched to a book on the shelf by the title and
authors the device sends (else its file name); the rest are linked once in Settings. Progress on a
linked book moves its current page and starts it when it was on the want-to-read list.

## Characters

The book page lists each novel's characters with a one- or two-sentence note on each — only what you learn
in the first chapters, so nothing is spoiled — in every preferred language (Settings → Book search). Non-fiction
gets none.

**With an OpenAI key** (Settings → AI — each person their own; `OPENAI_API_KEY` counts for the owner) the app writes the characters of that person's books itself, in the background:
right after a book is added, whenever the refresh button on a book page asks for a rewrite, and every 30 minutes for anything
still missing. Pick the model there (the list comes from your account; default `gpt-5-mini`) and whether it
may search the web — more accurate for translations and lesser-known books. Answers come back as strict,
schema-checked JSON. A bad key or an empty quota pauses writing until the settings change; a book that fails
otherwise is retried after two hours. `OPENAI_BASE_URL` points it at another server that speaks the
Responses API.

**Without a key**, `scripts/characters.py` can write them with a command-line model you're already signed in
to — Claude Code (`claude -p`) or the Codex CLI (`codex exec`) — so there's no API key at all. Run it from cron
or a systemd timer next to the app; it stands aside while the app has its own key:

```bash
BOOKSHELF_URL=http://localhost:8171 BOOKSHELF_TOKEN=pb_… ENGINE=claude MODEL=sonnet \
  python3 scripts/characters.py          # writes what is missing; exits fast when nothing is
```

A token covers the books of the person it belongs to (admins too); each person runs it with their own token. The API is in [docs/api.md](docs/api.md#characters).

## Backups

The database is a single file, and covers sit next to it in `covers/`.

- **UI:** Settings → Backup & data → *Download database* (a consistent `VACUUM INTO` copy).
- **CLI:** `docker exec pocket-bookshelf pocket-bookshelf backup` writes to `/data/backups/`.
- **API:** `GET /api/v1/backup`.
- **Export:** JSON (complete and re-importable) or CSV (one row per book).

Restic / Borg / Backrest users: back up `DATA_DIR`, but snapshot the live `pocket-bookshelf.db` through one of the methods above rather than copying the WAL files raw. See [docs/backup.md](docs/backup.md).

## Development

```bash
make dev      # Go API on :8080 (demo data, no login) + Vite on :5173
make test     # go vet + go test + tsc
make all      # production build: web UI embedded into ./pocket-bookshelf
```

Requires Go 1.22+ and Node 20+. Stack: Go + chi + modernc SQLite (no CGO), and React 19 + Vite + TypeScript with no UI framework. Planned next steps are in [docs/roadmap.md](docs/roadmap.md).

## Credits

Book data and covers come from [Open Library](https://openlibrary.org/) (Internet Archive) and [Google Books](https://books.google.com/). Sibling projects built the same way are [Pocket Habits](https://github.com/abjelosevic88/pocket-homelab-habit-tracker), [Pocket Inventory](https://github.com/abjelosevic88/pocket-homelab-inventory), [Pocket Invoicing](https://github.com/abjelosevic88/pocket-homelab-invoicing), [Pocket Split](https://github.com/abjelosevic88/pocket-homelab-cost-split) and [Pocket Subtitles](https://github.com/abjelosevic88/pocket-homelab-subtitles).

## License

MIT

## Choosing the AI provider

In Settings, text generation defaults to **Automatic — prefer Pocket AI**.
If Pocket Core reports an enabled link from this app to Pocket AI, generation
uses Pocket AI's Claude account and model for the signed-in person. Otherwise,
automatic mode uses that person's saved OpenAI-compatible configuration. You
can explicitly select **Pocket AI** or **OpenAI**. Standalone installs need no
Pocket Core or Pocket AI and continue to use their own API keys.

Upgrade Pocket AI and this app together. On existing Core installations, approve
the new **Generate text** (`inference:write`) link under **Core → Links**; new
permissions are not automatically granted by an app update. The person's
**Apps acting for me** switch must also be enabled. Existing opt-outs are
preserved. Every generation request obtains a fresh, person-scoped token.

Core outages and Pocket AI generation failures are reported without silently
retrying against OpenAI. Pocket AI runs a text-only, temporary Claude session
without tools, filesystem access, MCP servers or saved conversation history.
The model and Claude account come from the person's Pocket AI settings.

The background character writer uses the same provider choice. Web search and
the OpenAI model selector apply only to OpenAI; Pocket AI uses its existing
knowledge and can report unknown books.

Pocket AI model selection: AI settings offer the models discovered from your connected Claude account. “Use Pocket AI default” follows your current model in Pocket AI (Sonnet when no model is set). An explicit choice is saved per person in this app and never changes the Pocket AI default or your OpenAI model. Loading models does not create a chat or send a generation prompt. If a saved model is no longer offered, choose another model or return to the default; generation does not silently substitute a different model.
