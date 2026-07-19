# Content Storage

## Directory Layout

```
content/
├── 20260713_afd888d6/             # article: YYYYMMDD_<hash>
│   ├── index.md
│   ├── comments.md
│   ├── 1.webp                     # article image (sequential)
│   ├── 2.gif
│   ├── c_92d93709_1.webp          # comment image (c_<commentUID>_<N>)
│   └── c_92d93709_2.png
├── 20260713_afd888d6_hello-world/ # with custom slug
│   ├── index.md
│   ├── comments.md
│   └── 1.webp
└── _drafts/
    └── ...                         # non-whitelisted senders
```

Directory name format: `YYYYMMDD_<8-char-hash>[_<slug>]`

- `parseDirHash(name)` → `parts[1]` = hash
- `parseDirSlug(name)` → `parts[2]` = slug (empty if none)

## `index.md` Format (Article)

```markdown
---
uniqueid: afd888d6
subject: Hello World
author: Alice
author_hash: ff8d9819
author_email: alice@example.com
date: 2026-07-12T12:00:00+09:00
---

Article body in markdown.
```

Six frontmatter fields. Everything after `---` / `---` is the body.

## `comments.md` Format (Comments)

Each comment is a self-contained YAML frontmatter block. Blocks are separated by a newline before the next `---`.

```markdown
---
uniqueid: 92d93709
author: Alice
author_hash: ff8d9819
author_email: alice@example.com
date: 2026-07-12T12:00:00+09:00
reply_to: ""
---

Top-level comment body.

---
uniqueid: 82f44013
author: Bob
author_hash: 2b3b2b9c
author_email: bob@example.com
date: 2026-07-12T12:01:00+09:00
reply_to: 92d93709
---

Reply to Alice's comment.
```

Six fields per comment. `reply_to` is the target comment's `uniqueid`, empty for top-level comments.

## Store API (`blog/store.go`)

| Method | Description |
|---|---|
| `NewStore(contentDir)` | Initialize store, create dir if needed |
| `SaveArticle(a)` | Write `content/<uid>/index.md` |
| `SaveComment(c)` | Append to `content/<parent>/comments.md` |
| `SaveDraft(a)` | Write to `content/_drafts/<uid>/index.md` |
| `GetArticle(uid)` | Parse `index.md` frontmatter + body |
| `GetArticleBySlug(slug)` | Lookup by slug directory name, O(1) cache |
| `FindByAlias(slug)` | Alias for GetArticleBySlug |
| `GetComments(articleID)` | Parse all blocks from `comments.md` |
| `ListArticles()` | All articles sorted by date desc |
| `ListArticlesPaged(page, perPage)` | Paginated listing, returns slice + total count |
| `FindByUniqueID(uid)` | Alias for GetArticle |
| `FindComment(articleID, commentID)` | Find a comment within an article |
| `FindCommentByID(commentID)` | Search all articles for a comment |
| `ArticleExists(uid)` | Check directory exists |
| `CommentExists(commentID)` | Search all articles for comment |
| `DeleteArticle(uid)` | Remove entire article directory |
| `SaveImage(articleID, filename, data)` | Write image file to article directory |
| `ListImages(articleID)` | List all image files (`.jpg`, `.jpeg`, `.png`, `.webp`, `.gif`) in article dir |
| `ListCommentImages(articleID, commentUID)` | List images with prefix `c_<commentUID>_` |
| `GetArticleDirName(a)` | Return and create article directory path |

## Memory Cache

The Store maintains an in-memory `hashMap` and `slugMap` mapping `directory name` → `hash`/`slug`. `buildCache()` is called lazily on first lookup and invalidated on `SaveArticle()` or `DeleteArticle()`. All `findByHash()` and `findBySlug()` calls are O(1) after cache warmup.

Cache invalidation resets the maps to `nil`; the next request triggers a single `os.ReadDir()` scan to rebuild. `sync.RWMutex` with double-checked locking ensures concurrent safety.

## Frontmatter Parsing

- `parseFrontmatter(data)`: finds `---\n` at start, `\n---\n` as closing delimiter, returns `(map[string]string, body, error)`
- `splitCommentBlocks(data)`: state machine with YAML-lookahead to distinguish `---` horizontal rules from frontmatter boundaries
- `getStr(m, key)`: safe map access returning empty string on nil/missing key
