# Deployment

## Build & Run

```bash
go build -o mailblogger .
./mailblogger serve -config config.yaml
```

## Docker

Multi-stage build: `golang:1.26-alpine` (compile) → `alpine:3.21` (runtime with `libwebp`).

```bash
docker build -t mailblogger .
docker run -p 8080:8080 -v ./config.yaml:/app/config.yaml -v ./content:/app/content mailblogger
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
    restart: unless-stopped
```

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
