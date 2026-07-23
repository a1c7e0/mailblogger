# ROADMAP

## v0.1 — Complete

### Core
- [x] IMAP polling + exponential backoff reconnect
- [x] SMTP sender (port 465 implicit TLS), `NewSenderFromConfig` factory
- [x] Article publish via whitelist
- [x] Article management: `[EDIT]` / `[DELETE]` commands by email
- [x] Comment threading via plus-address routing (`<mailbox>+<id>@domain`)
- [x] SHA256 unique IDs (8-char hex), displayed on all items
- [x] Author hash (SHA256 of email, 8-char hex) for same-person linking
- [x] DKIM signature verification (DNS TXT lookup, reject on failure)
- [x] Email quote stripping (remove reply template, **preserve user-authored `>` quotes**)
- [x] Config hot-reload via fsnotify

### Content Storage
- [x] YAML frontmatter + markdown filesystem store
- [x] Timestamped directory: `YYYYMMDD_hash_slug`
- [x] Custom slug via body config
- [x] Slug alias resolved from directory name (no frontmatter storage)
- [x] Draft mode: non-whitelisted senders → `_drafts/`
- [x] Memory cache for hash/slug lookups (O(1) after warmup)
- [x] Image refs: `![alt](1)` → `![alt](1.webp)` at save time

### Web
- [x] goldmark markdown → HTML (GFM tables, footnotes, definition lists)
- [x] lore.kernel.org style: left-aligned, monospace, blue links, flat design
- [x] Configurable container width (`site.width`, default 600px)
- [x] Banner avatar (`site.avatar`), full container width, flush top/left
- [x] Nav links (`site.links`), fully customizable title + URL
- [x] `<time datetime="...">` with JS client-local timezone conversion
- [x] Date tooltip: ISO 8601 + Unix timestamp
- [x] mailto: reply links with quoted body context
- [x] Comment reply-target highlighting (scroll + outline, no layout shift)
- [x] Clickable #uid copies permalink (slug or hash URL)
- [x] `[copy reply address]` button per comment
- [x] Pagination (`?page=N`)
- [x] Dark mode (`@media prefers-color-scheme`)
- [x] Atom feeds: `/feed.xml` (articles), `/feed-full.xml` (articles + comments)
- [x] Comment count displayed in `Comments (N)` heading
- [x] Notification hint: config default on/off with `[NOTIFY]`/`[MUTE]` override
- [x] Server struct slimmed to 5 fields, template funcs reduced to 8
- [x] Web package consolidated to 3 files: server.go, static.go, render.go

### Notifications
- [x] Email notification on reply (configurable default + per-email `[NOTIFY]`/`[MUTE]`)
- [x] Thread context in notification email (up to 4 ancestors)
- [x] Unique ID shown in notification thread (`Name (#uniqueid)`)
- [x] `Reply-To` routes back to comment thread
- [x] Same-author suppression
- [x] Subject keywords stripped from display title

### Privacy
- [x] Display name from email From header name; fallback to author hash
- [x] Zero sender email addresses in frontend HTML (backend only)
- [x] Author hash visible via tooltip, uniqueid on all items

### Infrastructure
- [x] Graceful shutdown (SIGTERM → drain HTTP + stop IMAP)
- [x] Multi-stage Dockerfile
- [x] docker-compose.yml
- [x] Comprehensive agent docs in `docs/` (15 modules)

## v0.2 — Short term

### Content
- [ ] Tags / categories via `[TAG:...]` subject prefix
- [ ] Article full-text search
- [ ] Scheduled publishing (future `Date:` header)
- [ ] Multi-file attachments (save to `attachments/`, link in body)
- [ ] Archive by month/year pages

### UX
- [ ] Keyboard shortcuts (j/k next/prev article, r reply)
- [ ] Article word/read-time count
- [ ] Image gallery / lightbox view

## v0.3 — Medium term

### Architecture
- [ ] SQLite backend option
- [ ] Prometheus metrics endpoint
- [ ] Admin web panel (basic stats, moderation)

### Publishing
- [ ] Multi-author (different mailbox per section)
- [ ] HTML-to-markdown body import improvement
- [ ] Email-based comment moderation queue

### Federation
- [ ] ActivityPub (Mastodon-compatible)
- [ ] WebMention
- [ ] Import from WordPress / Jekyll