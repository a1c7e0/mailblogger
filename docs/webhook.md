# Webhook: Cloudflare Email Worker

## Overview

Alternative to IMAP polling. Cloudflare Email Worker forwards raw RFC 2822 messages to `/api/raw-email`.

```
Sender → Cloudflare Email Routing → Worker → POST /api/raw-email → ProcessMessage()
```

IMAP and webhook can run simultaneously — both feed into `ProcessMessage()`.

## Server Config

```yaml
mail:
  address: blog@example.com
  # imap: (optional, can omit for webhook-only)

webhook:
  secret: "your-random-secret"
```

## Worker Setup

1. `cp wrangler.example.toml wrangler.toml` — set `LOCAL_SERVER_URL`
2. `cp worker.example.js worker.js`
3. `wrangler secret put WEBHOOK_SECRET`
4. `wrangler deploy`
5. Cloudflare Dashboard → Email Routing → Routes → point to worker

## API Endpoint

**POST /api/raw-email**

- Header: `X-Webhook-Secret: <secret>` — must match config
- Body: raw RFC 2822 email (up to 35 MiB)
- Response: `{"ok": true, "id": "abc12345", "type": "email"}`
- Errors: 403 (bad secret), 400 (bad format), 405 (not POST), 500 (processing)

## Flow

`ParseRawEmail(rawBytes)` extracts headers + `parseBodyParts()` → same `RawMessage` as IMAP → `ProcessMessage()` handles routing.
