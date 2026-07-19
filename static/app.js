// MailBlogger SPA Theme
(function() {
  'use strict';

  let siteData = null;

  // Escape HTML
  function esc(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  // Simple markdown renderer (basic subset)
  function renderMD(text) {
    if (!text) return '';
    let html = esc(text);
    // Images: ![alt](src)
    html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, function(m, alt, src) {
      // Resolve relative src against article base
      if (!src.startsWith('http') && !src.startsWith('/')) {
        src = currentBasePath() + '/' + src;
      }
      return '<figure><a href="' + src + '" target="_blank"><img src="' + src + '" alt="' + alt + '"></a></figure>';
    });
    // Links: [text](url)
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');
    // Bold
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    // Italic
    html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');
    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    // Headings
    html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
    html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
    html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
    // Horizontal rule
    html = html.replace(/^---$/gm, '<hr>');
    // Blockquote
    html = html.replace(/^&gt; (.+)$/gm, '<blockquote>$1</blockquote>');
    // Paragraphs
    html = html.replace(/\n\n/g, '</p><p>');
    html = '<p>' + html + '</p>';
    html = html.replace(/<p><\/p>/g, '');
    // Line breaks
    html = html.replace(/\n/g, '<br>');
    return html;
  }

  // Current article base path for image resolution
  let _basePath = '';
  function currentBasePath() { return _basePath; }

  // Format date
  function fmtDate(dateStr) {
    const d = new Date(dateStr);
    return d.toISOString().split('T')[0] + ' ' + d.toISOString().split('T')[1].substring(0,5) + ' UTC';
  }

  // Build mailto link
  function makeMailto(uniqueid, subject, emailLocal, emailDomain, body, author, date, isComment) {
    const addr = emailLocal + '+' + uniqueid + '@' + emailDomain;
    const subj = 'Re: ' + subject + (isComment ? ' - Comment' : '') + ' #' + uniqueid;
    let mailBody = '\n\n\n> ---\n> Write your reply above this line.\n>\n> On ' + date + ', ' + author + ' wrote:\n';
    if (body) {
      const lines = body.split('\n').slice(0, 10);
      mailBody += '> ' + lines.join('\n> ').substring(0, 800) + '\n';
    }
    return 'mailto:' + addr + '?subject=' + encodeURIComponent(subj) + '&body=' + encodeURIComponent(mailBody);
  }

  // Fetch JSON helper
  async function api(path) {
    const res = await fetch(path);
    if (!res.ok) throw new Error('API error: ' + res.status);
    return res.json();
  }

  // Render header
  async function renderHeader() {
    if (!siteData) siteData = await api('/api/site');
    const s = siteData;
    let html = '';
    if (s.avatar) {
      html += '<div class="header-banner-area"><img src="' + esc(s.avatar) + '" alt="" class="avatar-banner"></div>';
    }
    html += '<h1><a href="/">' + esc(s.title) + '</a></h1>';
    if (s.subtitle) html += '<p class="subtitle">' + esc(s.subtitle) + '</p>';
    if (s.links && s.links.length) {
      html += '<nav class="nav-links">';
      for (const l of s.links) {
        html += '<a href="' + esc(l.url) + '">' + esc(l.title) + '</a>';
      }
      html += '</nav>';
    }
    document.getElementById('header').innerHTML = html;
    document.title = s.title;
  }

  // Render footer
  async function renderFooter() {
    if (!siteData) siteData = await api('/api/site');
    let html = '<p><a href="/">&larr; Back to index</a></p>';
    if (siteData.footer_html) html += siteData.footer_html;
    document.getElementById('footer').innerHTML = html;
  }

  // Article list page
  async function renderIndex() {
    const articles = await api('/api/articles');
    if (!siteData) siteData = await api('/api/site');
    document.title = siteData.title;

    if (!articles || articles.length === 0) {
      document.getElementById('app').innerHTML = '<p class="empty">No articles yet.</p>';
      return;
    }

    let html = '';
    for (const a of articles) {
      const link = a.slug || a.uniqueid;
      html += '<article class="article-summary">';
      html += '<h2><a href="/' + esc(link) + '" class="article-link">' + esc(a.subject) + '</a></h2>';
      html += '<div class="article-meta">';
      if (siteData.show_author) {
        html += '<span class="author">' + esc(a.author) + '</span>';
      }
      html += '<time>' + fmtDate(a.date) + '</time>';
      html += '</div>';
      if (a.excerpt) {
        html += '<p class="excerpt">' + esc(a.excerpt) + '</p>';
      }
      html += '</article>';
    }
    document.getElementById('app').innerHTML = html;
  }

  // Single article page
  async function renderArticle(id) {
    let article;
    try {
      article = await api('/api/article/' + id);
    } catch (e) {
      document.getElementById('app').innerHTML = '<h2>Not Found</h2><p>Article not found.</p>';
      document.title = 'Not Found';
      return;
    }

    const baseSlug = article.slug || article.uniqueid;
    _basePath = '/' + baseSlug;
    if (!siteData) siteData = await api('/api/site');

    document.title = article.subject + ' - ' + siteData.title;

    let html = '<article class="article-full">';
    html += '<h2 class="article-subject">' + esc(article.subject) + '</h2>';
    html += '<div class="article-meta-bar">';
    html += '<span class="author">' + esc(article.author) + '</span>';
    html += '<time>' + fmtDate(article.date) + '</time>';
    html += '<a href="/' + esc(baseSlug) + '" class="uid">#' + esc(article.uniqueid) + '</a>';
    html += '</div>';
    html += '<div class="article-body">' + renderMD(article.body) + '</div>';

    // Unreferenced images
    if (article.images && article.images.length) {
      const refs = (article.body.match(/!\[[^\]]*\]\(([^)]+)\)/g) || []).map(function(m) {
        return m.match(/\]\(([^)]+)\)/)[1].replace(/\.[^.]+$/, '');
      });
      const unreferenced = article.images.filter(function(img) {
        const name = img.replace(/\.[^.]+$/, '');
        return !refs.includes(name) && !img.startsWith('c_');
      });
      if (unreferenced.length) {
        html += '<div class="attachments-section"><hr><h4>Attachments</h4><div class="img-tiles">';
        for (const img of unreferenced) {
          html += '<a href="' + _basePath + '/' + img + '" target="_blank"><img src="' + _basePath + '/' + img + '" width="150" height="150" loading="lazy"></a>';
        }
        html += '</div></div>';
      }
    }
    html += '</article>';

    // Comments section
    html += '<section class="comments-section">';
    const replyAddr = article.email_local + '+' + article.uniqueid + '@' + article.email_domain;
    const mailtoLink = makeMailto(article.uniqueid, article.subject, article.email_local, article.email_domain, article.body, article.author, fmtDate(article.date), false);
    html += '<h3>Comments <a href="' + esc(mailtoLink) + '" class="reply-link">[reply]</a> ';
    html += '<a href="#" class="copy-link" data-address="' + esc(replyAddr) + '">[copy reply address]</a></h3>';

    try {
      const comments = await api('/api/article/' + id + '/comments');
      if (comments && comments.length) {
        html += renderComments(comments, article);
      } else {
        html += '<p class="empty">No comments yet.</p>';
      }
    } catch (e) {
      html += '<p class="empty">No comments yet.</p>';
    }

    html += '<p class="notify-hint">Send <code>settings</code> in email subject to configure your notification preferences.</p>';
    html += '</section>';

    document.getElementById('app').innerHTML = html;

    // Copy link handlers
    document.querySelectorAll('.copy-link').forEach(function(el) {
      el.addEventListener('click', function(e) {
        e.preventDefault();
        const addr = el.getAttribute('data-address');
        navigator.clipboard.writeText(addr).then(function() {
          el.textContent = '[copied!]';
          setTimeout(function() { el.textContent = '[copy reply address]'; }, 2000);
        });
      });
    });
  }

  // Render comments
  function renderComments(comments, article) {
    // Build comment map and organize
    const commentMap = {};
    comments.forEach(function(c) { commentMap[c.uniqueid] = c; });

    const top = [];
    const replies = {};
    comments.forEach(function(c) {
      if (!c.reply_to || c.reply_to === article.uniqueid) {
        top.push(c);
      } else {
        if (!replies[c.reply_to]) replies[c.reply_to] = [];
        replies[c.reply_to].push(c);
      }
    });

    let html = '';
    const rendered = [];
    top.forEach(function(c) {
      rendered.push(c);
      html += renderSingleComment(c, 0, commentMap, article);
      if (replies[c.uniqueid]) {
        replies[c.uniqueid].forEach(function(r) {
          rendered.push(r);
          html += renderSingleComment(r, 1, commentMap, article);
        });
      }
    });
    return html;
  }

  function renderSingleComment(c, depth, commentMap, article) {
    const cls = 'comment' + (depth === 1 ? ' comment-reply' : '') + (c.deleted ? ' comment-deleted' : '');
    const baseSlug = article.slug || article.uniqueid;
    let html = '<div id="c-' + c.uniqueid + '" class="' + cls + '">';
    html += '<div class="comment-header">';
    html += '<span class="author">' + esc(c.author) + '</span>';
    html += '<time>' + fmtDate(c.date) + '</time>';
    html += '<a href="/' + esc(baseSlug) + '#c-' + c.uniqueid + '" class="uid">#' + esc(c.uniqueid) + '</a>';
    if (!c.deleted) {
      const mailtoLink = makeMailto(c.uniqueid, article.subject, article.email_local, article.email_domain, c.body, c.author, fmtDate(c.date), true);
      html += ' <a href="' + esc(mailtoLink) + '" class="reply-link">[reply]</a>';
      const copyAddr = article.email_local + '+' + c.uniqueid + '@' + article.email_domain;
      html += ' <a href="#" class="copy-link" data-address="' + esc(copyAddr) + '">[copy reply address]</a>';
    }
    html += '</div>';
    if (c.reply_to && c.reply_to !== article.uniqueid && commentMap[c.reply_to]) {
      const parent = commentMap[c.reply_to];
      html += '<div class="reply-to-info">&#x21b3; <a href="#c-' + c.reply_to + '">' + esc(parent.author) + '</a>';
      html += ' <a href="#c-' + c.reply_to + '" class="uid">#' + c.reply_to + '</a></div>';
    }
    if (c.deleted) {
      html += '<div class="comment-body comment-deleted-body"><em>[已删除]</em></div>';
    } else {
      html += '<div class="comment-body">' + esc(c.body).replace(/\n/g, '<br>') + '</div>';
    }
    if (c.edits && c.edits.length) {
      html += '<details class="comment-edits"><summary>Edit history (' + c.edits.length + ')</summary>';
      c.edits.forEach(function(e) {
        html += '<div class="edit-entry"><time>' + fmtDate(e.date) + '</time>';
        html += '<div class="edit-body">' + esc(e.body).replace(/\n/g, '<br>') + '</div></div>';
      });
      html += '</details>';
    }
    html += '</div>';
    return html;
  }

  // Client-side routing
  function getPath() {
    return window.location.pathname;
  }

  async function route() {
    const path = getPath();
    await renderHeader();
    await renderFooter();

    if (path === '/' || path === '') {
      await renderIndex();
    } else {
      const id = path.replace(/^\//, '').replace(/\/$/, '');
      await renderArticle(id);
    }
  }

  // SPA link interception
  document.addEventListener('click', function(e) {
    const a = e.target.closest('a');
    if (!a) return;
    const href = a.getAttribute('href');
    if (!href) return;
    // Only intercept internal links
    if (href.startsWith('http') || href.startsWith('mailto:') || href.startsWith('#') || href.startsWith('/static/') || href.startsWith('/feed') || href.startsWith('/sitemap') || href.startsWith('/settings')) return;
    e.preventDefault();
    history.pushState(null, '', href);
    route();
  });

  // Handle back/forward
  window.addEventListener('popstate', route);

  // Initial load
  route();
})();
