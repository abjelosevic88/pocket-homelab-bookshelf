#!/usr/bin/env python3
"""Write the characters of books on a Pocket Bookshelf.

Asks Pocket Bookshelf which books have no characters yet in the preferred
languages (Settings → Metadata), has a language model write a short,
spoiler-free note on each character — looking the book up on the web when it
needs to — and saves the result back through the API.

The model runs through a command-line tool you are already signed in to, so
no API key is needed:

  claude  Claude Code (`claude -p`, web search allowed)   — default
  codex   OpenAI Codex CLI (`codex exec`, web search on)

Environment:
  BOOKSHELF_URL     e.g. http://localhost:8171
  BOOKSHELF_TOKEN   an API token (Settings → API); an admin token covers every user's books
  ENGINE            claude | codex (default claude)
  MODEL             model for the engine (default: sonnet for claude, the CLI default for codex)
  CONCURRENCY       books written at once (default 2)
  LIMIT             books per run (default 30)

Usage:
  characters.py              write what is missing
  characters.py --book 42    write one book (even if it has characters)
  characters.py --dry-run    print the prompt for the first missing book

Python 3.9+, standard library only. Run it from cron or a systemd timer.
"""
import argparse
import concurrent.futures
import json
import os
import re
import subprocess
import sys
import tempfile
import urllib.request

URL = os.environ.get("BOOKSHELF_URL", "http://localhost:8080").rstrip("/")
TOKEN = os.environ.get("BOOKSHELF_TOKEN", "")
ENGINE = os.environ.get("ENGINE", "claude")
MODEL = os.environ.get("MODEL", "sonnet" if ENGINE == "claude" else "")
CONCURRENCY = int(os.environ.get("CONCURRENCY", "2"))
LIMIT = int(os.environ.get("LIMIT", "30"))
TIMEOUT = 600  # seconds per book

LANGS = {
    "en": "English",
    "sr": "Serbian in the Latin script (latinica). Use the character names as Serbian translations of the book spell them "
          "when that is well established (e.g. Frodo Bagins); otherwise keep the original spelling",
    "hr": "Croatian", "bs": "Bosnian", "sl": "Slovenian", "mk": "Macedonian", "de": "German", "fr": "French",
    "es": "Spanish", "it": "Italian", "pt": "Portuguese", "ru": "Russian", "hu": "Hungarian", "pl": "Polish",
    "cs": "Czech", "sk": "Slovak", "nl": "Dutch", "sv": "Swedish", "no": "Norwegian", "da": "Danish",
    "fi": "Finnish", "ro": "Romanian", "bg": "Bulgarian", "el": "Greek", "tr": "Turkish", "uk": "Ukrainian",
}

PROMPT = """You are writing the character list for a book in a personal reading log.

The book:
{facts}

First make sure which book this is — the shelf title may be a translation (often Serbian). If you are not
completely sure of the book or of its characters, look it up on the web (Wikipedia, the publisher, reputable
reference or fan wikis) before writing. Never invent characters or details.

List the book's characters: every main character and the important supporting ones — usually 5 to 15, up to
25 for large ensemble novels. For each, one or two sentences that help a reader keep track of who is who:
who they are, what they do, how they relate to the others.

Spoilers — the reader may not have finished the book. Describe each character only as the reader meets them
in the first chapters of THIS book, in the present tense: who they are and where they stand at that point.
- nothing that happens after their introduction: no events from the middle or end of the book, no deaths,
  fates, betrayals, twists, secret or hidden identities, later jobs, marriages or relationships;
- no "later", "eventually", "turns out", "is revealed", "ultimately", "comes to" (nor "kasnije", "na kraju",
  "ispostavi se" and the like in other languages);
- a character who only appears later in the book: say who they are, not what happens when they appear;
- in a later book of a series, what earlier books established is fine;
- when in doubt, leave it out.

If the book has no characters — non-fiction, history, biography, memoir, essays, self-help, science, poetry —
answer with kind "nonfiction". If you still cannot identify the book or its characters reliably after looking
it up, answer with kind "unknown".

Write the list in these languages, the same characters in the same order in each:
{langs}

Reply with only this JSON object, no other text and no code fences:
{{"kind": "fiction" | "nonfiction" | "unknown",
  "characters": {{{example}}}}}
"""


