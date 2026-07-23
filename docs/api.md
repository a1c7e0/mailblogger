# JSON API

All endpoints return `Content-Type: application/json`.

## Caching

All GET endpoints include `ETag` and `Cache-Control` headers. Clients sending `If-None-Match` receive `304 Not Modified` when the response hasn't changed.

| Endpoint | max-age |
|---|---|
| `/api/site` | 10s |
| `/api/articles` | 5s |
| `/api/article/{id}` | 5s |
| `/api/article/{id}/comments` | 3s |
| `/api/locale` | 30s |

## Read Endpoints

### GET /api/site

Site configuration merged with theme.json fields.

```json
{
  "lang": "en",
  "show_author": true,
  "avatar": "/static/avatar.png",
  "width": 600,
  "links": [{"Title": "About", "URL": "/about"}],
  "email_local": "blog",
  "email_domain": "example.com",
  "title": "My Blog",
  "subtitle": "Email-to-blog",
  "description": "Send an email, it becomes a post.",
  "footer_html": ""
}
```

Fields from theme.json (`title`, `subtitle`, `description`, `footer_html`, etc.) are merged into the response. Config fields take precedence over theme.json.

### GET /api/articles

Paginated articles, sorted by date descending.

**Query parameters:**

| Param | Default | Max | Description |
|---|---|---|---|
| `page` | 1 | - | Page number |
| `per_page` | 20 | 100 | Items per page |

**Response:**
```json
{
  "articles": [
    {
      "uniqueid": "afd888d6",
      "slug": "hello-world",
      "subject": "Hello World",
      "author": "Alice",
      "author_hash": "ff8d9819",
      "date": "2026-07-12T12:00:00Z",
      "banner": "2",
      "excerpt": "Hello World This is the post body..."
    }
  ],
  "total": 42,
  "page": 1,
  "per_page": 20,
  "total_pages": 3
}
```

`excerpt`: markdown-stripped, truncated to 160 chars.

### GET /api/article/{id}

Article detail. `{id}` can be hash or slug.

**Query parameters:**

| Param | Default | Description |
|---|---|---|
| `include` | - | Set to `comments` to embed comments in response |
| `comments_limit` | 50 | Max comments when `include=comments` (max 200) |

**Response:**
```json
{
  "uniqueid": "afd888d6",
  "slug": "hello-world",
  "subject": "Hello World",
  "author": "Alice",
  "author_hash": "ff8d9819",
  "author_email": "alice@example.com",
  "date": "2026-07-12T12:00:00Z",
  "banner": "2",
  "body": "Markdown content...",
  "body_html": "<p>Rendered HTML...</p>",
  "images": ["1.webp", "2.gif"],
  "email_local": "blog",
  "email_domain": "example.com"
}
```

`body_html`: server-rendered HTML from Goldmark (GFM + footnotes + definition lists). Use this instead of client-side markdown rendering when possible. Fenced code blocks include `.code-block`, `.code-block-header`, and `.code-copy-btn[data-code-copy]` markup; a client theme can attach its own clipboard behavior.

With `?include=comments`:
```json
{
  "...": "...",
  "comments": [ ... ],
  "comments_total": 53
}
```

When `comments_total` exceeds `comments_limit`, only the first N comments are returned.

### GET /api/article/{id}/comments

Comments for an article. Filtered by `history.show_deleted` and `history.show_replies` settings.

```json
[
  {
    "uniqueid": "92d93709",
    "author": "Bob",
    "author_hash": "2b3b2b9c",
    "author_email": "bob@example.com",
    "date": "2026-07-12T13:00:00Z",
    "reply_to": "",
    "body": "Great article!",
    "deleted": false,
    "edits": []
  }
]
```

### GET /api/locale

Merged locale strings for the given language (with English fallback).

**Query parameters:**

| Param | Default | Description |
|---|---|---|
| `lang` | `site.lang` | Language code (e.g. `zh`, `en`) |

**Response:** flat key-value object of all locale strings, English as base with requested language merged on top.

```json
{
  "comments": "评论",
  "reply": "[回复]",
  "back_to_index": "← 返回首页",
  "not_found": "未找到",
  "theme_auto": "自动"
}
```

### GET /api/status

Health check.

```json
{"status": "ok", "host": "example.com"}
```

## Write Endpoints

### POST /api/article

Create an article programmatically. Processes through the same pipeline as email.

**Request:**
```json
{
  "from": "Alice <alice@example.com>",
  "to": "blog@example.com",
  "subject": "My Post",
  "body": "Markdown **content**",
  "html_body": "<p>Optional HTML fallback</p>",
  "images": [
    {"data": "base64...", "content_type": "image/png", "filename": "photo.png"}
  ],
  "date": "2026-01-15T10:30:00Z"
}
```

| Field | Required | Description |
|---|---|---|
| `from` | yes | Sender address (must be in whitelist) |
| `to` | no | Blog address |
| `subject` | yes | Article title |
| `body` | yes | Markdown body |
| `html_body` | no | HTML fallback (used for CID image recovery) |
| `images` | no | Base64-encoded image attachments |
| `date` | no | ISO 8601 date (default: now) |

**Response:**
```json
{"ok": true, "id": "afd888d6", "type": "article"}
```

### POST /api/comment

Create a comment.

**Request:**
```json
{
  "from": "Bob <bob@example.com>",
  "to": "blog+afd888d6@example.com",
  "subject": "Re: My Post",
  "body": "Great article!",
  "reply_to": "afd888d6",
  "images": [],
  "date": "2026-01-15T11:00:00Z"
}
```

If `to` is omitted and `reply_to` is provided, the `to` address is auto-generated from the blog's email config.

**Response:**
```json
{"ok": true, "id": "92d93709", "type": "comment"}
```

### POST /api/raw-email

Webhook endpoint for receiving raw RFC 2822 emails (used by Cloudflare Email Worker).

**Headers:**
- `Content-Type: message/rfc822`
- `X-Webhook-Secret: <secret>` — must match `webhook.secret` in config

**Body:** raw RFC 2822 email bytes (up to 35 MiB)

**Response:**
```json
{"ok": true, "id": "afd888d6", "type": "email"}
```

**Errors:**

| Status | Cause |
|---|---|
| 403 | Missing/invalid secret, or webhook not configured |
| 400 | Invalid email format |
| 405 | Non-POST method |
| 500 | Processing error (DKIM failure, etc.) |

## Error Format

All endpoints return errors as:
```json
{"ok": false, "error": "description of what went wrong"}
```

## Usage Examples

```bash
# List articles (paginated)
curl 'http://localhost:8080/api/articles?page=1&per_page=10'

# Get article with embedded comments
curl 'http://localhost:8080/api/article/afd888d6?include=comments'

# Get locale strings
curl 'http://localhost:8080/api/locale?lang=zh'

# Create article
curl -X POST http://localhost:8080/api/article \
  -H 'Content-Type: application/json' \
  -d '{"from":"Alice <alice@example.com>","subject":"Hello","body":"World"}'

# Create comment
curl -X POST http://localhost:8080/api/comment \
  -H 'Content-Type: application/json' \
  -d '{"from":"Bob <bob@example.com>","subject":"Re: Hello","body":"Nice!","reply_to":"afd888d6"}'

# Get comments
curl http://localhost:8080/api/article/afd888d6/comments
```
