# Web Server

## File: `web/server.go`

## Dependencies
- `net/http` for routing and serving
- `html/template` with custom `FuncMap`
- `go:embed` for template embedding
- `github.com/yuin/goldmark` with extensions for markdown rendering

## Routes
| Path | Handler | Description |
|---|---|---|
| `/` | `handleIndex` | Article list with pagination |
| `/<hash>` | `handleArticle` | Article by hash (tried first) |
| `/<slug>` | `handleArticle` | Article by slug (fallback) |
| `/<hash-or-slug>/<filename>` | `serveArticleFile` | Serve image or file from article directory |
| `/feed.xml` | `handleFeed` | Atom feed (articles only, 5min cache) |
| `/feed-full.xml` | `handleFeedFull` | Atom feed (articles + comments, 5min cache) |
| `/sitemap.xml` | `handleSitemap` | XML sitemap (all articles, 5min cache) |
| `/robots.txt` | `handleRobotsTXT` | User-provided from content/ or generated with sitemap pointer |
| `/static/*` | `handleStatic` | Static files from `static/`; intercepts `favicon.svg` and `avatar.*` from content/ |
| `/favicon.ico` | `handleFaviconICO` | Favicon from content/ or memory cache |
| `/api/status` | `handleAPIStatus` | API health check |
| `/api/article` | `handleAPIArticle` | Create article via JSON (POST) |
| `/api/comment` | `handleAPIComment` | Create comment via JSON (POST) |
| `/settings` | `handleSettings` | User notification + privacy settings (token-authenticated) |

## Template Functions

| Name | Signature | Description |
|---|---|---|
| `renderMD` | `(string) template.HTML` | Goldmark markdown to HTML with image post-processing |
| `renderPlaintext` | `(string) template.HTML` | Plain text with URL linking and line breaks |
| `mailto` | `(uniqueID, subject, emailLocal, emailDomain, body) string` | Generate mailto: link with body quoting |
| `fmtDate` | `(interface{}) string` | Format date to UTC: `YYYY-MM-DD HH:MM UTC` |
| `fmtDateTitle` | `(interface{}) string` | Full date tooltip: ISO 8601 + Unix timestamp |
| `datetimeISO` | `(interface{}) string` | ISO 8601 UTC: `YYYY-MM-DDTHH:MM:SSZ` |
| `rawHTML` | `(string) template.HTML` | Pass raw HTML through |
| `add`, `sub` | `(int, int) int` | Arithmetic helpers |
| `truncate` | `(string, int) string` | Truncate with ellipsis |
| `urlencode` | `(string) string` | URL-encode (alias for `url.QueryEscape`) |
| `commentImages` | `(articleID, commentUID string) []string` | List image filenames for a comment (prefix `c_<uid>_`) |
| `authorTooltip` | `(hash, email string) string` | Generate author tooltip: shows email+hash when visible, hash-only when hidden |

## Server Struct
```go
type Server struct {
    Store       *blog.Store
    Host        string       // web host for RSS links
    Scheme      string       // URL scheme (http or https)
    EmailLocal  string       // parsed from config address
    EmailDomain string       // parsed from config address
    HideEmail   bool         // global default for email visibility
    Site        config.SiteConfig
    Port        int
    Addr        string       // listen address
    tmpl        *template.Template
    configGetter func() *config.Config  // returns current config for API handlers
    avatarFile  string       // auto-detected avatar filename (e.g. "avatar.webp")
    cachedFaviconSVG []byte  // generated favicon.svg from avatar (memory only)
    cachedFaviconICO []byte  // generated favicon.ico from avatar (memory only)
}
```

## Handler Patterns

### Pagination (`handleIndex`)
- Query param: `?page=N` (default 1)
- 20 articles per page (`perPage`)
- Passes `Page`, `TotalPages`, `HasPrev`, `HasNext`, `PrevPage`, `NextPage` to template

### Article Page (`handleArticle`)
- Tries `GetArticle(id)` (hash lookup via cache), then `GetArticleBySlug(id)` (slug lookup via cache)
- Both O(1) after cache warmup; no `/h/` prefix needed
- Renders article + comments with threaded grouping (replies under parents)
- Template includes `<base href="/slug-or-id/">` so relative image `src="1"` resolves correctly without trailing-slash redirect
- Unreferenced images shown in "Attachments" section with grid gallery (comment images with `c_` prefix excluded)

### Markdown Rendering (`renderMarkdown`)
1. `ensureImageBreaks()` — pre-processes body to add blank lines before/after every `![...](...)` line, preventing inline image grouping
2. Goldmark renders markdown to HTML
3. `wrapImages()` — post-processes `<img>` tags: wraps in `<figure>` (centered), `<a target="_blank">` (clickable, new tab), `<figcaption>` (alt text)

