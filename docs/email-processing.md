# Email Processing

## IMAP Client (`email/imap.go`)

- `ConnectIMAP(cfg)` → TLS connection on port 993
- `FetchUnseen(c)` → search without `\Seen` flag, fetch with `BODY.PEEK[]`
- `DeleteEmails(c, seqNums)` → mark `\Deleted` + EXPUNGE

### Body Extraction

`parseBodyParts(parsed *mail.Message) → (body, html, images)`:
- `text/plain` → `decodeBody()` → `cleanBody()`
- `multipart/*` → `extractMultipartAll()` — single-pass recursive extraction (5 levels, 100 parts)
- `text/html` → `decodeBody()` → `htmlToMarkdown()`

Both `ParseRawEmail()` (webhook) and `parseMessage()` (IMAP) call `parseBodyParts()`.

### Body Cleaning

- `cleanBody()`: trim, normalize `\r\n` → `\n`, strip email quotes
- `stripEmailQuotes()`: truncates at first `On ... wrote:`, `> ---` followed by "Write your reply above this line", or `---` line. **User-authored `>` quotes in the body are preserved.**

## IMAP Poller (`email/poller.go`)

`Poller` struct with `Start()` method. Short-lived connections, 30s interval:
1. Load latest config → create Processor
2. `FetchOnce()` → connect → fetch → process → delete → disconnect
3. Wait 30s, repeat

Exponential backoff on failure: 1s → 2s → 4s ... max 2min, reset on success.

`FetchOnce()` is also used by the one-shot `fetch` command in `main.go`.

## Processor (`email/processor.go` + `processor_*.go`)

### Dispatch (`ProcessMessage`)

```
DKIM check → fail → sendErrorReply()
Settings command? → handleSettingsCommand() → token + email link
Hash forward? → handleHashForward() → transparent passthrough
parseTargetID(To):
  No uniqueid → processArticle()      [processor_article.go]
  Has uniqueid → processComment()     [processor_comment.go]
```

### Article Lifecycle (`processor_article.go`)

- `processArticle()`: whitelist → draft or publish; parseBodyConfig for slug/banner/notify/title; save images + CID replacement; resolve numeric image refs to full filenames
- `handleEditCommand()`: archive version, replace body/images, update config fields
- `handleDeleteCommand()`: archive to `_deleted/` or permanent delete

### Comment Lifecycle (`processor_comment.go`)

- `processComment()`: match article or comment target; [WATCH]/[MUTE] tags; save comment + images
- `handleEditCommentCommand()` / `handleDeleteCommentCommand()`: update/mark-deleted in `comments.json`

### Notification (`processor_notify.go`)

- `notifyReply()`: 3-tier priority check → send plain-text email with thread context
- `collectAncestors()`: walk reply chain up to 4 ancestors, 6000 char limit
- `sendErrorReply()` / `sendDraftReply()`: automated responses

### Shared Utilities (`processor.go`)

- `parseBodyConfig()`: extract `---` delimited key-value block from email body
- `cleanSubject()`: strip Re:/Fwd: prefixes iteratively
- `parseNotifyTag()`: extract [WATCH]/[MUTE] from subject
- `buildEmailMessage()`: construct RFC 2822 plain-text email
- `matchPattern()`: whitelist matching (`*`, exact, `*@domain`)
- `NewSenderFromConfig()`: factory for SMTP sender (eliminates duplicate construction)

## Body Configuration

Articles can declare config at the beginning of the body:

```
---
banner: 2
slug: my-post
notify: on
title: Custom Title
---

Actual body starts here.
```

Detection: opening `---`, known keys only (`banner`, `slug`, `notify`, `title`), closing `---`. Unknown key or invalid format → no config block, entire body is text.

## DKIM Verification (`email/dkim.go`)

1. Extract `DKIM-Signature` header → parse domain, selector, signature
2. DNS TXT query: `<selector>._domainkey.<domain>`
3. RSA PKCS1v15 verify with SHA256

No DKIM header → pass through. Verification fails → reject.

## Image Handling (`email/images.go`)

- `extractImagesFromMultipart()`: recursive image extraction from MIME parts
- `convertToWebp()`: JPEG/PNG → WebP (CGo); registers JPEG and PNG decoders before decoding. GIF/WebP pass through unchanged.
- `saveArticleImages()`: sequential files (`1.webp`, `2.gif`), CID→number map
- `saveCommentImages()`: prefix `c_<commentUID>_<N>.<ext>`

Conversion occurs when a message is received or an article is edited. Existing image files are not retroactively rewritten.

### CID Replacement & Numeric Resolution

1. `replaceCIDInBody()`: converts `![alt](cid:xxx)` → `![alt](1)` and bare `cid:xxx` → `1`
2. `resolveImageNumbers()`: scans article directory, maps `1` → `1.webp` in markdown before saving

## RawMessage Struct

```go
type RawMessage struct {
    SeqNum    uint32
    From      *mail.Address
    To        []*mail.Address
    Subject   string
    Date      string
    MessageID string
    Body      string        // cleaned text/plain
    HTMLBody  string        // extracted HTML (for CID recovery)
    RawBody   []byte        // raw bytes for DKIM
    Images    []ImageData   // extracted attachments
}
```