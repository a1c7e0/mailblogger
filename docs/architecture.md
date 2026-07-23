# Architecture

## Project Structure

```
mailblogger/
├── main.go                      # CLI entry point, fetch/serve commands, signal handling, config watcher
├── config/config.go             # YAML config loading, defaults, address parsing
├── blog/
│   ├── article.go               # Article, Comment, CommentEdit structs
│   ├── uniqueid.go              # SHA256 hash generation for IDs and author linking
│   ├── store.go                 # Filesystem storage, sync.Once cache, filtered comment queries
│   └── store_sql.go             # SQLite metadata (tokens, user prefs, watchers/muters)
├── email/
│   ├── imap.go                  # IMAP client, MIME parsing, parseBodyParts, htmlToMarkdown
│   ├── smtp.go                  # SMTP sender (implicit TLS, port 465), NewSenderFromConfig factory
│   ├── processor.go             # Processor struct, ProcessMessage dispatch, config parsing
│   ├── processor_article.go     # Article create/edit/delete lifecycle
│   ├── processor_comment.go     # Comment create/edit/delete lifecycle
│   ├── processor_notify.go      # Notification emails, ancestor threading, quoted replies
│   ├── poller.go                # IMAP Poller with backoff, FetchOnce
│   ├── images.go                # Image extraction, WebP conversion, CID replacement
│   └── dkim.go                  # DKIM signature verification via DNS
├── web/
│   ├── server.go                # HTTP routing, handlers, SPA, settings page (5 fields, 6 template funcs)
│   ├── static.go                # Static files, assets, feed, sitemap, robots.txt
│   ├── render.go                # Markdown→HTML, image/code-block wrapping, date formatting, mailto links
│   ├── api.go                   # REST API: POST article/comment, GET site/articles/status, raw-email webhook
│   └── templates/               # go:embed HTML templates (index.html, article.html, settings.html)
├── static/                      # CSS and JS (spa.js for client-side navigation)
├── themes/                      # Custom theme directory (SPA entry points)
├── tools/sendmail.go            # SMTP test tool
└── content/                     # Generated output (articles, comments, images, SQLite DB)
```

## Data Flow

```
Email → IMAP poll (30s) or webhook (/api/raw-email)
  → ParseRawEmail() / parseMessage() → parseBodyParts()
    → text/plain, multipart/*, text/html → RawMessage (Body, HTMLBody, Images)
  → ProcessMessage()
    → DKIM check → fail → error reply
    → Settings command? → handleSettingsCommand()
    → Hash forward? → handleHashForward()
    → parseTargetID() extracts uniqueid from To header
    → No uniqueid → processArticle()
      → Whitelist check → fail → SaveDraft() + draft reply
      → parseBodyConfig() → extract banner/slug/notify/title
      → Extract images → saveArticleImages() → CID replacement
      → resolveImageNumbers() → SaveArticle()
    → Has uniqueid → processComment()
      → Target article? → check [EDIT]/[DELETE] or new comment
      → Target comment? → reply comment + notifyReply()
  → DeleteEmails() → EXPUNGE
```

## Design Decisions

- **No external web framework**: `net/http` + `html/template` + `go:embed`
- **Filesystem as source of truth**: articles/comments as markdown + frontmatter; SQLite for metadata only
- **No external mail parser**: `net/mail` for MIME, custom `parseBodyParts` + `extractMultipartAll`
- **Plus-addressing**: `blog@domain` → `blog+<uid>@domain` for routing
- **sync.Once cache**: article list, hash maps, slug maps built once, invalidated on writes
- **Implicit TLS SMTP**: port 465, no STARTTLS
- **IMAP polling**: 30s interval, short-lived connections, exponential backoff (1s → 2min)
- **Three web files**: `server.go` (SSR/SPA), `static.go` (assets/feed/sitemap), `render.go` (content rendering)
- **Sender factory**: `NewSenderFromConfig` eliminates duplicate SMTP construction

## Module Interface Summary

| Module | Interface | Depth |
|--------|-----------|-------|
| `blog.Store` | `SaveArticle`, `GetArticle`, `ListArticles`, `SaveComment`, `GetComments`, `AddWatcher/Muter`, `ShouldNotify` | Deep: 30+ methods behind small public interface |
| `email.Processor` | `ProcessMessage` | Deep: full email lifecycle behind one method |
| `web.Server` | `Handler()`, `SetConfigGetter`, `InvalidateFeedCache` | Deep: HTTP routing, rendering, API behind small interface |
| `email.Poller` | `Start()` | Shallow: single method, encapsulates polling loop |