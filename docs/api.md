# JSON API

All endpoints return `Content-Type: application/json`.

## Read Endpoints

### GET /api/site

Site configuration.

```json
{
  "lang": "en",
  "show_author": true,
  "avatar": "/static/avatar.png",
  "width": 600,
  "links": [{"Title": "About", "URL": "/about"}],
  "email_local": "blog",
  "email_domain": "example.com"
}
```

### GET /api/articles

All articles, sorted by date descending.

```json
[
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
]
```

`excerpt`: markdown-stripped, truncated to 160 chars.

### GET /api/article/{id}

Article detail. `{id}` can be hash or slug.

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
  "images": ["1.webp", "2.gif"],
  "email_local": "blog",
  "email_domain": "example.com"
}
```

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
# Create article
curl -X POST http://localhost:8080/api/article \
  -H 'Content-Type: application/json' \
  -d '{"from":"Alice <alice@example.com>","subject":"Hello","body":"World"}'

# List articles
curl http://localhost:8080/api/articles

# Get article detail
curl http://localhost:8080/api/article/afd888d6

# Create comment
curl -X POST http://localhost:8080/api/comment \
  -H 'Content-Type: application/json' \
  -d '{"from":"Bob <bob@example.com>","subject":"Re: Hello","body":"Nice!","reply_to":"afd888d6"}'

# Get comments
curl http://localhost:8080/api/article/afd888d6/comments
```
