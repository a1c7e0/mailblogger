# Email Processing

## IMAP Client (`email/imap.go`)

### Connection
- Implicit TLS on port 993
- `ConnectIMAP(cfg)` → authenticated client
- `FetchUnseen(c)` → search for messages without `\Seen` flag, fetch with `BODY.PEEK[]`
- `MarkAsSeen(c, seqNums)` → mark messages as `\Seen` via STORE (uses message sequence numbers, not UIDs)
- `DeleteEmails(c, seqNums)` → mark as `\Deleted`, then EXPUNGE

### Polling Mode (`main.go:imapPoller`)
Short-lived connections, 30-second interval:
1. Connect → `FetchUnseen()` → process → `DeleteEmails()` → disconnect
2. Wait 30 seconds
3. Repeat

Connection failures use exponential backoff (1s → 2s → 4s ... max 2min), reset on success.

### Body Extraction
Priority: `text/plain` > `text/html`.
- `extractMultipartAll()`: single-pass extraction of text body, HTML body, and images from multipart structure. Recursive up to 5 levels deep, max 100 parts per level.
- `extractMultipart()`: text-only extraction (used for non-multipart fallback)
- `cleanBody()`: strip `\r\n`, trim whitespace, strip email quotes (see below)
- `htmlToMarkdown()`: pre-compiled regexes for HTML tag stripping (fallback for HTML-only emails)

### Single-Parse Architecture
`parseMessage()` calls `mail.ReadMessage()` once, then:
- `text/plain` → `decodeBody()` → `cleanBody()`
- `multipart/*` → `extractMultipartAll()` returns `(text, html, images)` in one pass
- `text/html` → `decodeBody()` → `htmlToMarkdown()`
- `scanMIMEPartsForHTML()` — HTML body extraction

This avoids parsing the same raw email body 3 times.

### Email Quote Stripping
`stripEmailQuotes(body)` matches lines starting with:
- `On ... wrote:` pattern (common email reply delimiter)
- `>` or `|` prefix (quoted text)
- Bare `---` separator line

Truncates everything from the first matched line. If truncation would produce empty body, keeps original.

## Processor (`email/processor.go`)

### Processing Flow

```
ProcessMessage(raw)
  ├── DKIM check → fail → sendErrorReply()
  ├── isSettingsCommand(raw)? → handleSettingsCommand()
  │     Generate token → email settings link → return
  ├── handleHashForward(raw)?
  │     To=blog+hash@domain → lookup real email → forward passthrough → return
  ├── parseTargetID(To) → extract uniqueid from wmail+<id>@domain
  ├── No uniqueid → processArticle()
  │     ├── Whitelist check → fail → SaveDraft() + sendDraftReply()
  │     ├── parseBodyConfig() → invalid → sendErrorReply()
  │     ├── Slug conflict → sendErrorReply()
  │     ├── Banner validation → sendErrorReply()
  │     └── Generate uniqueid → SaveArticle() + apply notify watcher/muter
  └── Has uniqueid → processComment()
        ├── [WATCH]/[MUTE] → update article_watchers/muters in SQLite
        ├── Try article match → check [EDIT]/[DELETE] or top-level comment
        │     └── [EDIT]: parseBodyConfig() → invalid → sendErrorReply()
        ├── Try comment match → reply comment
        │     └── Target not found → sendErrorReply()
        └── notifyReply() using 3-tier priority
```

### Settings Command
Subject after stripping Re:/Fwd: prefixes equals `settings` (case-insensitive):
1. Clean expired tokens
2. Generate new token (32-byte random hex, 24h TTL)
3. Store in `settings_tokens` table
4. Send reply email with settings link: `scheme://host/settings?t=<token>`
5. Return (no article/comment created)

### Hash Forwarding
When someone sends email to `blog+<author_hash>@domain`:
1. `handleHashForward()` detects To address without plus-addressing in the local part (just a hash)
2. Looks up `FindEmailByHash()` in SQLite
3. Forwards as transparent passthrough to `blog+hash@domain` (which routes through normal comment flow)
4. From: original sender, Reply-To: original sender — recipient sees the real person, not the blog
5. No `Fwd:` prefix — completely transparent

