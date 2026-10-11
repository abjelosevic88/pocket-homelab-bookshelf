# Book metadata: where the details come from

Typing a whole book in by hand is the fastest way to stop logging books, so adding one starts with a search. The server asks free public catalogues and fills in title, authors, pages, year, publisher, ISBN, description and cover. You only pick the right result and say where the book stands.

All lookups happen **on the server**, so the browser never talks to Google or Open Library. Covers are downloaded once and stored in `DATA_DIR/covers`, which means browsing your shelf makes no outside requests and keeps working if a catalogue goes away.

## The catalogues

| | Open Library | Google Books |
|---|---|---|
| Key | none | free API key |
| Strengths | older books, translations and non-English editions; community-edited; generous cover archive | recent editions, reliable page counts and descriptions |
| Used for | search (`/search.json`), the edition behind an ISBN (`/isbn/{isbn}.json`), work descriptions (`/works/{id}.json`), covers (`covers.openlibrary.org`) | search and volume details (`/books/v1/volumes`) |
| Limits | asks for an identifying User-Agent (sent); cover lookups *by ISBN* are limited to ~100 per 5 minutes per IP | 1,000 requests a day on the free key |

With **All** (the default) both are searched in parallel and the results interleaved; a book both know (same ISBN-13) is shown once, with missing fields borrowed from the other. If one catalogue fails, you get the other's results and a warning.

**Preferred languages** (Settings → Metadata, up to three, in order): for each one Open Library is also asked which works have an edition in that language. Those works come first, shown as that edition (its title, cover and ISBN, with the original title underneath). Works with no edition in your languages follow in catalogue order. The same applies to authors' bibliographies, and Google Books results are re-ordered by language. Open Library's Serbian, Croatian and Bosnian coverage is thin, so a second language like English is a useful fallback.

**ISBN searches** (`9780441478125`, `978-0-441-47812-5`, `ISBN 0441478123`) are detected and pinned to that edition: its page count, publisher and cover.

### Getting a Google Books key

Google refuses anonymous requests (the shared quota is zero), so Google Books needs a key:

1. Open the [Google Cloud console](https://console.cloud.google.com/), create or pick a project.
2. *APIs & Services → Library → Books API → Enable*.
3. *APIs & Services → Credentials → Create credentials → API key*. Optionally restrict it to the Books API.
4. Paste it under **Settings → Metadata** (or set `GOOGLE_BOOKS_API_KEY`) and press *Test Google Books*.

## Covers

- Picking a search result downloads its cover with the book.
- **Edit details → Find details & cover** searches again for an existing book and replaces details and cover.
- **Upload cover…** takes any JPEG/PNG/WebP/GIF, and you can paste an image URL from anywhere.
- After an import, **Settings → Backup & data → Find missing covers** looks up every cover-less book with an ISBN on Open Library, one every 3 seconds to respect its rate limit.

Downloads only go to public addresses: the server refuses to fetch from loopback, private (RFC 1918), link-local or CGNAT/Tailscale (100.64/10) addresses, also after redirects, so a pasted URL cannot be used to reach your LAN.

## Importing a reading history

**Settings → Backup & data → Import Goodreads / StoryGraph CSV** reads:

- a **Goodreads** export (*My Books → Import and export → Export library*): rating, review, date read, date added, exclusive shelf (read / currently-reading / to-read), other shelves as tags, binding as format, ISBNs;
- a **StoryGraph** export (*Manage account → Export StoryGraph library*): star rating (rounded to half stars), review, read status including did-not-finish, last date read, tags, format;
- this app's own **export.csv**.

Books already on your shelf (same ISBN, or same title and author) are skipped, so importing twice is harmless.

## Importing Kindle highlights

**Quotes → Import from Kindle…** (also under Settings → Backup & data) reads the `My Clippings.txt` file every Kindle keeps in its `documents` folder (connect it with a USB cable).

1. **Parse.** Highlights, notes and bookmarks are read with their page, location and date (English, German, French, Spanish, Italian and Portuguese Kindle menus). Bookmarks and "clipping limit" placeholders are dropped. When you extend a highlight, the Kindle appends a new entry; only the last, longest version is kept. A note is attached to the highlight it was written on, and a note without a highlight becomes a quote of its own.
2. **Match.** Each Kindle book is matched to your shelf by title (ignoring subtitles, series, accents and case) and author. If that fails, the title is looked up in Open Library and compared with the Open Library work each of your books is linked to, so the English *The Power of Now* on the Kindle finds your *Moć sadašnjeg trenutka*. Books not on your shelf are offered as new books, filled in from Open Library.
3. **Review.** Every Kindle book shows where its highlights will go. You can pick another book, create a new one (as read, reading or want to read) or skip it.
4. **Import.** Highlights become quotes with their page (or `loc. 1201` when the book has no page numbers), note and original date. Running the same file again later only adds what is new.

## Authors

Every name in a book's authors becomes an author entry, linked to the book. A background worker then looks each new author up, one per second:

1. **Find the Open Library author.** First through the book's own Open Library work, which names its authors exactly. Open Library has duplicate, near-empty author records, so when a name search finds the same person with far more works, that record wins. If the books aren't linked to Open Library, the name search alone decides (exact or alternate name match, most prolific first).
2. **Biography.** The English Wikipedia summary when Open Library links the author to Wikidata, plus a **full biography** behind *Read more*: the article's introduction and its life sections (early life, career, exile, death, personal life …), without works lists, legacy, themes or references, up to about 20,000 characters, with a link to continue on Wikipedia. Wikipedia text is CC BY-SA and credited on the page. Otherwise the Wikipedia page with exactly that name, used only if it describes a writer, psychologist, philosopher or similar (so a footballer of the same name is ignored). Open Library's own biography is the last fallback, cleaned of markdown and of a second language.
3. **Photo.** Open Library's author photo, or the Wikipedia image. It's downloaded like covers.

The **bibliography** on an author page is Open Library's list of their works, most published first. Study guides, summaries, collected-works volumes, anthologies and barely catalogued records are filtered out. English titles are shown when an English edition exists, with the original underneath. Works already on your shelf are recognised by their Open Library link, or by title.

Author pages are editable. A changed name is written into every book; renaming onto an existing author merges the two. A hand-edited biography or dates is kept when you refresh. **Wrong person?** links a different Open Library author.
