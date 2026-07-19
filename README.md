# MailBlogger

Email-to-blog. Send an email, it becomes a post. Reply to a post by email, it becomes a comment.

Styled like a kernel mailing list archive — monospace, left-aligned, zero JavaScript requirement (graceful fallback).

## Quick Start

```bash
# 1. Edit config.yaml with your mail address (IMAP optional, see below)
# 2. Build
go build -o mailblogger .

# 3. Start web server
./mailblogger serve

# Open http://localhost:8080
```

### Receiving Emails

MailBlogger supports two ways to receive emails — use either or both:

**Option A: IMAP polling** (traditional)

Configure IMAP in `config.yaml` and use `./mailblogger fetch` for one-shot or `./mailblogger serve` for auto-polling every 30s.

**Option B: Cloudflare Email Worker** (webhook, recommended)

No IMAP needed. Cloudflare receives emails and POSTs them to your server in real-time.

1. Set `webhook.secret` in `config.yaml`
2. Deploy the Worker (see `worker.example.js` + `wrangler.example.toml`)
3. Configure Email Routing in Cloudflare Dashboard → point to Worker

```yaml
# config.yaml (webhook-only, no IMAP)
mail:
  address: blog@example.com
webhook:
  secret: "your-random-secret"
```

## How It Works

### Publishing an Article

Send an email to `wmail@owowo.dev` (or whatever address you configure). The sender must be in the whitelist (`config.yaml`).

```
From: Your Name <you@example.com>
To: wmail@owowo.dev
Subject: My First Post

Hello World! This is the post body in markdown.
```

### Commenting on an Article

Every article has an 8-char unique ID displayed on the page. Reply to `wmail+<uniqueid>@owowo.dev`:

```
From: Reader <reader@example.com>
To: wmail+afd888d6@owowo.dev
Subject: Re: My First Post

Great article! I have a question...
```

### Replying to a Comment

Every comment also has its own unique ID. Send to `wmail+<comment_uniqueid>@owowo.dev`:

```
From: Another Reader <other@example.com>
To: wmail+92d93709@owowo.dev
Subject: Re: My First Post

I was wondering the same thing.
```

### Notification Emails

When someone replies to your comment, you receive an email notification. The `Reply-To` header points to `wmail+<new_comment_uid>@domain` — just hit Reply in your email client to continue the discussion.

### Editing / Deleting Articles

As the article author, send to `wmail+<article_uid>@owowo.dev`:

| Action | Subject | Effect |
|---|---|---|
| Edit | `[EDIT] New Title` | Replaces article body and title |
| Delete | `[DELETE]` | Removes the entire article directory |

### Body Configuration

Article options can be declared at the beginning of the email body:

```
---
banner: 2
slug: my-post
notify: on
---

Actual article body starts here.
```

| Key | Description |
|---|---|
| `banner` | Image number to use as page banner (replaces site avatar) |
| `slug` | Custom URL (same as `[SLUG:xxx]` in subject) |
| `notify` | `on`/`true` → watch article; `off`/`false` → mute article |

Use 3+ dashes as delimiters. If the config block is malformed, you'll receive an error reply.

### Settings

Send an email with subject `settings` to get a link to configure notification preferences and email privacy.

### Error Replies

If your email can't be processed (invalid config, slug conflict, target not found, etc.), you'll receive an automated error reply explaining the issue.

## Configuration

```yaml
# config.yaml
mail:
  address: wmail@owowo.dev
  imap:                              # optional — omit to disable IMAP polling
    server: imap.purelymail.com
    port: 993
    username: wmail@owowo.dev
    password: your-password-here
  smtp:
    server: smtp.purelymail.com
    port: 465
    # username and password fall back to IMAP credentials if not set
  whitelist:                 # allowed article authors
    - "*@owowo.dev"

webhook:                     # Cloudflare Email Worker webhook
  secret: "your-secret"      # must match Worker's WEBHOOK_SECRET env var

content_dir: content       # where articles are stored

site:
  title: My Blog           # displayed in header
  subtitle: ""             # optional tagline
  footer_html: ""          # HTML footer (supports <a>, <script>, etc.)

web:
  port: 8080
  host: 0.0.0.0
```

## Content Structure

```
content/
├── 20260713_afd888d6_hello-world/   # date_hash_slug
│   ├── index.md                     # frontmatter YAML + markdown body
│   ├── comments.md                  # multiple YAML-document blocks
│   ├── 1.webp                       # article images (sequential)
│   └── c_92d93709_1.png             # comment images
├── _drafts/                         # non-whitelisted submissions
└── mailblogger.db                   # SQLite (tokens, prefs, watchers)
```

## Privacy

- Author names come from the email's From header display name (e.g., `Alice` from `Alice <a@b.com>`)
- If no name is set, a hash of the email address is shown instead
- Author email visibility is controlled by `privacy.hide_email` (default: hidden) and per-user preferences
- Each author is identified by a stable `author_hash` (SHA256 of email, first 8 chars)
- Unique IDs are shown on all articles and comments
- Visitors can contact authors via `blog+<author_hash>@domain` without seeing the real address

## Testing

A test SMTP sender is provided for development:

```bash
cd tools && go build -o sendmail sendmail.go

# Send a test article
./sendmail "wmail@owowo.dev" "Hello World" "# Post body in markdown"

# Send a test comment (replace <uid> with actual article ID)
./sendmail -name "Alice" -from "alice@example.com" "wmail+<uid>@owowo.dev" "Re: Hello" "Great post!"

# Then process
cd .. && ./mailblogger fetch
```
