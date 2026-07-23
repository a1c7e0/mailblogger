# Deployment

## Build & Run

```bash
go build -o mailblogger .
./mailblogger serve -config config.yaml
```

## Docker

Multi-stage build: `golang:1.26-alpine` (compile with `gcc`/`musl-dev` for CGo) → `alpine:3.21` (runtime with `libwebp`).

```bash
docker build -t mailblogger .
docker run -p 8080:8080 -v ./config.yaml:/app/config.yaml -v ./content:/app/content -v ./themes:/app/themes mailblogger
```

### docker-compose.yml

```yaml
services:
  mailblogger:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./content:/app/content
      - ./config.yaml:/app/config.yaml:ro
      - ./themes:/app/themes
    restart: unless-stopped
```

### Entrypoint & Theme Volume

The container uses `/entrypoint.sh` which syncs the baked-in default theme (`/app/default-theme/default/`) into the themes volume (`/app/themes/default/`) on every start:

- First run: populates `themes/default/` from the image
- Image upgrade: new/modified files are copied (via `cp -ru`)
- `theme.json`: **never overwritten** after first run — your customizations are preserved
- Custom themes in other directories are untouched

To override the default theme, edit files directly in `./themes/default/`.

## Production Notes

1. `config.yaml`: exclude from git, permissions `600`
2. `content/`: persistent volume, back up regularly
3. TLS: reverse proxy (nginx/Caddy) for HTTPS
4. SMTP: port 465 implicit TLS required
5. DKIM: configure sender domain's DKIM records

## Ports

| Service | Port | Protocol |
|---|---|---|
| Web | 8080 | HTTP |
| IMAP | 993 | TLS |
| SMTP | 465 | TLS |

## Signals

`SIGINT` / `SIGTERM` → graceful shutdown (10s timeout): HTTP drains, Poller stops, config watcher stops.

## Architecture Changes (v2025-07)

- Web package consolidated to 3 files: `server.go` (SSR/SPA), `static.go` (assets/feed/sitemap), `render.go` (content rendering)
- SMTP sender factory: `NewSenderFromConfig` eliminates duplicate construction
- Image references: numeric `![1]` in markdown, resolved to `1.webp` at save time
- Quote stripping: preserves user-authored `>` quotes, only removes reply template