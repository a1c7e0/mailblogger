# AGENTS.md

## Build & Run

```bash
go build -o mailblogger .          # build
./mailblogger fetch                # one-shot: pull new emails via IMAP, process, mark as seen
./mailblogger serve                # start web server + IMAP poll (if configured) + config watcher
./mailblogger serve -config path   # custom config path
```

IMAP is optional. If `mail.imap.server` is not configured, `fetch` exits early and `serve` skips the IMAP poller. Emails can be received solely via the `/api/raw-email` webhook (Cloudflare Email Worker).

## Documentation

Full agent documentation in `docs/`. Read these before modifying any module:

| Doc | Module |
|---|---|
| `docs/architecture.md` | Project structure, data flow, design decisions |
| `docs/config.md` | Configuration reference, address parsing, whitelist |
| `docs/content-storage.md` | Frontmatter format, file layout, store API |
| `docs/email-processing.md` | IMAP client, processor flow, DKIM, polling, notification |
| `docs/smtp.md` | SMTP sender, notification email format, test tool |
| `docs/web-server.md` | HTTP handlers, rendering, SPA, API, feed, settings |
| `docs/comment-threading.md` | Threading model, reply routing, notification chain |
| `docs/uniqueids.md` | Hash generation, author hashing, display names |
| `docs/privacy.md` | Email hiding, frontend data exposure |
| `docs/ux.md` | Timestamps, buttons, pagination, dark mode |
| `docs/deployment.md` | Docker, ports, signals, production notes |
| `docs/testing.md` | Manual test flow, API endpoints, sendmail tool |
| `docs/webhook.md` | Cloudflare Email Worker webhook, raw email endpoint |
| `docs/api.md` | JSON API endpoints, request/response formats |
| `docs/themes.md` | Theme system, locale files, creating custom themes |

## Project Architecture

```
main.go             → CLI entry point, fetch/serve, signal handling, config watcher
config/config.go    → YAML config loading, defaults, address parsing
blog/
  article.go        → Article, Comment, CommentEdit structs
  uniqueid.go       → SHA256 hash generation for IDs and author linking
  store.go          → Filesystem storage, sync.Once cache, filtered comment queries
  store_sql.go      → SQLite metadata (tokens, user prefs, watchers/muters)
email/
  imap.go           → IMAP client, MIME parsing, parseBodyParts, htmlToMarkdown
  smtp.go           → SMTP sender (implicit TLS, port 465)
  processor.go      → Processor struct, ProcessMessage dispatch, config parsing
  processor_article.go → Article create/edit/delete lifecycle
  processor_comment.go → Comment create/edit/delete lifecycle
  processor_notify.go  → Notification emails, ancestor threading, quoted replies
  poller.go         → IMAP Poller with backoff, FetchOnce
  images.go         → Image extraction, WebP conversion, CID replacement
  dkim.go           → DKIM signature verification via DNS
web/
  server.go         → HTTP routing, handlers, SPA, settings page
  render.go         → Markdown→HTML, image wrapping, date formatting, mailto links
  assets.go         → Favicon/avatar detection, ICO generation
  api.go            → REST API: POST article/comment, GET site/articles/status, raw-email webhook
  feed.go           → Atom feed generation with 5min cache
  sitemap.go        → XML sitemap generation
static/             → CSS and JS (spa.js for client-side navigation)
themes/             → Custom theme directory (SPA entry points)
tools/sendmail.go   → SMTP test tool for development
content/            → Generated output (articles, comments, images, SQLite DB)
docs/               → Per-module agent documentation
```

## Dependencies

| Package | Version | Usage |
|---|---|---|
| `github.com/emersion/go-imap` | v1.2.1 | IMAP fetch, search, store flags |
| `github.com/yuin/goldmark` | v1.8.4 | Markdown → HTML with GFM + footnotes + definition lists |
| `gopkg.in/yaml.v3` | v3.0.1 | Config load, frontmatter marshal/unmarshal |
| `github.com/fsnotify/fsnotify` | v1.10.1 | Config file change detection |
| `modernc.org/sqlite` | v1.53.0 | SQLite metadata (tokens, user prefs, article watchers/muters) |
| `github.com/chai2010/webp` | v1.4.0 | Image → WebP encoding (CGo, requires libwebp) |
| `golang.org/x/image` | v0.44.0 | Image scaling (draw.NearestNeighbor for ICO generation) |

No external web framework — `net/http` + `html/template`.

## Data Storage

Two-tier architecture:
- **Filesystem**: articles, comments, images (markdown + frontmatter)
- **SQLite** (`content/mailblogger.db`): settings tokens, user notification preferences, per-article watch/mute lists

SQLite does NOT store article/comment content. Filesystem is the source of truth for content.

## JSON API

Read-only JSON API endpoints:
- `GET /api/site` — site information
- `GET /api/articles` — all articles
- `GET /api/article/{id}` — article detail (by hash or slug)
- `GET /api/article/{id}/comments` — article comments
- `GET /api/status` — server status

## Testing

```bash
go test ./...                     # run all unit tests (109)
go test ./blog/...                # blog package tests
go test ./email/...               # email package tests
go test ./web/...                 # web package tests (includes API tests)
```
