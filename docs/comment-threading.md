# Comment Threading

## Model

Comments stored in `comments.json` (JSON array) per article. `reply_to` field points to target comment's `uniqueid`; empty = top-level reply to article.

Visual depth capped at 1:

```
comment1              ← depth 0
  comment2            ← depth 1 (reply to comment1)
  comment3            ← depth 1 (reply to comment2, capped)
comment4              ← depth 0
```

## Rendering Order

`renderArticleBodyWithComments()` groups comments so replies follow their parent:

1. Collect top-level comments (`reply_to` empty or = article ID)
2. For each top-level, append its replies immediately after
3. Replies at depth 1 grouped by `reply_to` lookup

## Reply Routing

**Top-level comment**: send to `<mailbox>+<article_uniqueid>@domain`
→ `parseTargetID()` matches article → `processComment()` with `replyTo=""`

**Reply to comment**: send to `<mailbox>+<comment_uniqueid>@domain`
→ `CommentExists()` finds comment + parent article → `processComment()` with `replyTo=comment.UniqueID`

## Frontend

- Reply target: `↳ <author> #<uid>` link, scrolls + highlights parent comment
- Reply button: `mailto:` link with pre-filled To, Subject, quoted body
- Copy address: `<mailbox>+<uniqueid>@domain` to clipboard
- Copy permalink: `/<slug>#c-<uid>` or `/<hash>#c-<uid>`

## Comment Images

Saved as `c_<commentUID>_<N>.<ext>` in article directory. Displayed as clickable filename hyperlinks below comment body (not inline).

## Edit/Delete

Subject after stripping `Re:`/`Fwd:` (case-insensitive):
- `edit` → update body; old body saved to `edits` array if `history.comment.keep`
- `delete` → mark `deleted: true`, clear body; clear `edits` if `!history.comment.keep`

Non-author emails treated as regular replies.