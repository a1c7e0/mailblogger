# Architecture

## Project Structure

```
mailblogger/
├── main.go                  # Entry point: CLI, fetch/serve commands, signal handling, atomic config
├── config.yaml              # User configuration (excluded from git)
├── config.example.yaml      # Template configuration (committed)
├── config/config.go         # YAML config loading, defaults, address parsing
├── blog/
│   ├── article.go           # Article and Comment structs
│   ├── uniqueid.go          # SHA256 hash generation for IDs and author linking (hashID internal)
│   └── store.go             # Filesystem storage: read/write articles, comments, images; sync.Once cache
├── email/
│   ├── imap.go              # IMAP client: connect, fetch, parse (single-pass multipart), body cleaning
│   ├── smtp.go              # SMTP sender: implicit TLS on port 465, full domain for TLS/auth
│   ├── processor.go         # Email dispatch: article, comment, [EDIT], [DELETE], DKIM, notify
│   ├── images.go            # Image extraction from parsed MIME, recursive multipart, WebP conversion (Go), CID replacement
│   └── dkim.go              # DKIM signature verification via DNS TXT lookup
├── web/
│   ├── server.go            # HTTP server, handlers, template funcs, markdown render, scheme-aware
│   ├── feed.go              # Atom feed generation with 5min cache, image URL rewriting
│   ├── api.go               # REST API: POST /api/article, POST /api/comment, GET /api/status
│   └── templates/
│       ├── index.html       # Article list with pagination
│       └── article.html     # Article + comments with reply/copy/link buttons, image gallery
├── static/
│   ├── style.css              # Monospace, left-aligned, dark mode
│   └── spa.js                 # SPA navigation, page init (timezone, copy, highlights)
├── tools/sendmail.go         # SMTP test tool for development
├── Dockerfile                # Multi-stage build
├── docker-compose.yml        # Service definition
└── docs/                     # Agent documentation
```

## Data Flow

```
Email → IMAP poll (30s interval) → FetchUnseen()
  → parseMessage() → single mail.ReadMessage → RawMessage (with Images, HTMLBody)
    → extractMultipartAll() single-pass: text + HTML + images
  → ProcessMessage()
    → DKIM check → fail → error reply
    → parseTargetID() extracts uniqueid from To header
    → No uniqueid → processArticle()
      → Whitelist check → fail → SaveDraft() + draft reply
      → parseBodyConfig() → invalid → error reply
      → Slug conflict / bad banner → error reply
      → Extract images → saveArticleImages() → CID replacement
      → Generate uniqueid → SaveArticle()
    → Has uniqueid → processComment()
      → Target not found → error reply
      → Article match → check [EDIT]/[DELETE] or top-level comment
      → Comment match → reply comment + saveCommentImages() + notifyReply()
  → DeleteEmails() → EXPUNGE
  → disconnect → wait 30s → repeat
```

## Key Design Decisions

- **No external web framework**: `net/http` + `html/template` + `go:embed`
- **No database**: filesystem with YAML frontmatter + markdown body
- **No external mail parser**: `net/mail` for MIME, custom recursive multipart extraction
- **Minimal JS**: SPA navigation (`static/spa.js`), timezone conversion, clipboard, comment highlighting; header/footer persist across page navigations
- **Implicit TLS SMTP**: port 465, no STARTTLS
- **IMAP polling**: 30-second interval, short-lived connections; exponential backoff on failure
- **Plus-addressing**: `mail.address: blog@owowo.dev` → `EmailLocal=blog`, `EmailDomain=owowo.dev` → `blog+<uid>@owowo.dev`
- **sync.Once cache**: article list, hash maps, and slug maps built once and invalidated on writes
- **Feed cache**: 5-minute TTL with automatic invalidation on article changes
- **API endpoint**: POST /api/article and /api/comment for programmatic access without email
- **Avatar auto-detection**: scans `content/avatar.{png,jpg,webp,svg}` at startup; favicon auto-generated from avatar and cached in memory