def api(method, path, body=None):
    req = urllib.request.Request(URL + "/api/v1" + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None,
                                 headers={"Authorization": "Bearer " + TOKEN, "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as r:
        raw = r.read()
    return json.loads(raw) if raw else None


def prompt_for(book, langs):
    facts = [f"- Title on the shelf: {book['title']}" + (f": {book['subtitle']}" if book.get("subtitle") else "")]
    if book.get("work_title"):
        facts.append(f"- Original title: {book['work_title']}")
    facts.append(f"- Author(s): {book.get('authors') or 'unknown'}")
    if book.get("published"):
        facts.append(f"- Published: {book['published']}")
    if book.get("series"):
        facts.append(f"- Series: {book['series']}" + (f", book {book['series_no']}" if book.get("series_no") else ""))
    if book.get("source_url"):
        facts.append(f"- Catalogue page: {book['source_url']}")
    if book.get("description"):
        facts.append(f"- Catalogue description: {book['description']}")
    lang_lines = "\n".join(f"- {l}: {LANGS.get(l, l)}" for l in langs)
    example = ", ".join(f'"{l}": [{{"name": "…", "role": "main" | "supporting", "description": "…"}}]' for l in langs)
    return PROMPT.format(facts="\n".join(facts), langs=lang_lines, example=example)


def run_engine(prompt):
    """Returns the model's reply text."""
    with tempfile.TemporaryDirectory(prefix="pb-characters-") as work:
        if ENGINE == "codex":
            out = os.path.join(work, "reply.txt")
            cmd = ["codex", "exec", "--skip-git-repo-check", "-s", "read-only", "-c", "tools.web_search=true", "-o", out]
            if MODEL:
                cmd += ["-m", MODEL]
            subprocess.run(cmd + [prompt], cwd=work, capture_output=True, text=True, timeout=TIMEOUT, check=True)
            with open(out) as f:
                return f.read()
        cmd = ["claude", "-p", "--output-format", "json", "--allowedTools", "WebSearch", "WebFetch"]
        if MODEL:
            cmd += ["--model", MODEL]
        # Run in an empty directory so no project instructions or memory are loaded.
        p = subprocess.run(cmd, input=prompt, cwd=work, capture_output=True, text=True, timeout=TIMEOUT, check=True)
        reply = json.loads(p.stdout)
        if reply.get("is_error"):
            raise RuntimeError(reply.get("result") or "claude reported an error")
        return reply.get("result", "")


def parse(text):
    """The answer object in a reply (tolerates code fences and prose around it)."""
    dec = json.JSONDecoder()
    for m in re.finditer(r"\{", text):
        try:
            obj, _ = dec.raw_decode(text, m.start())
        except ValueError:
            continue
        if isinstance(obj, dict) and "kind" in obj:
            return obj
    raise ValueError("no answer in the reply: " + text[:200])


def write_book(book, langs):
    reply = parse(run_engine(prompt_for(book, langs)))
    kind = reply.get("kind")
    sets = {}
    for l in langs:
        chars = (reply.get("characters") or {}).get(l) or []
        if kind == "fiction" and chars:
            sets[l] = {"status": "ok", "characters": chars}
        else:
            sets[l] = {"status": "none" if kind == "nonfiction" else "unknown", "characters": []}
    source = ENGINE + (" " + MODEL if MODEL else "")
    api("PUT", f"/books/{book['id']}/characters", {"source": source, "sets": sets})
    counts = ", ".join(f"{l}:{len(s['characters'])}" for l, s in sets.items())
    return f"{kind} ({counts})"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--book", type=int, help="write this book's characters again")
    ap.add_argument("--dry-run", action="store_true", help="print the prompt for the first missing book")
    args = ap.parse_args()
    if not TOKEN:
        sys.exit("BOOKSHELF_TOKEN is not set")

    missing = api("GET", f"/characters/missing?limit={LIMIT}")
    if args.book:
        b = api("GET", f"/books/{args.book}")["book"]
        b = next((m for m in missing["books"] if m["id"] == args.book), b)
        todo = [(b, missing["langs"])]
    else:
        todo = [(b, b["missing"]) for b in missing["books"]]
    if not todo:
        return
    if args.dry_run:
        print(prompt_for(*todo[0]))
        return

    failed = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=CONCURRENCY) as pool:
        futures = {pool.submit(write_book, b, langs): b for b, langs in todo}
        for f in concurrent.futures.as_completed(futures):
            b = futures[f]
            try:
                print(f"#{b['id']} {b['title']}: {f.result()}", flush=True)
            except Exception as e:  # one book failing must not stop the others
                failed += 1
                print(f"#{b['id']} {b['title']}: FAILED — {e}", file=sys.stderr, flush=True)
    if failed == len(todo):
        sys.exit(1)


if __name__ == "__main__":
    main()
