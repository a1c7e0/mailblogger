// MailBlogger Default Theme
(function() {
  'use strict';

  let siteData = null;
  let themeData = null;
  let locale = {};

  async function loadTheme() {
    try {
      const res = await fetch('/theme.json');
      if (res.ok) themeData = await res.json();
    } catch (e) { themeData = {}; }
  }

  async function loadLocale(lang) {
    locale = Object.assign({}, themeData);
    try {
      const res = await fetch('/locales/' + lang + '.json');
      if (res.ok) {
        const strings = await res.json();
        locale = Object.assign(locale, strings);
      }
    } catch (e) {}
    if (lang !== 'en') {
      try {
        const res = await fetch('/locales/en.json');
        if (res.ok) {
          const strings = await res.json();
          locale = Object.assign({}, strings, locale);
        }
      } catch (e) {}
    }
  }

  function t(key, vars) {
    let s = locale[key] || key;
    if (vars) {
      for (const [k, v] of Object.entries(vars)) {
        s = s.replace('{{' + k + '}}', v);
      }
    }
    return s;
  }

  function esc(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  function renderMD(text) {
    if (!text) return '';
    let html = esc(text);
    html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, function(m, alt, src) {
      if (!src.startsWith('http') && !src.startsWith('/')) {
        src = currentBasePath() + '/' + src;
      }
      return '<figure><a href="' + src + '" target="_blank" rel="noopener"><img src="' + src + '" alt="' + alt + '"></a>' + (alt ? '<figcaption>' + alt + '</figcaption>' : '') + '</figure>';
    });
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, function(m, text, url) {
      var isExternal = url.startsWith('http://') || url.startsWith('https://');
      return '<a href="' + url + '"' + (isExternal ? ' target="_blank"' : '') + '>' + text + '</a>';
    });
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
    html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
    html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
    html = html.replace(/^---$/gm, '<hr>');
    html = html.replace(/^&gt; (.+)$/gm, '<blockquote>$1</blockquote>');
    html = html.replace(/\n\n/g, '</p><p>');
    html = '<p>' + html + '</p>';
    html = html.replace(/<p><\/p>/g, '');
    html = html.replace(/\n/g, '<br>');
    return html;
  }

  let _basePath = '';
  function currentBasePath() { return _basePath; }

  function fmtDate(dateStr) {
    const d = new Date(dateStr);
    const pad = function(n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function fmtDateShort(dateStr) {
    const d = new Date(dateStr);
    const pad = function(n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate());
  }

  function datetimeISO(dateStr) {
    return new Date(dateStr).toISOString().replace(/\.\\d+Z$/, 'Z');
  }

  function getTzOffset(dateStr) {
    if (dateStr.endsWith('Z')) return '+00:00';
    var m = dateStr.match(/([+-]\\d{2}:\\d{2})$/);
    return m ? m[1] : null;
  }

  function fmtDateInTz(dateStr, tzOffset) {
    var d = new Date(dateStr);
    var m = tzOffset.match(/([+-])(\\d{2}):(\\d{2})/);
    if (!m) return fmtDate(dateStr);
    var sign = m[1] === '+' ? 1 : -1;
    var offsetMs = sign * (parseInt(m[2]) * 3600 + parseInt(m[3]) * 60) * 1000;
    var shifted = new Date(d.getTime() + offsetMs);
    var pad = function(n) { return String(n).padStart(2, '0'); };
    return shifted.getUTCFullYear() + '-' + pad(shifted.getUTCMonth()+1) + '-' + pad(shifted.getUTCDate())
         + ' ' + pad(shifted.getUTCHours()) + ':' + pad(shifted.getUTCMinutes());
  }

  function authorTooltip(hash, email) {
    return email + '\nhash: ' + hash;
  }

  function makeMailto(uniqueid, subject, emailLocal, emailDomain, body, author, date, isComment) {
    const addr = emailLocal + '+' + uniqueid + '@' + emailDomain;
    const subj = 'Re: ' + subject + (isComment ? ' - Comment' : '') + ' #' + uniqueid;
    let mailBody = '\n\n\n> ---\n> ' + t('write_reply') + '\n>\n> On ' + date + ', ' + author + ' wrote:\n';
    if (body) {
      const lines = body.split('\n').slice(0, 10);
      mailBody += '> ' + lines.join('\n> ').substring(0, 800) + '\n';
    }
    return 'mailto:' + addr + '?subject=' + encodeURIComponent(subj) + '&body=' + encodeURIComponent(mailBody);
  }

  async function api(path) {
    const res = await fetch(path);
    if (!res.ok) throw new Error('API error: ' + res.status);
    return res.json();
  }

  async function renderHeader() {
    if (!siteData) siteData = await api('/api/site');
    const s = siteData;
    let html = '<div class="header-banner-area">';
    if (s.avatar) {
      html += '<img src="' + esc(s.avatar) + '" alt="" class="avatar-banner">';
    }
    html += '</div>';
    html += '<h1><a href="/">' + esc(locale.title || 'Blog') + '</a></h1>';
    if (locale.subtitle) html += '<p class="subtitle">' + esc(locale.subtitle) + '</p>';
    if (s.links && s.links.length) {
      html += '<nav class="nav-links">';
      for (const l of s.links) {
        html += '<a href="' + esc(l.URL) + '">' + esc(l.Title) + '</a>';
      }
      html += '</nav>';
    }
    document.getElementById('header').innerHTML = html;
    document.title = locale.title || 'Blog';
    document.documentElement.lang = s.lang || 'en';
  }

  async function renderFooter() {
    if (!siteData) siteData = await api('/api/site');
    let html = '<p><a href="/">' + t('back_to_index') + '</a></p>';
    const footer = locale.footer_html || '';
    if (footer) {
      if (footer.startsWith('/')) {
        try {
          const res = await fetch(footer);
          if (res.ok) html += await res.text();
        } catch (e) {}
      } else {
        html += footer;
      }
    }
    document.getElementById('footer').innerHTML = html;
  }

  async function renderIndex() {
    const articles = await api('/api/articles');
    if (!siteData) siteData = await api('/api/site');
    document.title = locale.title || 'Blog';

    let html = '';
    if (!articles || articles.length === 0) {
      const email = siteData.email_local + '@' + siteData.email_domain;
      html = '<table class="article-list"><tbody><tr><td colspan="2" class="empty">' + t('no_articles', {email: email}) + '</td></tr></tbody></table>';
    } else {
      html = '<table class="article-list"><tbody>';
      for (const a of articles) {
        const link = a.slug || a.uniqueid;
        html += '<tr>';
        var tz = getTzOffset(a.date) || '';
        html += '<td class="article-date"><time datetime="' + datetimeISO(a.date) + '" data-orig="' + esc(a.date) + '" data-tz="' + esc(tz) + '">' + fmtDateShort(a.date) + '</time></td>';
        html += '<td class="article-info">';
        html += '<a href="/' + esc(link) + '" class="article-link">' + esc(a.subject) + '</a>';
        html += '<span class="article-meta">';
        if (siteData.show_author) {
          html += ' <span class="author" title="' + esc(authorTooltip(a.author_hash, '')) + '">' + esc(a.author) + '</span>';
        }
        html += ' <a href="#" class="uid" data-address="/' + esc(link) + '">#' + esc(a.uniqueid) + '</a>';
        html += '</span>';
        html += '</td></tr>';
      }
      html += '</tbody></table>';
    }
    document.getElementById('app').innerHTML = html;
    initCopyLinks();
    initTimeHover();
  }

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

    document.title = article.subject + ' - ' + (locale.title || 'Blog');

    // Update banner if article has one
    const bannerArea = document.querySelector('.header-banner-area');
    if (bannerArea && article.banner) {
      bannerArea.innerHTML = '<a href="' + _basePath + '/' + article.banner + '" target="_blank" rel="noopener"><img src="' + _basePath + '/' + article.banner + '" alt="" class="avatar-banner"></a>';
    }

    let html = '<article class="article-full">';
    html += '<h2 class="article-subject">' + esc(article.subject) + '</h2>';
    html += '<div class="article-meta-bar">';
    html += '<span class="author" title="' + esc(authorTooltip(article.author_hash, article.author_email)) + '">' + esc(article.author) + '</span>';
    var tz = getTzOffset(article.date) || '';
    html += '<time class="date" datetime="' + datetimeISO(article.date) + '" data-orig="' + esc(article.date) + '" data-tz="' + esc(tz) + '">' + fmtDate(article.date) + '</time>';
    html += '<a href="/' + esc(baseSlug) + '" class="uid" title="permalink" data-address="/' + esc(baseSlug) + '">#' + esc(article.uniqueid) + '</a>';
    html += '</div>';
    html += '<div class="article-body">' + renderMD(article.body) + '</div>';

    if (article.images && article.images.length) {
      const refs = (article.body.match(/!\[[^\]]*\]\(([^)]+)\)/g) || []).map(function(m) {
        return m.match(/\]\(([^)]+)\)/)[1].replace(/\.[^.]+$/, '');
      });
      const unreferenced = article.images.filter(function(img) {
        const name = img.replace(/\.[^.]+$/, '');
        return !refs.includes(name) && !img.startsWith('c_');
      });
      if (unreferenced.length) {
        html += '<div class="attachments-section"><hr><h4 class="attachments-title">' + t('attachments') + '</h4><div class="img-tiles">';
        for (const img of unreferenced) {
          html += '<a href="' + _basePath + '/' + img + '" target="_blank" rel="noopener"><img src="' + _basePath + '/' + img + '" width="150" height="150" loading="lazy"></a>';
        }
        html += '</div></div>';
      }
    }
    html += '</article>';

    // Comments section
    html += '<section class="comments-section">';
    const replyAddr = article.email_local + '+' + article.uniqueid + '@' + article.email_domain;
    const mailtoLink = makeMailto(article.uniqueid, article.subject, article.email_local, article.email_domain, article.body, article.author, fmtDate(article.date), false);
    html += '<h3>' + t('comments') + ' <a href="' + esc(mailtoLink) + '" class="reply-link">' + t('reply') + '</a>';
    html += ' <a href="#" class="copy-link" data-address="' + esc(replyAddr) + '">' + t('copy_address') + '</a></h3>';

    try {
      const comments = await api('/api/article/' + id + '/comments');
      if (comments && comments.length) {
        html += renderComments(comments, article);
      } else {
        html += '<p class="empty">' + t('no_comments') + '</p>';
      }
    } catch (e) {
      html += '<p class="empty">' + t('no_comments') + '</p>';
    }

    html += '<p class="notify-hint">' + t('notify_hint') + '</p>';
    html += '</section>';

    document.getElementById('app').innerHTML = html;
    initCopyLinks();
    initReplyTargetLinks();
    highlightHash();
    initTimeHover();
  }

  function renderComments(comments, article) {
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
    top.forEach(function(c) {
      html += renderSingleComment(c, 0, commentMap, article);
      if (replies[c.uniqueid]) {
        replies[c.uniqueid].forEach(function(r) {
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
    html += '<span class="author" title="' + esc(authorTooltip(c.author_hash, c.author_email)) + '">' + esc(c.author) + '</span>';
    var ctz = getTzOffset(c.date) || '';
    html += '<time class="date" datetime="' + datetimeISO(c.date) + '" data-orig="' + esc(c.date) + '" data-tz="' + esc(ctz) + '">' + fmtDate(c.date) + '</time>';
    html += ' <a href="/' + esc(baseSlug) + '#c-' + c.uniqueid + '" class="uid" title="permalink" data-address="/' + esc(baseSlug) + '#c-' + c.uniqueid + '">#' + esc(c.uniqueid) + '</a>';
    if (!c.deleted) {
      const mailtoLink = makeMailto(c.uniqueid, article.subject, article.email_local, article.email_domain, c.body, c.author, fmtDate(c.date), true);
      html += ' <a href="' + esc(mailtoLink) + '" class="reply-link">' + t('reply') + '</a>';
      const copyAddr = article.email_local + '+' + c.uniqueid + '@' + article.email_domain;
      html += ' <a href="#" class="copy-link" data-address="' + esc(copyAddr) + '">' + t('copy_address') + '</a>';
    }
    html += '</div>';
    if (c.reply_to && c.reply_to !== article.uniqueid && commentMap[c.reply_to]) {
      const parent = commentMap[c.reply_to];
      html += '<div class="reply-to-info">&#x21b3; <a href="#c-' + c.reply_to + '" class="reply-target-link">' + esc(parent.author) + '</a>';
      html += ' <a href="#c-' + c.reply_to + '" class="reply-target-link uid">#' + c.reply_to + '</a></div>';
    }
    if (c.deleted) {
      html += '<div class="comment-body comment-deleted-body"><em>' + t('deleted') + '</em></div>';
    } else {
      html += '<div class="comment-body">' + esc(c.body).replace(/\n/g, '<br>') + '</div>';
    }
    if (c.edits && c.edits.length) {
      html += '<details class="comment-edits"><summary>' + t('edit_history') + ' (' + c.edits.length + ')</summary>';
      c.edits.forEach(function(e) {
        var etz = getTzOffset(e.date) || '';
        html += '<div class="edit-entry"><time data-orig="' + esc(e.date) + '" data-tz="' + esc(etz) + '">' + fmtDate(e.date) + '</time>';
        html += '<div class="edit-body">' + esc(e.body).replace(/\n/g, '<br>') + '</div></div>';
      });
      html += '</details>';
    }
    html += '</div>';
    return html;
  }

  function initCopyLinks() {
    document.querySelectorAll('.copy-link,.uid[data-address]').forEach(function(btn) {
      btn.addEventListener('click', function(e) {
        e.preventDefault();
        e.stopPropagation();
        const addr = this.getAttribute('data-address');
        navigator.clipboard.writeText(addr).then(function() {
          const orig = btn.textContent;
          btn.textContent = '[copied]';
          setTimeout(function() { btn.textContent = orig; }, 1500);
        }).catch(function() {
          const orig = btn.textContent;
          btn.textContent = '[error]';
          setTimeout(function() { btn.textContent = orig; }, 1500);
        });
      });
    });
  }

  function initReplyTargetLinks() {
    document.querySelectorAll('.reply-target-link').forEach(function(link) {
      link.addEventListener('click', function(e) {
        e.preventDefault();
        const hash = this.getAttribute('href');
        const target = document.querySelector(hash);
        if (target) {
          document.querySelectorAll('.comment').forEach(function(c) { c.classList.remove('highlight'); });
          target.classList.add('highlight');
          target.scrollIntoView({behavior: 'smooth', block: 'center'});
        }
      });
    });
  }

  function initTimeHover() {
    document.querySelectorAll('time[data-tz]').forEach(function(el) {
      var origDatetime = el.getAttribute('data-orig');
      var tz = el.getAttribute('data-tz');
      if (!tz || !origDatetime) return;
      var localText = el.textContent;
      var publisherText = fmtDateInTz(origDatetime, tz);
      if (localText === publisherText) return;
      el.style.cursor = 'help';
      el.addEventListener('mouseenter', function() { el.textContent = publisherText; });
      el.addEventListener('mouseleave', function() { el.textContent = localText; });
    });
  }

  function highlightHash() {
    if (window.location.hash) {
      const target = document.querySelector(window.location.hash);
      if (target) {
        target.classList.add('highlight');
        setTimeout(function() { target.scrollIntoView({block: 'center'}); }, 100);
      }
    }
  }

  function getPath() {
    return window.location.pathname;
  }

  async function route() {
    const path = getPath();
    if (!siteData) siteData = await api('/api/site');
    await loadTheme();
    await loadLocale(siteData.lang || 'en');
    await renderHeader();
    await renderFooter();

    if (path === '/' || path === '') {
      await renderIndex();
    } else {
      const id = path.replace(/^\//, '').replace(/\/$/, '');
      await renderArticle(id);
    }
  }

  document.addEventListener('click', function(e) {
    const a = e.target.closest('a');
    if (!a) return;
    const href = a.getAttribute('href');
    if (!href) return;
    if (href.startsWith('http') || href.startsWith('mailto:') || href.startsWith('#') || href.startsWith('/static/') || href.startsWith('/feed') || href.startsWith('/sitemap') || href.startsWith('/settings')) return;
    e.preventDefault();
    history.pushState(null, '', href);
    route();
  });

  window.addEventListener('popstate', route);
  route();
})();
