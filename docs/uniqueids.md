# Unique IDs & Author Hashing

## File: `blog/uniqueid.go`

## Hash Function

```go
func hashID(input string) string {
    h := sha256.Sum256([]byte(input))
    return fmt.Sprintf("%x", h)[:8]
}
```

All IDs are 8-character hex strings from SHA256.

## ID Generation

- **Article ID**: `GenUniqueID(messageID)` — from email Message-ID header. Fallback: `from+subject+date`.
- **Comment ID**: `GenUniqueID(messageID + parentID)` — includes parent article/comment ID for deduplication.
- **Author hash**: `GenAuthorHash(email)` — deterministic per email address.

## Display Names

`GenDisplayName(name, email)`: use `From:` name if present, otherwise author hash as fallback.

## Directory Names

Articles stored as `YYYYMMDD_<hash>[_<slug>]`. Hash and slug parsed from directory name:
- `parseDirHash(name)` → `parts[1]`
- `parseDirSlug(name)` → `parts[2]` (empty if none)

## Custom Slugs

Set via body config:
```
---
slug: my-custom-url
---
```

Rules: lowercase letters, digits, dashes; must match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`.
