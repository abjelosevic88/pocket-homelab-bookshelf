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
| `CORE_URL` / `CORE_REGISTRATION_KEY` | – | [Pocket Core](https://git.bjelke.org/abjelosevic88/pocket-homelab-core): register on start (the key is needed once; the minted app token lives in `$DATA_DIR/core.token`), publish `bookshelf.started` / `bookshelf.finished` / `bookshelf.abandoned` / `bookshelf.rated`, and accept the tokens Core hands to paired phones. |
| `CORE_APP_URL` | `BASE_URL` | How Core reaches this app for health checks (e.g. `http://host.docker.internal:8171`). |
| `DEMO_DATA` | `false` | Seed a sample shelf into an *empty* database. |

## Book metadata

| Variable | Default | Description |
|---|---|---|
| `GOOGLE_BOOKS_API_KEY` | – | Enables Google Books as a second catalogue for the owner (first account). A key saved under Settings → Book search takes precedence; other people save their own. |
| `OPENAI_API_KEY` | – | Lets the app write the characters of the owner's books itself. A key saved under Settings → AI takes precedence; other people save their own. |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Another server speaking the OpenAI Responses API. |
| `GOOGLE_BOOKS_COUNTRY` | `US` | Country sent to Google Books with every request (ISO code, e.g. `BA`). Google needs one and fails with *503 backendFailed* when it cannot place the server's IP address. |
| `METADATA_TIMEOUT` | `12s` | Timeout for catalogue requests. |

See [metadata.md](metadata.md) for what each catalogue gives you and how covers are fetched.

## Authentication & users

| Variable | Default | Description |
|---|---|---|
| `AUTH_MODE` | `local` | `local`: built-in email + password, first-run setup screen, users managed under Settings → People. `proxy`: trust the user name from a header set by your SSO proxy; users are auto-created on first visit (first one becomes admin). `none`: no login at all, one implicit user — only for trusted networks. |
| `AUTH_PROXY_HEADER` | `Remote-User` | Header carrying the user name in `proxy` mode. |
| `AUTH_PROXY_EMAIL_HEADER` | `Remote-Email` | Header carrying the email in `proxy` mode. |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | In `local` mode, create this admin on first start instead of showing the setup screen. Password must be 8+ characters. |
| `SESSION_TTL` | `720h` | Browser session lifetime. |
| `SECURE_COOKIES` | `false` | Mark the session cookie `Secure`. Turn on behind HTTPS. |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (PocketID, Authelia, Authentik, Keycloak…). Needs `BASE_URL`; the callback is `BASE_URL/api/v1/auth/oidc/callback`. On a box with Pocket Core none of these are needed — the app receives its client when it registers. |
| `OIDC_NAME` | `single sign-on` | Button label: "Sign in with …" (the Core-issued client is labelled "Pocket Core"). |
| `OIDC_AUTO_CREATE` | `true` | Give an unknown (verified) email a member account on first sign-in instead of refusing it. Always on for the Core-issued client. |

Every user has **their own shelf** — books, ratings, thoughts, quotes, reading history and plan are private to each account. Settings are personal too — yearly goal, start page, languages, templates, daily highlights, and the Google Books and OpenAI keys — and admins get no access to anyone else's shelf. Roles: **admin** manages the accounts, the app title and can download the whole database (`/backup`); **member** has their shelf. Keys set through the environment (`GOOGLE_BOOKS_API_KEY`, `OPENAI_API_KEY`) belong to the owner (the first account); others bring their own. API tokens (`Authorization: Bearer …` or `X-API-Key`) work in every mode and act as the user who created them.

### Single sign-on (OIDC)

In `local` mode a configured provider adds a "Sign in with …" button above the password form; the password form stays. Pocket Bookshelf uses the authorization-code flow with PKCE and the client secret, checks the ID token's RS256 signature against the provider's published keys (plus issuer, audience, expiry and nonce) and matches the person to a user by **verified** email. With Pocket Core on the box this is zero configuration: the app asks for a relying-party client when it registers and uses the one Core issues (kept in `$DATA_DIR/core.token.oidc.json`); explicit `OIDC_*` variables take precedence when set. Without Core and without `OIDC_*` nothing changes: no button, and `/api/v1/auth/oidc/*` answers 404.


## In-app settings

- **Settings → General:** app title, yearly reading goal (books), which page opens first.
- **Settings → Book search:** default search source (All / Open Library / Google Books) and the Google Books API key.

## Forgot your password?

```bash
docker exec pocket-bookshelf pocket-bookshelf reset-password            # first user, random password printed
docker exec pocket-bookshelf pocket-bookshelf reset-password -email me@example.com -password NewPass123
```
