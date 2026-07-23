# Testing

## Unit Tests

```bash
go test ./...                     # all tests (112)
go test ./blog/...                # blog: frontmatter, comments, uniqueID, store, SQLite
go test ./email/...               # email: cleanSubject, matchPattern, parseNotifyTag, htmlToMarkdown, DKIM, ParseRawEmail
go test ./web/...                 # web: API endpoints (httptest), buildRawMessage
```

## API Test Flow

```bash
# Build and start
go build -o mailblogger .
./mailblogger serve &

# Create article
curl -X POST http://localhost:8080/api/article \
  -H 'Content-Type: application/json' \
  -d '{"from":"Alice <alice@example.com>","subject":"Test Post","body":"Hello **world**"}'

# Create comment (use actual article ID from response)
curl -X POST http://localhost:8080/api/comment \
  -H 'Content-Type: application/json' \
  -d '{"from":"Bob <bob@example.com>","subject":"Re: Test","body":"Nice!","reply_to":"<article-id>"}'

# Check feeds
curl http://localhost:8080/feed.xml
curl http://localhost:8080/api/status
```

## Manual Test Flow (Email)

```bash
# Send article
./tools/sendmail "blog@domain.com" "Test" "Body"

# Fetch (or wait for poller)
./mailblogger fetch

# Edit
./tools/sendmail "blog+<article_id>@domain.com" "edit" "New body"

# Delete
./tools/sendmail "blog+<article_id>@domain.com" "delete" ""

# Comment + reply
./tools/sendmail -name "Alice" -from "a@b.com" "blog+<aid>@domain" "Re: Test" "Comment"
./tools/sendmail -name "Bob" -from "b@c.com" "blog+<cid>@domain" "Re: Test [NOTIFY]" "Reply"
```

## Test Tool

`tools/sendmail.go` — standalone SMTP sender:
```bash
cd tools && go build -o sendmail sendmail.go
./sendmail -name "Alice" -from "alice@example.com" "blog+<uid>@domain" "Re: Test" "Body"
```

## Key Assertions

- Article uniqueid: 8 hex chars (SHA256 of Message-ID)
- Comment uniqueid includes parent ID in hash input
- Author hash consistent for same email across articles/comments
- `delete` subject removes article (or marks comment deleted)
- `edit` subject replaces body, archives old version
- Email quotes stripped (`On ... wrote:`, `> ---` reply template); **user-authored `>` quotes are preserved**
- Notification 3-tier: per-article override > per-user pref > global default
- Same-author replies don't trigger notification
- Feed cache: 5min TTL, invalidated on article changes
- All config fields hot-reloadable
- Image refs: `![1]` in markdown → `![1](1.webp)` at save time
- SMTP sender constructed via `NewSenderFromConfig` factory