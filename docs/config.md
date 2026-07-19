# Configuration

## File: `config.yaml`

Excluded from git via `.gitignore`. Template at `config.example.yaml`.

## Structure

```yaml
mail:           # email identity, servers, access control, notifications
  address:      # blog email address (parsed into EmailLocal + EmailDomain)
  imap:         # IMAP server connection
  smtp:         # SMTP server connection (falls back to IMAP credentials)
  whitelist:    # allowed article authors
  notify:       # per-type notification defaults

content_dir:    # article storage directory

site:           # blog appearance and branding
  title:
  subtitle:
  description:
  lang:
  footer_html:
  show_author:
  width:
  links:

web:            # HTTP server settings
  port:
  host:
  scheme:       # URL scheme for feed links (http or https)

privacy:        # email display settings
  hide_email:   # hide author emails in frontend

history:        # version history and deletion behavior
  article:
    keep:       # create edit_N folders on edit; move to _deleted/ on delete
    visible:    # allow web access to edit_N/ paths
  comment:
    keep:       # store edits array in comment JSON
    visible:    # include edits in API responses
  show_deleted: # show "[已删除]" placeholder for deleted comments
  show_replies: # show replies to deleted comments
```

## Fields

### Mail

| Field | Type | Default | Description |
|---|---|---|---|
| `mail.address` | string | — | Email address for the blog. Parsed into `EmailLocal` and `EmailDomain`. |
| `mail.imap.server` | string | — | IMAP server hostname. |
| `mail.imap.port` | int | `993` | IMAP port. |
| `mail.imap.username` | string | — | IMAP login. |
| `mail.imap.password` | string | — | IMAP password. |
| `mail.smtp.server` | string | — | SMTP server hostname. |
| `mail.smtp.port` | int | `465` | SMTP port (implicit TLS). |
| `mail.smtp.username` | string | `imap.username` | SMTP login. Falls back to IMAP. |
| `mail.smtp.password` | string | `imap.password` | SMTP password. Falls back to IMAP. |
| `mail.whitelist` | []string | — | Allowed article authors. Supports `*@domain.com`. |
| `mail.notify.article` | bool | `true` | Default: notify article authors when their articles receive comments. |
| `mail.notify.comment` | bool | `false` | Default: notify comment authors when their comments receive replies. |

### Site

| Field | Type | Default | Description |
|---|---|---|---|
| `site.title` | string | `MailBlogger` | Page title. |
| `site.subtitle` | string | `` | Tagline below title. |
| `site.description` | string | `` | Site description for meta tags, OpenGraph, and Twitter Cards. |
| `site.lang` | string | `en` | HTML `lang` attribute. |
| `site.footer_html` | string | `` | Raw HTML footer (supports `<script>`, `<a>`, etc). |
| `site.show_author` | bool | `true` | Show author on index page. |
| `site.width` | int | `600` | Max content width in pixels. |
| `site.links` | []NavLink | `` | Nav links shown below subtitle. Each has `title` and `url`. |

### Web

| Field | Type | Default | Description |
|---|---|---|---|
| `web.port` | int | `8080` | HTTP listen port. |
| `web.host` | string | `0.0.0.0` | HTTP listen address. |
| `web.scheme` | string | `https` | URL scheme for feed links and generated URLs (`http` or `https`). |

### Privacy

| Field | Type | Default | Description |
|---|---|---|---|
| `privacy.hide_email` | bool | `true` | Hide author email addresses in the frontend. Reply forwarding still works. |

### Other

| Field | Type | Default | Description |
|---|---|---|---|
| `content_dir` | string | `content` | Directory for article storage. |

## Address Parsing

```yaml
mail:
  address: blog@owowo.dev
```

Automatically parsed into:
- `EmailLocal` = `blog`
- `EmailDomain` = `owowo.dev`

The plus-addressing pattern becomes: `blog+<uniqueid>@owowo.dev`

## Notification Preferences

Users can configure per-user notification and privacy preferences via email:
- Send an email with subject `settings` (case-insensitive, after stripping Re:/Fwd:)
- System replies with a time-limited settings link (24h expiry)
- Click link to configure: notification preferences, email visibility

Three-tier notification priority:
1. Per-article overrides (body config `notify:` or `[WATCH]`/`[MUTE]` tags in subject)
2. Per-user preferences (saved via settings page)
3. Global defaults (`mail.notify.article` and `mail.notify.comment`)

Email privacy: per-user `hide_email` overrides `privacy.hide_email` global default.

## Hash Forwarding

Visitors can contact authors by emailing `blog+<author_hash>@domain`. The system forwards the email transparently to the author's real address. This works without exposing the author's email.

## Custom URL (Slug)

Set in body config:
```
---
slug: my-custom-url
---
```
Slug rules: lowercase letters, digits, dashes only; must start and end with alphanumeric.
Accessible at `/<slug>`; hash URL remains at `/<hash>`.

## Avatar & Favicon

Avatar and favicon are auto-detected from the `content/` directory at startup. No config field needed.

### Avatar
Place a file named `avatar.{png,jpg,webp,svg}` in `content/`. Scanned in order: `.png` → `.jpg` → `.webp` → `.svg`. First match wins. Displayed as full-width banner at page top.

### Favicon
- `content/favicon.svg` — used if present (served at `/static/favicon.svg`)
- `content/favicon.ico` — used if present (served at `/favicon.ico`)
- If no favicon file exists and an avatar is detected, both SVG and ICO favicons are generated from the avatar and cached in memory (not written to disk). If no avatar either, no favicon is served (browser uses its default).

## Banner Image

Set in body config to override the auto-detected site avatar with an article image:
```
---
banner: 2
---
```
The number refers to the image attachment order (1 = first image). The selected image replaces the site avatar at the page top (same position, same full-width styling), clickable to view full-size. On SPA navigation, the banner area swaps automatically.

## Body Config `title`

Override article title:
```
---
title: Custom Title
---
```
For new articles, overrides the email subject. For edit commands, overrides the original title.

## Body Config `notify`

Set notification preference for the article:
```
---
notify: on
---
```
Values: `on`/`true`/`watch` → author watches article (notified of comments). `off`/`false`/`mute` → author mutes article.

## Whitelist Patterns

- `*` — allow all
- `alice@example.com` — exact match
- `*@example.com` — any user at example.com
