# Webhook: Cloudflare Email Worker

## Overview

Instead of polling IMAP, emails can be received via a Cloudflare Email Worker that forwards raw RFC 2822 messages to the local server's `/api/raw-email` endpoint.

```
Sender → Cloudflare Email Routing → Worker → POST /api/raw-email → ProcessMessage()
```

IMAP and webhook can run simultaneously — both feed into the same `ProcessMessage()` pipeline.

## Server Setup

Add to `config.yaml`:

```yaml
# Disable IMAP (webhook-only mode)
mail:
  address: blog@example.com

webhook:
  secret: "your-random-secret"
```

Or keep IMAP enabled for parallel mode:

```yaml
mail:
  address: blog@example.com
  imap:
    server: imap.example.com
    ...

webhook:
  secret: "your-random-secret"
```

## Worker Setup

1. Copy `wrangler.example.toml` to `wrangler.toml` and edit:
   - Set `LOCAL_SERVER_URL` to your server's public URL
   - Update `name` if desired

2. Copy `worker.example.js` to `worker.js`

3. Set the webhook secret:
   ```bash
   wrangler secret put WEBHOOK_SECRET
   ```

4. Deploy:
   ```bash
   wrangler deploy
   ```

5. In Cloudflare Dashboard → Email Routing → Routes, add a route pointing to the worker

## API Endpoint

### POST /api/raw-email

Accepts a raw RFC 2822 email body.

**Headers:**
- `Content-Type: message/rfc822`
- `X-Webhook-Secret: <secret>` — must match `webhook.secret` in config

**Limits:**
- Body size: 35 MiB (accommodates base64-encoded attachments from 25 MiB Cloudflare limit)

**Response:**
```json
{"ok": true, "id": "abc12345", "type": "email"}
```

**Error responses:**
- `403` — missing/invalid secret, or webhook not configured
- `400` — invalid email format
- `405` — non-POST method
- `500` — processing error (DKIM failure, etc.)

## Email Flow

The raw email is parsed into the same `RawMessage` struct used by IMAP:

1. Headers extracted (From, To, Subject, Date, Message-ID)
2. MIME body parsed (text/plain, text/html, multipart with images)
3. `RawBody` preserved for DKIM verification
4. `ProcessMessage()` handles routing (article, comment, settings, etc.)

## Worker Files

| File | Description |
|---|---|
| `worker.example.js` | Worker source code template |
| `wrangler.example.toml` | Wrangler config template |

These are templates — copy and customize:
```bash
cp worker.example.js worker.js
cp wrangler.example.toml wrangler.toml
```

Add `worker.js` and `wrangler.toml` to `.gitignore` since they contain deployment-specific config.