### Article Management Commands
Only the original article author (matched by email) can execute commands. Subject after stripping `Re:`/`Fwd:` prefixes is matched case-insensitively:
- `edit` → replace article body; always delete all existing images and save new ones if any; update banner/slug/title from body config; handle notify preference from body config; keep original title unless overridden by body config `title`
- `delete` → remove entire article directory, ignore body content

Non-author emails with subject `edit` or `delete` are treated as regular comments.

### Body Configuration
Articles can declare configuration at the beginning of the body using a frontmatter-style block:

```
---
banner: 2
slug: my-post
---

Actual article body starts here.
```

**Detection rules:**
- Opening delimiter: `---` (3+ dashes) on the first line
- Each line between delimiters must be `key: value` where key is a known config key (`banner`, `slug`, `notify`)
- If any line has an unknown key or invalid format → no config block, entire body is treated as text
- Empty lines between config lines are allowed
- No closing `---` → no config block

**Supported keys:**
|| Key | Description |
||---|---|
|| `banner` | Image number to use as article banner (replaces site avatar at page top). `1` = first image |
|| `slug` | Custom URL slug (same as `[SLUG:xxx]` in subject) |
|| `notify` | `on`/`true`/`watch` → watch article; `off`/`false`/`mute` → mute article |
|| `title` | Override article title (new articles: overrides email subject; edit: overrides original title) |

Body config applies to both new articles and `[EDIT]` commands. Subject-level config (e.g. `[SLUG:xxx]`) takes precedence over body config.

If the config block is detected but malformed (e.g. unclosed block, invalid line), the email is rejected with an error reply.

### Error Replies
The system sends automated error replies to the sender when:
| Scenario | Error message |
|---|---|
| Slug already in use | `Slug "xxx" is already in use.` |
| Invalid banner value | `Invalid banner value "xxx". Must be a positive number.` |
| Comment target not found | `No article or comment found with ID "xxx".` |
| DKIM verification failed | `Email rejected: DKIM verification failed for <domain>.` |
| Hash forward failed | `Failed to forward your email.` |

Non-whitelisted senders receive a draft notification instead of an error.

Error replies do NOT quote the original body, preventing duplication when the user hits Reply.

Both commands are detected case-insensitively, anywhere in subject.

### Draft Mode
Non-whitelisted sender → `SaveDraft()` to `content/_drafts/<uid>/`

### Subject Cleaning
`cleanSubject()` strips `Re:`, `Fwd:` prefixes (case-insensitive, iteratively).
`[NOTIFY]`, `[MUTE]`, `[WATCH]`, `[NOWATCH]` tags are stripped via `stripNotifyTags()`.
`[SLUG:...]` tag is extracted for custom URL and stripped from subject.

## Notification Control

### Three-Tier Priority

```
1. Per-article overrides (SQLite: article_watchers / article_muters)
   → muters take priority over watchers
2. Per-user preferences (SQLite: user_prefs table)
   → configured via settings page
3. Global defaults (config: mail.notify.article / mail.notify.comment)
```

### Decision Logic (`store.ShouldNotify`)
```
ShouldNotify(authorHash, articleID, isArticle):
  if IsMuter(articleID, authorHash) → false
  if IsWatcher(articleID, articleHash) → true
  prefs = GetPrefs(authorHash)
  if prefs != nil → use prefs.article_notify or prefs.comment_notify
  → use global default
```

### Per-Article Overrides
- `[WATCH]` or `[NOTIFY]` in subject → add author to article_watchers
- `[MUTE]` or `[NOWATCH]` in subject → add author to article_muters
- Applied when sending comments, stored in SQLite

### Per-User Preferences
- Configured via web settings page (`/settings?t=<token>`)
- Stored in `user_prefs` table (author_hash → article_notify, comment_notify, hide_email)
- Token-based auth: email `settings` → receive link → 24h expiry
- `hide_email` controls whether author email is shown in frontend tooltips

