# Comment Threading

## Model

Comments have a `reply_to` field pointing to the target comment's `uniqueid`. Empty means top-level (reply to article).

Comments are stored in `comments.json` (JSON array) in the article directory. Each comment can have an `edits` array for edit history and a `deleted` flag for marked deletion.

Depth is capped at 1 visual level:
```
comment1              ← top-level (depth 0)
  comment2            ← reply to comment1 (depth 1)
  comment3            ← reply to comment2 (depth 1, capped)
  comment4            ← reply to comment1 (depth 1)
comment5              ← top-level (depth 0)
```

### Rendering Order
In `renderArticleBody()`, comments are grouped so each reply immediately follows its parent comment. The algorithm:
1. Collect all top-level comments (`reply_to` is empty or equals article ID)
2. For each top-level comment, append its replies directly after it
3. Replies at depth 1 are grouped under their parent (by `reply_to` lookup)

This produces a threaded view where replies appear directly beneath the comment they're responding to, rather than in strict chronological order.

## Reply Routing

### New Top-Level Comment
Send to: `<mailbox>+<article_uniqueid>@domain`
- `parseTargetID()` → matches article
- `processComment()` → `parentID = article.UniqueID`, `replyTo = ""`

### Reply to Comment
Send to: `<mailbox>+<comment_uniqueid>@domain`
- `parseTargetID()` → no article match
- `CommentExists()` → finds comment + its parent article
- `processComment()` → `parentID = article`, `replyTo = comment.UniqueID`

## Frontend

### Reply Target Display
In the comment header, if `replyTo != ""`:
```html
<div class="reply-to-info">
  ↳ <a href="#c-<reply_to>" class="reply-target-link"><parent_author_name></a>
    <a href="#c-<reply_to>" class="reply-target-link uid">#<reply_to_uid></a>
</div>
```

Both the name and the uid are clickable `reply-target-link` elements. Clicking scrolls to and highlights the parent comment via `outline: 2px solid #2060a0`.

### Reply Links
Each article and comment has a `[reply]` button generating a `mailto:` link:
```
mailto:<mailbox>+<uniqueid>@<domain>?subject=Re:+<subject>&body=> <quoted body>
```

For comment replies, the body is pre-filled with the parent comment's text (``>`` quoted, max 800 chars).

### Copy Buttons
- `[copy reply address]` — copies `<mailbox>+<uniqueid>@domain` to clipboard
- Clicking `#uniqueid` copies permalink: `/<slug>` or `/<hash>` with optional `#c-<id>` fragment

## Notification Flow

1. Bob replies to Alice's comment → `notifyReply(c, notify)` called
2. If notification enabled (`notify: true` or `[NOTIFY]` in subject):
   - SMTP email sent to Alice's `author_email`
   - `Reply-To: <mailbox>+<new_comment_uid>@domain`
3. Alice opens email, hits "Reply"
4. Email client pre-fills To with the Reply-To address
5. Alice's reply routes to `<mailbox>+<new_comment_uid>@domain`
6. System processes as a reply to Bob's comment
7. Quote stripping removes Alice's email client quoting (`On ... wrote:` lines)

### Same-Author Suppression
`notifyReply()` skips if `parentComment.AuthorEmail == c.AuthorEmail` (don't notify yourself).

## Comment Images

When a comment email includes image attachments, they are saved alongside the article:

- Filenames: `c_<commentUID>_<N>.<ext>` (e.g., `c_92d93709_1.webp`)
- Stored in the same directory as the parent article's `index.md`
- Listed via `store.ListCommentImages(articleID, commentUID)`
- Rendered in the template as clickable filename hyperlinks (e.g., `<a href="...">c_92d93709_1.weba>`)
- NOT rendered inline as `<img>` — clicking the link navigates to the image file

The `commentImages` template function calls `store.ListCommentImages()` and returns the filtered list. The template uses the article's slug or hash for the link `href`.

## Comment Edit/Delete

Subject after stripping `Re:`/`Fwd:` prefixes is matched case-insensitively:
- `edit` → update comment body; old body saved to `edits` array if `history.comment.keep` is true
- `delete` → mark comment as deleted (`deleted: true`); clear `edits` if `history.comment.keep` is false

Non-author emails with subject `edit` or `delete` are treated as regular replies.

## Notification Control

See [Email Processing → Notification Control](email-processing.md#notification-control-notify).
