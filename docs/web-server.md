# Web Server

## Files

| File | Responsibility |
|---|---|
| `web/server.go` | HTTP routing, handlers, SPA, settings page, template setup |
| `web/render.go` | Markdown→HTML, image wrapping, date formatting, mailto links, excerpt |
| `web/assets.go` | Favicon/avatar detection, ICO generation, base64 encoding |
| `web/api.go` | REST API endpoints, raw email webhook |
| `web/feed.go` | Atom feed generation with 5min cache |
| `web/sitemap.go` | XML sitemap generation |

## Routes

| Path | Handler | Description |
|---|---|---|
| `GET /` | `handleSPA` → `handleIndex` | Article list with pagination |
| `GET /<hash-or-slug>` | `handleSPA` → `handleArticle` | Article page |
| `GET /<id>/<file>` | `handleSPA` → `serveArticleFile` | Article image/file |
| `GET /api/site` | `handleAPISite` | Site info JSON |
| `GET /api/articles` | `handleAPIArticles` | All articles JSON |
| `GET /api/article/{id}` | `handleAPIArticleDetail` | Article detail JSON |
| `GET /api/article/{id}/comments` | `handleAPIArticleComments` | Article comments JSON |
| `GET /api/status` | `handleAPIStatus` | Health check |
| `POST /api/article` | `handleAPIArticle` | Create article via JSON |
| `POST /api/comment` | `handleAPIComment` | Create comment via JSON |
| `POST /api/raw-email` | `handleAPIRawEmail` | Webhook: receive raw RFC 2822 email |
| `GET /feed.xml` | `handleFeed` | Atom feed (articles, 5min cache) |
| `GET /feed-full.xml` | `handleFeedFull` | Atom feed (articles + comments) |
| `GET /sitemap.xml` | `handleSitemap` | XML sitemap |
| `GET /robots.txt` | `handleRobotsTXT` | User-provided or generated |
| `GET /static/*` | `handleStatic` | Static files; intercepts favicon.svg, avatar |
| `GET /favicon.ico` | `handleFaviconICO` | Favicon from content/ or memory cache |
| `GET /settings` | `handleSettings` | User notification settings (token auth) |

## SPA Mode

When `theme` is configured, `handleSPA` serves `themes/<name>/index.html` for all non-API, non-static routes. The theme's `app.js` fetches data from the JSON API.

Without a theme, `handleSPA` falls back to SSR: `handleIndex` for `/`, `handleArticle` for `/<id>`.

## Template Functions

| Name | Description |
|---|---|
| `renderMD` | Goldmark markdown → HTML with figure wrapping |
| `renderPlaintext` | Plain text with URL auto-linking and `<br>` |
| `mailto` | Generate mailto: link with quoted body |
| `fmtDate` / `fmtDateTitle` / `datetimeISO` | Date formatting (UTC, tooltip, ISO) |
| `commentImages` | List image filenames for a comment |
| `authorTooltip` | Email+hash or hash-only based on privacy settings |
| `rawHTML` / `add` / `sub` / `truncate` / `urlencode` | Utilities |

## Markdown Rendering (`render.go`)

1. `ensureImageBreaks()` — add blank lines around `![...](...)` references
2. Goldmark renders markdown → HTML (GFM + footnotes + definition lists)
3. `wrapImages()` — wrap `<img>` in `<figure>` + `<a target="_blank">` + `<figcaption>`

## Comment Filtering

`blog.Store.GetFilteredComments(articleID, opts)` is the single filtering interface:
- `opts.ShowDeleted`: include deleted comments
- `opts.ShowReplies`: include replies to deleted comments

Used by both SSR rendering (`renderArticleBody`) and JSON API (`handleAPIArticleComments`).

## Feed Generation (`feed.go`)

- `/feed.xml`: Atom 1.0, one `<entry>` per article
- `/feed-full.xml`: same + all comments appended with `<hr/>`
- Relative `<img src>` rewritten to absolute URLs
- 5-minute TTL cache, invalidated on article changes

## Settings Page (`handleSettings`)

Token-based auth: email "settings" → receive link → 24h expiry.
- GET: show notification toggles, email privacy toggle, article list
- POST: save preferences, redirect with `?saved=1`
- CSRF protection via cookie + hidden form field

## Config Hot-Reload

`watchConfig()` uses `fsnotify` on `config.yaml`. On change: load → `atomic.Pointer` swap → `srv.UpdateConfig()`. All config fields hot-reloadable.

## Graceful Shutdown

`SIGINT`/`SIGTERM` → close `done` channel (stops Poller) → `httpSrv.Shutdown()` with 10s timeout.
