# MailBlogger

Email-to-blog. Send an email, it becomes a post. Reply by email, it becomes a comment.

## Quick Start

```bash
# 1. Copy and edit config
cp config.example.yaml config.yaml
# Edit mail.address, IMAP/SMTP credentials, whitelist

# 2. Build
go build -o mailblogger .

# 3. Start
./mailblogger serve
# Open http://localhost:8080
```

## Receiving Emails

**Option A: IMAP polling** — configure `mail.imap` in config.yaml. `./mailblogger serve` polls every 30s. `./mailblogger fetch` for one-shot.

**Option B: Cloudflare Email Worker** (recommended) — no IMAP needed. Cloudflare receives emails and POSTs to your server in real-time.

```yaml
# config.yaml (webhook-only)
mail:
  address: blog@example.com
webhook:
  secret: "your-random-secret"
```

Deploy the Worker (see `worker.example.js` + `wrangler.example.toml`), configure Email Routing in Cloudflare Dashboard.

Both can run simultaneously.

## Publishing

Send an email to your blog address. Sender must be in the whitelist.

```
From: Alice <alice@example.com>
To: blog@example.com
Subject: My First Post

Hello World! Markdown **supported**.
```

## Commenting

Every article and comment has an 8-char unique ID shown on the page. Reply to `blog+<id>@domain`:

```
To: blog+afd888d6@example.com
Subject: Re: My First Post

Great article!
```

## Editing & Deleting

Send to `blog+<article_id>@domain`:

| Subject | Effect |
|---|---|
| `edit` | Replace body. Old version archived if `history.article.keep` is true |
| `delete` | Move to `_deleted/` or permanently remove |

Same for comments: send `edit` or `delete` to `blog+<comment_id>@domain`.

## Body Configuration

Declare options at the beginning of the email body:

```
---config
banner: 2
slug: my-post
notify: on
title: Custom Title
---

Article body starts here.
```

| Key | Description |
|---|---|
| `banner` | Image number to use as page banner (replaces site avatar) |
| `slug` | Custom URL slug (lowercase, digits, dashes) |
| `title` | Override article title |
| `notify` | `on`/`true` → watch; `off`/`false` → mute |

## Notification

When someone replies to your comment, you receive a notification email. Hit Reply to continue the discussion — the `Reply-To` header routes back to the thread.

Send an email with subject `settings` to configure notification preferences.

Three-tier priority: per-article override > per-user preference > global default.

## API

Read-only JSON API for building custom frontends. See [docs/api.md](docs/api.md).

| Endpoint | Description |
|---|---|
| `GET /api/site` | Site information (includes theme.json) |
| `GET /api/articles` | Paginated articles |
| `GET /api/article/{id}` | Article detail (by hash or slug) |
| `GET /api/article/{id}/comments` | Article comments |
| `GET /api/locale?lang=zh` | Merged locale strings |
| `POST /api/article` | Create article |
| `POST /api/comment` | Create comment |
| `POST /api/raw-email` | Webhook: receive raw email |

## Themes

Themes control the entire frontend. Set `theme` in config.yaml:

```yaml
# Single theme
theme: default

# Per-language themes (requires site.auto_lang: true)
theme:
  en: default
  zh: default
```

Theme files go in `themes/<name>/`. See [docs/themes.md](docs/themes.md) for the full theme authoring guide.

## Configuration

See `config.example.yaml` for all options. Key fields:

```yaml
mail:
  address: blog@example.com
  imap:
    server: imap.example.com
    username: blog@example.com
    password: your-password
  smtp:
    server: smtp.example.com
    port: 465
  whitelist:
    - "*@example.com"

site:
  lang: en
  show_author: true
  width: 600
  # auto_lang: false       # detect browser language for per-language themes
  # links:                 # navigation links in header
  #   - title: "About"
  #     url: "/about"

web:
  port: 8080
  scheme: https

theme: default              # or per-language map (see Themes section)

privacy:
  hide_email: true

history:
  article:
    keep: true
    visible: true
  comment:
    keep: true
    visible: true
  show_deleted: true
  show_replies: true
```

All config fields are hot-reloadable — edit `config.yaml` and changes take effect immediately.

## Content Structure

```
content/
├── 20260713_afd888d6_hello-world/
│   ├── index.md           # frontmatter + markdown body
│   ├── comments.json      # JSON array of comments
│   ├── 1.webp             # article images
│   └── edit_0/            # archived version (if history enabled)
├── _drafts/               # non-whitelisted submissions
├── _deleted/              # archived deleted articles
└── mailblogger.db         # SQLite metadata
```

## Privacy

- Author names from email `From:` header; fallback to hash
- Author emails hidden by default (`privacy.hide_email`)
- Visitors contact authors via `blog+<author_hash>@domain` without seeing real addresses
- Notification `Reply-To` routes through blog, not the replier's address

## Docker

```bash
docker build -t mailblogger .
docker run -p 8080:8080 \
  -v ./config.yaml:/app/config.yaml \
  -v ./content:/app/content \
  mailblogger
```

## License

MIT
