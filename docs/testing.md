# Testing

## Unit Tests

```bash
go test ./...                     # run all unit tests
go test ./blog/...                # blog package: parseFrontmatter, splitCommentBlocks, uniqueID, store
go test ./email/...               # email package: cleanSubject, matchPattern, parseNotifyTag, htmlToMarkdown, etc.
go test ./web/...                 # web package: API endpoints (httptest), buildRawMessage
```

54 unit tests covering: parseFrontmatter, splitCommentBlocks, cleanSubject, matchPattern, parseNotifyTag, htmlToMarkdown, decodeBody, isWhitelisted, stripEmailQuotes, API article/comment creation, and more.

## API Test Flow

```bash
# 1. Build and start
go build -o mailblogger .
./mailblogger serve &

# 2. Create article via API
curl -X POST http://localhost:8080/api/article \
  -H 'Content-Type: application/json' \
  -d '{"from":"Alice <alice@example.com>","subject":"Test Post","body":"Hello **world**"}'

# 3. Check article appears on web
curl http://localhost:8080/

# 4. Create comment via API (use actual article ID from step 2)
curl -X POST http://localhost:8080/api/comment \
  -H 'Content-Type: application/json' \
  -d '{"from":"Bob <bob@example.com>","subject":"Re: Test Post","body":"Nice!","reply_to":"<article-id>"}'

# 5. Check article page has comment
curl http://localhost:8080/<article-id>

# 6. Check RSS
curl http://localhost:8080/feed.xml
curl http://localhost:8080/feed-full.xml

# 7. API health check
curl http://localhost:8080/api/status
```

## Manual Test Flow (Email)

```bash
# 1. Build and start
go build -o mailblogger .
./mailblogger serve &

# 2. Send article
./tools/sendmail "wmail@owowo.dev" "Test" "Body"

# 3. Fetch (or wait for IDLE)
./mailblogger fetch

# 4. Check web
curl http://localhost:8080/
curl http://localhost:8080/<article_id>

# 5. Check RSS
curl http://localhost:8080/feed.xml
curl http://localhost:8080/feed-full.xml

# 6. Test [EDIT]
./tools/sendmail "wmail+<article_id>@owowo.dev" "[EDIT] New Title" "New body"
./mailblogger fetch

# 7. Test [DELETE]
./tools/sendmail "wmail+<article_id>@owowo.dev" "[DELETE]" ""
./mailblogger fetch

# 8. Test comment + reply + notification
./tools/sendmail -name "Alice" -from "a@b.com" "wmail+<aid>@owowo.dev" "Re: Test" "Alice's comment"
./mailblogger fetch
# Get comment ID from content/<aid>/comments.md

./tools/sendmail -name "Bob" -from "b@c.com" "wmail+<cid>@owowo.dev" "Re: Test [NOTIFY]" "Bob's reply"
./mailblogger fetch
# Should show: notified a@b.com about reply <new_cid>

# 9. Reset test data
rm -rf content/*/
```

## Test Tool

`tools/sendmail.go` sends emails via SMTP for manual testing:

```bash
cd tools && go build -o sendmail sendmail.go

# Send article
./sendmail "wmail@owowo.dev" "Test Article" "# Markdown body"

# Send comment (use actual article uniqueid)
./sendmail -name "Alice" -from "alice@example.com" "wmail+<uid>@owowo.dev" "Re: Test" "Comment body"

# Send with notification
./sendmail -name "Bob" -from "bob@example.com" "wmail+<uid>@owowo.dev" "Re: Test [NOTIFY]" "Reply body"

# Simulate email client quoting (reply to notification)
./sendmail -name "Alice" -from "alice@example.com" "wmail+<uid>@owowo.dev" "Re: Test" "New reply.

On Mon, Jul 13, 2026, MailBlogger wrote:
> Quoted text here."
```

## Key Assertions

- Article uniqueid is 8 hex chars (SHA256 of Message-ID)
- Comment uniqueid includes parent ID in hash
- Author hash is consistent for same email across articles and comments
- `[DELETE]` removes the entire article directory
- `[EDIT]` replaces body and updates subject
- Quoted text in reply emails is stripped (no `On ... wrote:` or `>` lines)
- `notify: false` → no notification unless `[NOTIFY]` in subject
- `notify: true` → notification unless `[MUTE]` in subject
- Same-author replies don't trigger notification
- Dates render in client local timezone
- Dark mode activates with OS preference
- Feed output cached for 5 minutes, invalidated on article changes
- All config fields hot-reloadable (IMAP, SMTP, whitelist, notify, site)
