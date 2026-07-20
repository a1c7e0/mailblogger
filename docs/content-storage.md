# Content Storage

## Directory Layout

```
content/
├── 20260713_afd888d6/                 # article: YYYYMMDD_<hash>
│   ├── index.md                       # frontmatter + body
│   ├── comments.json                  # JSON array of comments
│   ├── 1.webp                         # article image (sequential)
│   ├── 2.gif
│   ├── c_92d93709_1.webp              # comment image (c_<commentUID>_<N>)
│   └── edit_0/                        # history snapshot (if history.article.keep)
│       ├── index.md
│       ├── comments.json
│       └── 1.webp
├── 20260713_afd888d6_hello-world/     # with custom slug
├── _drafts/                           # non-whitelisted senders
├── _deleted/                          # archived deleted articles
└── mailblogger.db                     # SQLite metadata
```

Directory name: `YYYYMMDD_<8-char-hash>[_<slug>]`

## `index.md` Format

```markdown
---
uniqueid: afd888d6
subject: Hello World
author: Alice
author_hash: ff8d9819
author_email: alice@example.com
date: 2026-07-12T12:00:00+09:00
banner: 2
---

Article body in markdown.
```

Seven frontmatter fields (banner optional). Everything after the closing `---` is the body.

## `comments.json` Format

JSON array of comment objects:

```json
[
  {
    "uniqueid": "92d93709",
    "author": "Alice",
    "author_hash": "ff8d9819",
    "author_email": "alice@example.com",
    "date": "2026-07-12T12:00:00Z",
    "reply_to": "",
    "body": "Top-level comment",
    "deleted": false,
    "edits": []
  }
]
```

- `reply_to`: target comment's uniqueid (empty for top-level)
- `deleted`: marked deletion flag
- `edits`: edit history array (when `history.comment.keep` is true)

## Store API (`blog/store.go`)

| Method | Description |
|---|---|
| `NewStore(contentDir)` | Initialize store + SQLite DB |
| `SaveArticle(a)` | Write `index.md` with frontmatter, invalidate cache |
| `SaveComment(c)` | Append to `comments.json` array |
| `SaveDraft(a)` | Write to `_drafts/<uid>/index.md` |
| `GetArticle(hash)` | Read `index.md` by hash (O(1) cache) |
| `GetArticleBySlug(slug)` | Read by slug (O(1) cache) |
| `GetComments(articleID)` | Parse `comments.json` |
| `GetFilteredComments(articleID, opts)` | Filter by ShowDeleted/ShowReplies |
| `ListArticles()` | All articles sorted by date desc |
| `ListArticlesPaged(page, perPage)` | Paginated listing |
| `FindComment(articleID, commentID)` | Find comment within article |
| `CommentExists(commentID)` | Search via comment cache |
| `EditComment(articleID, commentID, newBody)` | Update comment, append to edits |
| `DeleteComment(articleID, commentID)` | Mark as deleted, clear body |
| `DeleteArticle(hash)` | Remove directory permanently |
| `ArchiveArticle(hash)` | Move to `_deleted/` |
| `ArchiveArticleVersion(hash)` | Copy to `edit_N/` before edit |
| `SaveImage(articleID, filename, data)` | Write image to article dir |
| `ListImages(articleID)` | List image files |
| `ListCommentImages(articleID, commentUID)` | List `c_<uid>_*` images |

## SQLite Metadata (`blog/store_sql.go`)

Stores non-content data in `content/mailblogger.db`:

| Table | Purpose |
|---|---|
| `settings_tokens` | Auth tokens for settings page (24h TTL) |
| `user_prefs` | Per-user notification + privacy preferences |
| `article_watchers` | Per-article notification opt-in |
| `article_muters` | Per-article notification opt-out |

## Memory Cache

`sync.Once` pattern: `buildCache()` scans content directory on first access, builds `hashMap`, `slugMap`, `cmtMap`, `articleList`. Invalidated on `SaveArticle()`, `DeleteArticle()`, `ArchiveArticle()`. `sync.RWMutex` for concurrent safety.
