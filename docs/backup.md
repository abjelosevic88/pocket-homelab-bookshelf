# Backup & restore

Everything lives in `DATA_DIR` (`/data` in the container):

```
/data/pocket-bookshelf.db     # the SQLite database (WAL mode: -wal / -shm files appear while running)
/data/covers/                 # cover images, named <book id>-<random>.<ext>
/data/backups/                # VACUUM INTO copies made by the CLI
```

## Making a backup

- **UI:** Settings → Backup & data → *Download database*.
- **CLI:** `docker exec pocket-bookshelf pocket-bookshelf backup` (or `backup -o /data/backups/named.db`).
- **API:** `GET /api/v1/backup` with a token.
- **Portable:** `GET /api/v1/export.json` — your shelf as plain JSON (books, thoughts, reads, quotes), re-importable on another instance. Covers are not included; *Find missing covers* fetches them again by ISBN, or copy `covers/`.

All of these produce a consistent snapshot even while the app is writing. Copying `pocket-bookshelf.db` raw while the container runs can give you a torn file because of the WAL.

### With restic / Borg / Backrest / Kopia

Add `DATA_DIR` to your backup set (this covers `covers/`), **exclude** `pocket-bookshelf.db*`, and run the CLI backup (or the sqlite3 `.backup` command) as a pre-snapshot hook so the consistent copy under `backups/` is what gets picked up. Covers are re-downloadable, so you can exclude `covers/` too if size matters (a cover is 30–150 KB).

## Restoring

```bash
docker compose stop pocket-bookshelf
cp pocket-bookshelf-20261011-030000.db data/pocket-bookshelf.db
rm -f data/pocket-bookshelf.db-wal data/pocket-bookshelf.db-shm
docker compose start pocket-bookshelf
```

Or start a fresh instance and use Settings → Backup & data → *Import JSON*.
