# Privacy & Email Exposure

## Design

Sender emails stored in frontmatter (`author_email`) and SQLite. Frontend visibility controlled by per-user and global settings.

## Visibility Control

1. **Global**: `privacy.hide_email` in config (default: `true`)
2. **Per-user**: `hide_email` in user prefs (via settings page)

Per-user overrides global.

## Frontend Behavior

| Element | hide_email=true | hide_email=false |
|---|---|---|
| Author tooltip | `hash: abc12345` | `alice@example.com\nhash: abc12345` |
| Reply/copy links | Blog's address | Blog's address (same) |

## Backend Storage

`author_email` in frontmatter and SQLite for:
- Notification sending (SMTP)
- Article management auth (sender == author)
- Hash forwarding (`blog+hash@domain`)

## Hash Forwarding

Contact author without knowing real email:
1. Send to `blog+<author_hash>@domain`
2. System looks up real email via `FindEmailByHash()` (SQLite tokens + article history)
3. Forwards transparently to `blog+hash@domain`
4. From/Reply-To: original sender — recipient sees the real person

## Notification Privacy

- From address is blog's own address, not replier's
- Reply-To routes back to blog, keeping replier's email hidden
- Same-author replies never trigger notification