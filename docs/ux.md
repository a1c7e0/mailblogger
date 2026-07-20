# UX & Frontend

## Layout

- Left-aligned, `max-width: 600px`, monospace font stack
- Banner: site avatar or article banner image, full width, flush to container
- Dark mode via `@media (prefers-color-scheme: dark)`
- Blue hierarchy: article links `#1a5c8a` → reply links `#3377bb` → copy links `#5588bb`

## Timestamps

- Server: RFC3339 with timezone, rendered as `<time datetime="...">` elements
- Client: JS converts to local timezone on page load
- Index: date only (`YYYY-MM-DD`); article/comments: datetime with timezone
- Tooltip: `YYYY-MM-DD HH:MM:SS +0000 (Unix: ...)`

## Interactive Elements

- **Reply**: `mailto:` link with pre-filled To, Subject, quoted body (max 1200 chars)
- **Copy address**: `<mailbox>+<uid>@domain` to clipboard, `[copied]` for 1.5s
- **Copy permalink**: `/<slug>#c-<uid>` to clipboard via `#uniqueid` click
- **Comment highlight**: click `↳ author #uid` → outline + scroll to target; `#c-<id>` in URL triggers on load

## Pagination

`?page=N`, 20 articles/page. `← Newer` / `Older →` links.

## Reply Body Format

```
(reply space)


> ---
> Write your reply above this line. Only text above will be saved.
>
> On <date>, <author> (<hash>) wrote:
>
> > <quoted text>
```

All reference lines `>` prefixed for clean `stripEmailQuotes()` separation.

## Images

- **Article images**: sequential files (`1.webp`), referenced as `![alt](1)` (no extension)
- **`<base>` tag**: ensures relative `src="1"` resolves to `/slug/1`
- **wrapImages()**: `<img>` → `<figure>` + `<a target="_blank">` + `<figcaption>`
- **Unreferenced images**: shown in "Attachments" section below article
- **Comment images**: `c_<uid>_<N>.<ext>`, shown as clickable filename links (not inline)
- **Feed images**: relative paths rewritten to absolute URLs

## SPA Navigation (`static/spa.js`)

- Intercepts internal link clicks
- Replaces `<main>` content, swaps banner area
- Updates `<base>` tag and URL via `history.pushState()`
- Header/nav/footer persist across navigations
- External links, `mailto:`, `target="_blank"`, `/static/*`, `/feed*` not intercepted
- Browser back/forward triggers full reload

## Configurable Elements

- `site.lang` — language
- `site.show_author` — show/hide author on index
- `site.width` — max content width
- `site.links` — nav links below subtitle
- `site.auto_lang` — Accept-Language detection for per-language themes
