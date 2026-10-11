# Roadmap

Pocket Bookshelf is meant to stay small: a reading log, your thoughts, quotes and a plan. Ideas that fit, roughly in order:

## Next

- **Pocket Homelab companion app section**: reading now with page updates, finish with rating, the plan, a quote of the day. The API already covers it.
- **Barcode scan** in the add-book search (phone camera → ISBN), using the browser's `BarcodeDetector` where available.
- **Series**: an optional series name + number, so "read next in series" can feed the plan.

## Homelab integrations (opt-in)

- **Audiobookshelf**: import finished audiobooks (and progress) as reads with format *audio* — ABS already knows what you listened to and when.
- **Kavita / calibre-web**: offer "add to shelf" for ebooks you own; mark progress from Kavita's reading position.
- **Hardcover** (hardcover.app, free GraphQL API with a personal token) as a third catalogue, and optional two-way sync of ratings.
- **Webhook / Apprise** on finish ("finished X ★★★★½") for a journal or ntfy.

## Deliberately not planned

Social features, recommendations engines, reading challenges with badges, or a five-level shelf hierarchy. Tags and the plan cover organisation.