### Notification Email
`notifyReply(c, articleID)`:
- Skips if `c.ReplyTo == ""`, no sender configured, or same-author reply
- Calls `store.ShouldNotify()` for the parent comment's author
- Sends plain-text email to parent comment's author
- `From: <mailbox>@<domain>`
- `Reply-To: <mailbox>+<new_comment_uid>@<domain>` — replying routes back to thread
- Body includes new reply content with `>` quoting
- Walks the reply chain with `collectAncestors()` to include thread context (up to 5 ancestors, 6000 char limit)
- Each message in the thread shows `Name (#uniqueid)` for identification

## DKIM Verification (`email/dkim.go`)

### Flow
1. Extract `DKIM-Signature` header from raw email body
2. Parse parameters: domain (`d`), selector (`s`), signature (`b`), algorithm (`a`), headers (`h`)
3. DNS TXT query: `<selector>._domainkey.<domain>`
4. Parse RSA public key from PEM-encoded `p=` value
5. Hash canonicalized headers with SHA256
6. `rsa.VerifyPKCS1v15()` with decoded signature

### Behavior
- No DKIM header → pass through (not every sender uses DKIM)
- DKIM verification fails → reject the email
- DKIM verification passes → allow processing

## RawMessage Struct
```go
type RawMessage struct {
    SeqNum     uint32          // message sequence number for STORE
    From       *mail.Address
    To         []*mail.Address
    Subject    string
    Date       string
    MessageID  string
    Body       string           // cleaned text/plain body
    RawBody    []byte           // raw bytes for DKIM verification
    Images     []ImageData      // extracted image attachments
    HTMLBody   string           // extracted HTML body (for CID image recovery)
}
```

## Image Extraction (`email/images.go`)

### Flow
1. `extractImagesFromParsed(msg)` — accepts pre-parsed `*mail.Message`, recursively extracts images from nested multipart
2. For each image: decode transfer encoding (base64/quoted-printable), extract CID and filename
3. `convertToWebp()` — attempt JPEG/PNG → WebP via `github.com/chai2010/webp` (pure Go, CGo wrapper); GIF/WebP pass through
4. `saveArticleImages()` — save as sequential files (`1.webp`, `2.gif`, ...), build CID→number map
5. CID references in article body (`cid:xxx`) are replaced with the corresponding number
6. If CID images exist but body has no `![` references, HTML body is converted to markdown to recover image references

### Recursive Multipart
`extractImagesFromMultipart()` handles nested multipart structures (e.g., Thunderbird's `multipart/mixed` → `multipart/alternative` → images). Recursion limited to 5 levels, 100 parts per level.

### Comment Images
- Saved as `c_<commentUID>_<N>.<ext>` via `saveCommentImages()`
- Listed by `store.ListCommentImages()` and rendered as filename hyperlinks in the comment section

### Article Image Files
- Saved directly to the article directory via `os.WriteFile` (when dir is known) or `store.SaveImage()`
- Filenames: `1.webp`, `2.gif`, etc. (extension determined by `convertToWebp()`)
- Referenced in markdown as `![alt](1)` — no extension needed
- `<base>` tag in article template ensures relative `src="1"` resolves correctly

## Comment Storage

Comments are stored in `comments.json` (JSON array) in the article directory.

### JSON Structure
```json
[
  {
    "uniqueid": "a1b2c3",
    "author": "Alice",
    "author_hash": "...",
    "author_email": "alice@example.com",
    "date": "2025-01-01T00:00:00Z",
    "reply_to": "",
    "body": "Current content",
    "deleted": false,
    "edits": [
      { "date": "2025-01-02T00:00:00Z", "body": "Previous content" }
    ]
  }
]
```

### Comment Commands
Subject after stripping `Re:`/`Fwd:` prefixes is matched case-insensitively:
- `edit` → update comment body; if `history.comment.keep` is true, old body is saved to `edits` array
- `delete` → mark comment as deleted (`deleted: true`); if `history.comment.keep` is false, clear `edits` array

Non-author emails with subject `edit` or `delete` are treated as regular replies.

## Article History

### Edit History
When `history.article.keep` is true, editing an article:
1. Copies current `index.md`, `comments.json`, and images to `edit_N/` directory (N starts at 0)
2. Saves new content to the main directory

### Delete Archive
When `history.article.keep` is true, deleting an article:
1. Moves the entire article directory to `content/_deleted/`

When `history.article.keep` is false:
1. Permanently removes the article directory
