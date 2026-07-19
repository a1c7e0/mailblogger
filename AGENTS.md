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
| `docs/web-server.md` | HTTP handlers, goldmark, templates, graceful shutdown, API |
| `docs/comment-threading.md` | Threading model, reply routing, notification chain |
| `docs/uniqueids.md` | Hash generation, author hashing, display names |
| `docs/privacy.md` | Email hiding, frontend data exposure |
| `docs/ux.md` | Timestamps, buttons, pagination, dark mode |
| `docs/deployment.md` | Docker, ports, signals, production notes |
| `docs/testing.md` | Manual test flow, API endpoints, sendmail tool |
| `docs/webhook.md` | Cloudflare Email Worker webhook, raw email endpoint |

## Project Architecture

```
config/     -> YAML config loading (config.yaml at repo root)
blog/       -> domain models (Article, Comment), SHA256-hash unique IDs, filesystem store, sync.Once cache, SQLite metadata
email/      -> IMAP client, SMTP sender, email body extraction, process logic (article/command/comment/settings dispatch), DKIM
web/        -> HTTP server, goldmark md->html, Atom feeds, API endpoints, settings page, go:embed templates
static/     -> CSS served from filesystem
tools/      -> sendmail helper for testing (SMTP send)
content/    -> generated output directory (articles as folders with index.md + comments.md)
docs/       -> per-module agent documentation
```

## Dependencies

| Package | Version | Usage |
|---|---|---|
| `github.com/emersion/go-imap` | v1.2.1 | IMAP fetch, search, store flags |
| `github.com/yuin/goldmark` | v1.8.4 | Markdown → HTML with GFM + footnotes + definition lists |
| `gopkg.in/yaml.v3` | v3.0.1 | Config load, frontmatter marshal/unmarshal |
| `github.com/fsnotify/fsnotify` | v1.10.1 | Config file change detection |
| `modernc.org/sqlite` | v1.53.0 | SQLite metadata (tokens, user prefs, article watchers/muters) |
| `github.com/chai2010/webp` | v1.4.0 | Image → webp encoding (CGo, requires libwebp) |
| `golang.org/x/image` | v0.44.0 | Image scaling (draw.NearestNeighbor for ICO generation) |

No external web framework — `net/http` + `html/template`.

## Data Storage

Two-tier architecture:
- **Filesystem**: articles, comments, images (markdown + frontmatter)
- **SQLite** (`content/mailblogger.db`): settings tokens, user notification preferences, per-article watch/mute lists

SQLite does NOT store article/comment content. Filesystem is the source of truth for content.

## Testing

```bash
go test ./...                     # run all unit tests
go test ./blog/...                # blog package tests
go test ./email/...               # email package tests
go test ./web/...                 # web package tests (includes API tests)
```

109 unit tests covering: parseFrontmatter, splitCommentBlocks, cleanSubject, matchPattern, parseNotifyTag, htmlToMarkdown, parseBodyConfig, API endpoints, ParseRawEmail, webhook auth, and more.
