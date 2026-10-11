# Configuration

All settings are environment variables. Nothing is required.

## Server

| Variable | Default | Description |
|---|---|---|
| `HOST` | `0.0.0.0` | Listen address. |
| `PORT` | `8080` | Listen port. |
| `BASE_URL` | – | Public URL (e.g. `https://bookshelf.example.com`). Informational. |
| `DATA_DIR` | `./data` (`/data` in the container) | Database, covers and backups. |
| `DB_PATH` | `$DATA_DIR/pocket-bookshelf.db` | Override the database file. |
| `COVER_DIR` | `$DATA_DIR/covers` | Where cover images are stored. |
| `MAX_COVER_MB` | `10` | Size limit for an uploaded or downloaded cover. |
| `TZ` | `UTC` | Timezone for "today" (finish dates, the year in stats). The container ships `tzdata`. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | `text` | `text` or `json`. |
| `TRUST_PROXY` | `true` | Honour `X-Forwarded-For` / `X-Real-IP` for logging. |
| `METRICS_ENABLED` | `true` | Serve Prometheus metrics on `/metrics` (unauthenticated — restrict at the proxy if exposed). |
| `DEMO_DATA` | `false` | Seed a sample shelf into an *empty* database. |

## Book metadata

| Variable | Default | Description |
|---|---|---|
| `GOOGLE_BOOKS_API_KEY` | – | Enables Google Books as a second catalogue. A key saved under Settings → Metadata takes precedence. |
| `GOOGLE_BOOKS_COUNTRY` | `US` | Country sent to Google Books with every request (ISO code, e.g. `BA`). Google needs one and fails with *503 backendFailed* when it cannot place the server's IP address. |
| `METADATA_TIMEOUT` | `12s` | Timeout for catalogue requests. |

See [metadata.md](metadata.md) for what each catalogue gives you and how covers are fetched.

## Authentication & users

| Variable | Default | Description |
|---|---|---|
| `AUTH_MODE` | `local` | `local`: built-in email + password, first-run setup screen, users managed under Settings → Users. `proxy`: trust the user name from a header set by your SSO proxy; users are auto-created on first visit (first one becomes admin). `none`: no login at all, one implicit user — only for trusted networks. |
| `AUTH_PROXY_HEADER` | `Remote-User` | Header carrying the user name in `proxy` mode. |
| `AUTH_PROXY_EMAIL_HEADER` | `Remote-Email` | Header carrying the email in `proxy` mode. |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | In `local` mode, create this admin on first start instead of showing the setup screen. Password must be 8+ characters. |
| `SESSION_TTL` | `720h` | Browser session lifetime. |
| `SECURE_COOKIES` | `false` | Mark the session cookie `Secure`. Turn on behind HTTPS. |

Every user has **their own shelf** — books, ratings, thoughts, quotes, reading history and plan are private to each account. Roles: **admin** manages users and the shared settings; **member** has their shelf. API tokens (`Authorization: Bearer …` or `X-API-Key`) work in every mode and act as the user who created them.

## In-app settings

- **Settings → General:** app title, yearly reading goal (books), which page opens first.
- **Settings → Metadata:** default search source (All / Open Library / Google Books) and the Google Books API key.

## Forgot your password?

```bash
docker exec pocket-bookshelf pocket-bookshelf reset-password            # first user, random password printed
docker exec pocket-bookshelf pocket-bookshelf reset-password -email me@example.com -password NewPass123
```
