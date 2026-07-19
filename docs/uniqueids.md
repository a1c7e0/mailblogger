# Unique IDs & Author Hashing

## File: `blog/uniqueid.go`

## Unique IDs

Every article and comment gets an 8-character hex unique ID.

### Generation
Internal helper:
```go
func hashID(input string) string {
    h := sha256.Sum256([]byte(input))
    return fmt.Sprintf("%x", h)[:8]
}
```

Public wrappers `GenUniqueID(input)` and `GenAuthorHash(email)` both call `hashID`.

### Article IDs
`input = raw.MessageID` (the Message-ID header from the email).

Fallback if Message-ID is empty: `from + subject + date`.

### Comment IDs
`input = raw.MessageID + parentID` (parent is either article or comment uniqueid).

### Article Directory Name
Articles are stored as `YYYYMMDD_<hash>[_<slug>]`. The slug is extracted from the directory name at load time, sorted by date prefix.

### Custom Slug
Add `[SLUG:my-slug]` in email subject to set a custom URL. Slug must match `^[a-z0-9][a-z0-9-]*[a-z0-9]$`.

This ensures deduplication: same Message-ID + same parent = same unique ID, preventing re-processing of the same reply.

## Author Hashing

### Generation
```go
func GenAuthorHash(email string) string {
    return hashID(email)
}
```

Takes the raw email address (e.g., `alice@example.com`), hashes via `hashID`, returns first 8 hex chars.

### Purpose
- Links same-person comments across articles without revealing email
- Displayed in frontend via `title="hash: <author_hash>"` on author spans
- Used for same-author notification suppression in `notifyReply()`

## Display Names

```go
func GenDisplayName(name, email string) string {
    if name != "" {
        return name
    }
    return GenAuthorHash(email)
}
```

- `From: Alice <a@b.com>` → display `Alice`
- `From: a@b.com` (no name) → display `GenAuthorHash(email)` as fallback

## Privacy

See [Privacy & Email Exposure](privacy.md).
