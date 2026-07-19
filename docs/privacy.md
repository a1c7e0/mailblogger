# Privacy & Email Exposure

## Design Principle

Sender email addresses are stored in backend frontmatter (`author_email`) and SQLite, but their visibility in the frontend is controlled by per-user and global settings.

## Email Visibility Control

Two levels of control:
1. **Global default**: `privacy.hide_email` in config (default: `true`)
2. **Per-user override**: `hide_email` in user preferences (configured via settings page)

Priority: per-user preference > global default.

## What the Frontend Shows

| Element | hide_email=true | hide_email=false |
|---|---|---|
| Author span | Display name | Display name |
| Author tooltip | `hash: abc12345` | `alice@example.com` + `hash: abc12345` (two lines) |
| Reply link | `mailto:<mailbox>+<uid>@domain` | Same (blog's address) |
| Copy button | `<mailbox>+<uid>@domain` | Same (blog's address) |

## What the Backend Stores

`index.md` and `comments.md` frontmatter both include `author_email` for:
- Admin reference
- Notification sending (SMTP)
- Article management authorization (matching sender == article author)
- Hash forwarding (contact author via `blog+hash@domain`)

## Author Identity

- Same email → same `author_hash` (SHA256, 8 hex chars)
- Author hash is visible in frontend tooltips for identity correlation
- Display name comes from email `From: Name <addr>` header (user-controlled)
- No name → hash becomes display name

## Hash Forwarding

Visitors can contact authors without knowing their real email:
- Send email to `blog+<author_hash>@domain`
- System looks up real email via `FindEmailByHash()` in SQLite
- Forwards transparently to `blog+hash@domain` (routes through normal comment flow)
- From/Reply-To: original sender — recipient sees the real person

## Notification Privacy

- Notification emails sent based on 3-tier priority (per-article > per-user > global)
- Same-author replies never trigger notification (don't email yourself)
- Notification From address is blog's own address, not the replier's
- Reply-To routes back to blog, keeping replier's email hidden
- Per-user notification preferences configurable via settings page
