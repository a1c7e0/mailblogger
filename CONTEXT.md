# Domain Glossary

Core concepts and their canonical names in the MailBlogger domain.

## Content

- **Article** — a blog post, stored as `index.md` (YAML frontmatter + markdown body) in a date-prefixed directory. Created by email or API.
- **Comment** — a reply to an article or another comment, stored in `comments.json` (JSON array) in the article's directory.
- **Draft** — an article from a non-whitelisted sender, saved to `_drafts/` for review.
- **Unique ID** — 8-character hex string (SHA256 of Message-ID). Identifies articles and comments.
- **Author Hash** — 8-character hex string (SHA256 of email address). Stable identity across articles, used for privacy-preserving linking.
- **Slug** — custom URL path segment (e.g., `hello-world`). Set via body config. Alternative to hash-based URLs.
- **Banner** — article image number that replaces the site avatar at page top. Set via body config.
- **Frontmatter** — YAML block between `---` delimiters at the top of `index.md`. Contains article metadata.
- **Body Config** — `---` delimited key-value block at the start of an email body. Keys: `banner`, `slug`, `notify`, `title`.

## Email

- **Plus-addressing** — routing mechanism: `blog+<uniqueid>@domain` routes to article or comment by ID.
- **Target ID** — the uniqueid extracted from the `+` addressing in the `To` header. Determines whether an email creates an article (no target) or a comment (has target).
- **Hash Forwarding** — transparent email relay: `blog+<author_hash>@domain` forwards to the author's real email without exposing it.
- **DKIM** — email authentication via DNS. Verified on raw email bytes before processing.
- **IMAP Poller** — background loop that connects to IMAP, fetches unseen messages, processes them, and deletes them. 30s interval with exponential backoff.
- **FetchOnce** — single IMAP fetch-process-delete cycle, used by both the poller and the one-shot `fetch` command.
- **Webhook** — Cloudflare Email Worker that POSTs raw RFC 2822 emails to `/api/raw-email`. Alternative to IMAP.

## Notification

- **Watcher** — per-article opt-in for notifications (SQLite `article_watchers`).
- **Muter** — per-article opt-out for notifications (SQLite `article_muters`).
- **Three-tier priority** — per-article watcher/muter overrides per-user preference overrides global default.
- **Settings Token** — 32-byte random hex, 24h TTL, for accessing the settings page. Created by emailing subject `settings`.

## Storage

- **Store** — the central data access layer. Filesystem for content, SQLite for metadata.
- **Cache** — in-memory `sync.Once` maps (hash→dir, slug→dir, comment→article). Invalidated on writes.
- **History** — version control for articles and comments. `edit_N/` directories hold snapshots. `_deleted/` holds archived deletions.
- **Comments JSON** — `comments.json` file in each article directory. JSON array of comment objects with optional `edits` array and `deleted` flag.

## Web

- **SPA** — single-page application mode. When a theme is configured, all non-API routes serve the theme's `index.html`.
- **SSR** — server-side rendering fallback. Uses Go `html/template` when no theme is configured.
- **Theme** — client-side frontend in `themes/<name>/`. Contains `index.html`, `app.js`, and optional locale files.
- **Feed Cache** — 5-minute TTL cache for Atom feeds, invalidated on article changes.
