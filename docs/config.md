# Configuration

## File: `config.yaml`

Excluded from git. Template at `config.example.yaml`.

## Structure

```yaml
mail:
  address: blog@domain.com        # parsed into EmailLocal + EmailDomain
  imap:
    server: imap.domain.com
    port: 993
    username: user
    password: pass
  smtp:
    server: smtp.domain.com       # falls back to IMAP credentials
    port: 465
  whitelist:                       # allowed article authors
    - alice@example.com
    - *@example.com
  notify:
    article: true                 # notify authors about comments
    comment: false                # notify commenters about replies
  dkim: normal                    # none | normal | strict

content_dir: content

site:
  lang: en
  show_author: true
  width: 600
  auto_lang: false
  links:
    - title: About
      url: /about

web:
  port: 8080
  host: 0.0.0.0
  scheme: https

privacy:
  hide_email: true

webhook:
  secret: "random-string"

history:
  article:
    keep: true                    # create edit_N/ on edit, move to _deleted/ on delete
    visible: true                 # allow web access to edit_N/ paths
  comment:
    keep: true                    # store edits array in comment JSON
  show_deleted: true              # show deleted comment placeholders
  show_replies: true              # show replies to deleted comments

theme: default                    # or map of language → theme directory
```

## Defaults

| Field | Default |
|---|---|
| `mail.imap.port` | 993 |
| `mail.smtp.port` | 465 |
| `mail.smtp.username` | = `mail.imap.username` |
| `mail.smtp.password` | = `mail.imap.password` |
| `content_dir` | `content` |
| `site.lang` | `en` |
| `site.show_author` | `true` |
| `site.width` | 600 |
| `web.port` | 8080 |
| `web.host` | `0.0.0.0` |
| `web.scheme` | `https` |
| `privacy.hide_email` | `true` |
| `history.*.keep` | `true` |
| `history.*.visible` | `true` |
| `history.show_deleted` | `true` |
| `history.show_replies` | `true` |
| `mail.dkim` | `normal` |

## Address Parsing

`mail.address: blog@owowo.dev` → `EmailLocal=blog`, `EmailDomain=owowo.dev`

Plus-addressing: `blog+<uniqueid>@owowo.dev`

## Whitelist Patterns

- `*` — allow all
- `alice@example.com` — exact match
- `*@example.com` — any user at domain

Empty whitelist = allow all.

## Theme

Single theme: `theme: default` → serves `themes/default/index.html`

Per-language: `theme: { en: "theme-en", zh: "theme-zh" }` → auto-detect from `Accept-Language` header when `site.auto_lang` is true.

## Hot-Reload

All config fields are hot-reloadable via `fsnotify` on `config.yaml`. IMAP poller picks up new config on next cycle.