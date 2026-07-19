# UX & Frontend

## Page Design

- **Layout**: left-aligned, `max-width: 600px`, monospace font stack
- **Banner**: site avatar at page top, full `600px` width, flush to container edges (offset padding via negative margins). Auto-detected from `content/avatar.{png,jpg,webp,svg}`. Articles with `banner` in body config replace the avatar with the specified image (same position, same styling).
- **Nav links**: configured via `site.links`, displayed as a flex row with `gap: 14px` between items.
- **Styling**: lore.kernel.org inspired — minimal borders, no shadows, flat design
- **Dark mode**: `@media (prefers-color-scheme: dark)` with dark backgrounds and lighter text
- **Blue hierarchy**: article links `#1a5c8a` → reply links `#3377bb` → copy links `#5588bb`

## Timestamps

### Server-Side
Dates stored as RFC3339 with timezone. Rendered as `<time datetime="ISO8601">` elements.

### Client-Side
JavaScript on page load converts all `<time>` elements to client local timezone:
- Index page: date only (`YYYY-MM-DD`) via `data-format="date"`
- Article/comments: datetime with timezone (`YYYY-MM-DD HH:MM ±TZOFF`)

### Fallback
If JavaScript disabled, dates display as UTC.

### Tooltip
Hovering any date shows: `YYYY-MM-DD HH:MM:SS +0000 (Unix: 1234567890)`

## Interactive Elements

### Reply
`mailto:` link opens default email client with To and Subject pre-filled. Subject format: `Re: <article> - Comment #<uid>`.

For comment replies, body is pre-filled with:
```
(blank space for reply)


> ---
> Write your reply above this line. Only text above will be saved.
>
> On <date>, <author> (<hash>) wrote:
>
> > <quoted text>
```

All reference lines have `>` prefix so `stripEmailQuotes()` cleanly separates user input. Spaces use `%20` (not `+`). User writes above the first `>` line; everything below gets discarded.

### Copy Reply Address
Copies `<mailbox>+<uniqueid>@domain` to clipboard. Changes to `[copied]` for 1.5s.

### Copy Permalink via UID
Clicking any `#uniqueid` copies the permalink to clipboard:
- Index / article top (no slug): `http://host/<hash>`
- Index / article top (with slug): `http://host/<slug>`
- Comment: `http://host/<hash-or-slug>#c-<comment_uid>`

Text changes to `[copied]` for 1.5s after clicking.

### Comment Highlighting
Clicking `↳ <author> #<uid>` link:
1. Removes `.highlight` from all comments
2. Adds `.highlight` to target comment (`outline: 2px solid blue`)
3. Scrolls to target smoothly
4. URL hash (`#c-<id>`) also triggers highlight on page load

## Pagination

- `?page=N` query parameter, 20 articles per page
- Previous/Next links with `← Newer` / `Older →`
- Shows `Page N of M` between navigation links
- Placed above footer content, below footer border

## Notification Hint

Below comments section, a small hint indicates current notification status:
- `notify: false` → "Notifies are off — add [NOTIFY] in subject to enable."
- `notify: true` → "Notifies are on — add [MUTE] in subject to suppress."

## Responsive Design

- `max-width: 720px` container
- `font-size: 14px` base
- Tables collapse for small screens (natural overflow)
- Copy/paste friendly layout

## Configurable Elements

- `site.title` — header text
- `site.subtitle` — tagline below header
- `site.footer_html` — raw HTML footer (supports links, scripts, etc.)
- `site.show_author` — show/hide author on index page

## Images

### Article Images
- Images attached to email articles are saved as sequential files: `1.webp`, `2.gif`, etc.
- Referenced in markdown as `![alt](1)` — extension not required
- `ensureImageBreaks()` ensures blank lines around every image reference, so each renders on its own line
- `wrapImages()` post-processes rendered HTML: each `<img>` is wrapped in `<figure>` (centered), `<a target="_blank" rel="noopener">` (clickable, opens in new tab), and `<figcaption>` (displays alt text below image)
- Max height 400px, max width 100%
- Unreferenced images shown in "Attachments" section below article body, separated by `<hr>` and `<h4>` heading

### Comment Images
- Saved as `c_<commentUID>_<N>.<ext>` in the parent article's directory
- Displayed as clickable filename hyperlinks (`target="_blank"`) below the comment body
- NOT rendered inline — clicking the link opens the image file in a new tab

### `<base>` Tag
Article pages include `<base href="/slug-or-id/">` in `<head>`. This ensures relative `img src` values (e.g., `src="1"`) resolve to `/slug-or-id/1` instead of `/1`. Updated dynamically during SPA navigation.

### Feed Images
Atom feeds rewrite relative `<img src>` to absolute URLs (`http://host/articleID/filename`) so images display correctly in feed readers.

## SPA Navigation

Internal link clicks are intercepted by `static/spa.js`:
- `<main>` content is replaced; header title/nav and footer (music player) persist
- Banner area (`.header-banner-area`) is swapped: avatar shows on index, article banner shows on articles with banner
- `<base>` tag and URL updated via `history.pushState()`
- `initPage()` re-binds event handlers after content replacement
- External links, `mailto:`, `target="_blank"`, `/static/*`, `/feed*` are not intercepted
- Browser back/forward triggers full reload
