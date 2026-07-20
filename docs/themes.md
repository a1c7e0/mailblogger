# Theme System

Themes control the entire frontend. When a theme is configured, all non-API, non-static routes serve the theme's `index.html` instead of server-rendered templates.

## Configuration

```yaml
# Single theme for all languages
theme: default

# Per-language themes (requires site.auto_lang: true)
theme:
  en: theme-en
  zh: theme-zh
```

With `site.auto_lang: true`, the server reads the `Accept-Language` header and serves the matching theme. Unmatched languages fall back to `site.lang`.

## Directory Structure

```
themes/<name>/
├── index.html        # SPA entry point (required)
├── app.js            # Theme logic (required)
├── style.css         # Styles (recommended)
├── theme.json        # Theme metadata (recommended)
└── locales/          # Multi-language strings (optional)
    ├── en.json
    └── zh.json
```

All files in the theme directory are served at the root URL. For example, `themes/default/app.js` is accessible at `/app.js`.

## How It Works

1. Browser requests `/` or `/hello-world`
2. Server serves `themes/<name>/index.html` (the SPA shell)
3. `app.js` loads, fetches data from the JSON API (`/api/site`, `/api/articles`, `/api/article/{id}`, etc.)
4. JavaScript renders the page client-side
5. Internal link clicks are intercepted — `app.js` fetches new data and updates the DOM without full page reload

## theme.json

Theme metadata, loaded by `app.js` at startup.

```json
{
  "title": "My Blog",
  "subtitle": "Email-to-blog",
  "description": "Send an email, it becomes a post.",
  "footer_html": "<p>Powered by MailBlogger</p>"
}
```

All fields are optional. `footer_html` can be inline HTML or a file path (e.g., `"/footer.html"` — fetched via `fetch()`).

## Locale Files

Multi-language string overrides. Loaded in order:

1. `theme.json` — base values
2. `locales/{lang}.json` — language-specific overrides
3. `locales/en.json` — English fallback

Standard keys used by the default theme:

```json
{
  "comments": "Comments",
  "reply": "[reply]",
  "copy_address": "[copy reply address]",
  "deleted": "[deleted]",
  "no_comments": "No comments yet.",
  "no_articles": "No articles yet. Send an email to {{email}} to start blogging.",
  "back_to_index": "← Back to index",
  "edit_history": "Edit history",
  "notify_hint": "Send <code>settings</code> in email subject to configure notifications.",
  "attachments": "Attachments",
  "page_of": "Page {{page}} of {{total}}",
  "newer": "← Newer",
  "older": "Older →",
  "write_reply": "Write your reply above this line. Only text above will be saved."
}
```

Use `{{variable}}` syntax for interpolation. Theme code accesses strings via `t('key', {var: value})`.

## Creating a Custom Theme

### Minimal Example

**index.html:**
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Loading...</title>
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <header id="header"></header>
  <main id="app"></main>
  <footer id="footer"></footer>
  <script src="/app.js"></script>
</body>
</html>
```

**app.js** needs to:
1. Fetch `/api/site` for site configuration
2. Route based on `window.location.pathname`
3. Fetch `/api/articles` for the index page
4. Fetch `/api/article/{id}` and `/api/article/{id}/comments` for article pages
5. Intercept internal link clicks for SPA navigation

### API Data Shapes

**Site** (`GET /api/site`):
```
lang, show_author, avatar, width, links[], email_local, email_domain
```

**Article summary** (`GET /api/articles`):
```
uniqueid, slug, subject, author, author_hash, date, banner, excerpt
```

**Article detail** (`GET /api/article/{id}`):
```
uniqueid, slug, subject, author, author_hash, author_email, date,
banner, body, images[], email_local, email_domain
```

**Comment** (`GET /api/article/{id}/comments`):
```
uniqueid, author, author_hash, author_email, date, reply_to,
body, deleted, edits[]
```

### SPA Navigation Pattern

```javascript
document.addEventListener('click', function(e) {
  const a = e.target.closest('a');
  if (!a) return;
  const href = a.getAttribute('href');
  // Skip external links, mailto, anchors, static files, feeds
  if (href.startsWith('http') || href.startsWith('mailto:') ||
      href.startsWith('#') || href.startsWith('/static/') ||
      href.startsWith('/feed') || href.startsWith('/settings')) return;
  e.preventDefault();
  history.pushState(null, '', href);
  route(); // re-render based on new path
});
window.addEventListener('popstate', route);
```

### Image Handling

Article images are stored as sequential files (`1.webp`, `2.gif`). Reference in markdown as `![alt](1)` (no extension).

For the article page, set `<base href="/<slug-or-id>/">` so relative `src="1"` resolves correctly.

The `images` array in the article API response lists all image filenames. Unreferenced images (not in the markdown body) can be shown in an "Attachments" section.

### Comment Threading

Comments use `reply_to` for threading:
- Empty `reply_to` or `reply_to === article.uniqueid` → top-level comment
- Otherwise → reply to the comment with that `uniqueid`

Display pattern: top-level comments first, each followed immediately by its replies (depth capped at 1 visual level).

## Built-in SSR Fallback

Without a theme configured, MailBlogger uses server-side rendered templates (`web/templates/`). The SSR mode supports all features but requires JavaScript for timezone conversion and SPA navigation (`static/spa.js`).
