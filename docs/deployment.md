# Deployment

## Build

```bash
go build -o mailblogger .
```

## Run

```bash
./mailblogger serve -config config.yaml
```

## Docker

### Dockerfile
Multi-stage build:
1. `golang:1.26-alpine` — compile with CGo enabled (requires `libwebp-dev` in builder)
2. `alpine:3.21` — runtime with `ca-certificates`, `tzdata`, and `libwebp`

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

1. **config.yaml**: exclude from git, keep permissions `600`
2. **content/**: persistent volume, back up regularly
3. **TLS**: reverse proxy (nginx/Caddy) for HTTPS
4. **IMAP**: ensure server supports IDLE for real-time processing
5. **SMTP**: port 465 implicit TLS required
6. **DKIM**: configure sender domain's DKIM records for verification

## Ports

| Service | Port | Protocol |
|---|---|---|
| Web server | 8080 | HTTP |
| IMAP | 993 | TLS |
| SMTP | 465 | TLS |

## Filesystem

- `content/` — article data, must be writable and persistent
- `config.yaml` — must exist at startup (configurable via `-config` flag)
- `static/` — CSS files served to clients

## Signals

- `SIGINT` / `SIGTERM` → graceful shutdown (10s timeout)
- HTTP server drains requests
- IMAP IDLE goroutine stops
- Config file watcher stops