### SPA Navigation (`static/spa.js`)
Client-side navigation for internal links:
- Intercepts clicks on non-static, non-mailto, non-`target="_blank"` links
- Fetches target page, extracts `<main>` content, replaces current `<main>`
- Swaps `.header-banner-area` content (avatar/banner) from fetched page
- Updates `<base>` tag and URL via `history.pushState()`
- Calls `initPage()` to re-bind event handlers (timezone, copy, reply highlighting)
- Header title/nav and footer (music player, etc.) persist across navigations
- Browser back/forward triggers full reload as fallback

### Comments Rendering
`renderArticleBody()`:
1. Loads all comments via `GetComments(article.UniqueID)`
2. Groups into thread tree: top-level first, each followed by its replies
3. Passes `displayComments` to template with `Depth` (0 or 1) and `ReplyTarget`

### Email Privacy (`authorTooltip`)
Template function that generates author hover tooltip:
- Checks per-user `hide_email` preference via `store.ShouldHideEmail()`
- Falls back to global `privacy.hide_email` config
- When visible: `"alice@example.com\nhash: a1b2c3d4"` (email first, hash below)
- When hidden: `"hash: a1b2c3d4"` (hash only)
- Applied to article authors and comment authors in all templates

### Settings Page (`handleSettings`)
Token-based authentication:
- GET `/settings?t=<token>` — shows notification preferences + privacy setting + user's articles
- POST `/settings?t=<token>` — saves preferences, redirects with `?saved=1`
- Token: 32-byte random hex, 24h TTL, stored in SQLite `settings_tokens` table
- Invalid/expired tokens → 403
- Page shows: notification toggles (article replies, comment replies), email privacy toggle, article list (read-only), expiry countdown

### Config Reload
- `UpdateConfig(site)` — updates Site config at runtime
- `SetConfigGetter(fn)` — sets callback for API handlers to access current config
- Config is stored in `atomic.Pointer[config.Config]` in main.go, swapped atomically on file change
- All config fields (IMAP, SMTP, whitelist, notify, site) are hot-reloadable
- IMAP poller reads latest config on each reconnect cycle

## Feed Generation (`web/feed.go`)

Atom 1.0 format, using `xmlEscape()` for XML special chars.
- `/feed.xml`: one `<entry>` per article with rendered HTML content
- `/feed-full.xml`: same, but appends all comments to the article content with `<hr/>` separators
- Relative `<img src>` paths are rewritten to absolute URLs (`scheme://host/articleID/filename`) via `rewriteFeedImages()` so images display correctly in feed readers
- Feed output is cached for 5 minutes (`feedCacheStore`) with automatic invalidation when articles change

## API Endpoints (`web/api.go`)

### POST /api/article
Create an article programmatically (no email required).
```json
{
  "from": "Alice <alice@example.com>",
  "subject": "My Post",
  "body": "Markdown content",
  "html_body": "<p>Optional HTML fallback</p>",
  "images": [{"data": "base64...", "content_type": "image/png", "filename": "photo.png"}],
  "date": "2026-01-15T10:30:00Z"
}
```
Response: `{"ok": true, "id": "abc12345", "type": "article"}`

### POST /api/comment
Create a comment on an article.
```json
{
  "from": "Bob <bob@example.com>",
  "to": "blog+<article-id>@domain.com",
  "subject": "Re: My Post",
  "body": "Great article!",
  "reply_to": "<article-id>",
  "images": [],
  "date": "2026-01-15T11:00:00Z"
}
```
If `to` is omitted and `reply_to` is provided, the `to` address is auto-generated from the blog's email config.

### GET /api/status
Health check. Returns `{"status": "ok", "host": "...", "site": "..."}`.

## Main Entry Point (`main.go`)

### Commands
- `fetch`: one-shot IMAP fetch + process + delete
- `serve`: start web server + IMAP poll (30s) + config watcher

### Graceful Shutdown
`runServe()`:
1. Start `http.Server` in goroutine
2. Start IMAP poller goroutine
3. Start fsnotify config watcher
4. Block on `SIGINT`/`SIGTERM`
5. Close `done` channel → stops IMAP poller
6. `httpSrv.Shutdown()` with 10s timeout → drains HTTP connections

### Exponential Backoff
In `imapPoller()`: connection failures → wait 1s, 2s, 4s, ... up to 2min max. Resets on successful fetch cycle.

### Config Hot-Reload
`watchConfig()` uses `fsnotify` to watch `config.yaml` for write events. On change, loads new config, stores it atomically via `atomic.Pointer`, and calls `srv.UpdateConfig()` for site settings. IMAP poller picks up new config on its next cycle.
