// MailBlogger Default Theme
(function() {
  'use strict';

  let siteData = null;
  let themeData = null;
  let locale = {};

  // --- API cache ---
  var apiCache = {};
  var articlesCache = null;
  var articlesCacheTime = 0;

  // --- Available locales (expand as new locale files are added) ---
  var AVAILABLE_LOCALES = ['en', 'zh'];

  // --- Language helpers ---
  function detectBrowserLang() {
    var raw = (navigator.language || navigator.userLanguage || 'en').toLowerCase();
    var code = raw.split('-')[0];
    return AVAILABLE_LOCALES.indexOf(code) !== -1 ? code : 'en';
  }

  function getLang() {
    var stored = localStorage.getItem('lang');
    if (stored && AVAILABLE_LOCALES.indexOf(stored) !== -1) return stored;
    return detectBrowserLang();
  }

  function setLang(lang) {
    if (lang === detectBrowserLang()) {
      localStorage.removeItem('lang');
    } else {
      localStorage.setItem('lang', lang);
    }
  }

  // --- Theme (color scheme) helpers ---
  function getScheme() {
    return localStorage.getItem('color-scheme') || 'auto';
  }

  function setScheme(scheme) {
    if (scheme === 'auto') {
      localStorage.removeItem('color-scheme');
    } else {
      localStorage.setItem('color-scheme', scheme);
    }
    applyScheme(scheme);
  }

  function applyScheme(scheme) {
    var el = document.documentElement;
    if (scheme === 'auto') {
      el.removeAttribute('data-theme');
    } else {
      el.setAttribute('data-theme', scheme);
    }
  }

  // --- Theme data (merged into /api/site by backend) ---
  function loadTheme() {
    themeData = siteData || {};
  }

  // --- Locale loading (backend merges en fallback) ---
  async function loadLocale(lang) {
    try {
      var res = await api('/api/locale?lang=' + encodeURIComponent(lang));
      locale = Object.assign({}, themeData, res);
    } catch (e) {
      locale = Object.assign({}, themeData);
    }
  }

  function t(key, vars) {
    var s = locale[key] || key;
    if (vars) {
      for (var k in vars) {
        if (vars.hasOwnProperty(k)) {
          s = s.replace('{{' + k + '}}', vars[k]);
        }
      }
    }
    return s;
  }

  function esc(s) {
    var d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  function isRelativeAssetURL(url) {
    return url && !url.startsWith('/') && !url.startsWith('#') && !url.startsWith('?') &&
      !url.startsWith('//') && !/^[a-z][a-z0-9+.-]*:/i.test(url);
  }

  // Article HTML is rendered by the backend with Goldmark.  Keep image URLs
  // relative to the article directory, as Markdown image references such as
  // ![alt](1) are stored alongside the article.
  function renderArticleBody(html) {
    if (!html) return '';
    var container = document.createElement('div');
    container.innerHTML = html;
    container.querySelectorAll('img[src]').forEach(function(img) {
      var src = img.getAttribute('src');
      if (!isRelativeAssetURL(src)) return;
      var resolved = currentBasePath() + '/' + src;
      img.setAttribute('src', resolved);

      // renderMarkdown wraps Markdown images in a link to the same file.
      var link = img.closest('a');
      if (link && link.getAttribute('href') === src) link.setAttribute('href', resolved);
    });
    return container.innerHTML;
  }

  var _basePath = '';
  function currentBasePath() { return _basePath; }

  function fmtDate(dateStr) {
    var d = new Date(dateStr);
    var pad = function(n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  function fmtDateShort(dateStr) {
    var d = new Date(dateStr);
    var pad = function(n) { return String(n).padStart(2, '0'); };
    return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate());
  }

  function datetimeISO(dateStr) {
    return new Date(dateStr).toISOString().replace(/\.\d+Z$/, 'Z');
  }

  function getTzOffset(dateStr) {
    if (dateStr.endsWith('Z')) return '+00:00';
    var m = dateStr.match(/([+-]\d{2}:\d{2})$/);
    return m ? m[1] : null;
  }

  function fmtDateInTz(dateStr, tzOffset) {
    var d = new Date(dateStr);
    var m = tzOffset.match(/([+-])(\d{2}):(\d{2})/);
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
    var addr = emailLocal + '+' + uniqueid + '@' + emailDomain;
    var subj = 'Re: ' + subject + (isComment ? ' - Comment' : '') + ' #' + uniqueid;
    var mailBody = '\n\n\n> ---\n> ' + t('write_reply') + '\n>\n> On ' + date + ', ' + author + ' wrote:\n';
    if (body) {
      var lines = body.split('\n').slice(0, 10);
      mailBody += '> ' + lines.join('\n> ').substring(0, 800) + '\n';
    }
    return 'mailto:' + addr + '?subject=' + encodeURIComponent(subj) + '&body=' + encodeURIComponent(mailBody);
  }

  async function api(path, noCache) {
    if (!noCache && apiCache[path]) return apiCache[path];
    var res = await fetch(path);
    if (!res.ok) throw new Error('API error: ' + res.status);
    var data = await res.json();
    if (!noCache) apiCache[path] = data;
    return data;
  }

  async function fetchArticles() {
    var now = Date.now();
    if (articlesCache && now - articlesCacheTime < 5000) return articlesCache;
    var data = await api('/api/articles');
    articlesCache = data;
    articlesCacheTime = now;
    return data;
  }

  function invalidateArticlesCache() {
    articlesCache = null;
    articlesCacheTime = 0;
  }

  // --- Settings bar: language + theme switchers ---
  function initSettingsBar() {
    var header = document.getElementById('header');
    if (!header || header.querySelector('.settings-bar')) return;

    var bar = document.createElement('div');
    bar.className = 'settings-bar';

    // Language select
    var langLabel = document.createElement('span');
    langLabel.textContent = '🌐 ';
    var langSel = document.createElement('select');
    langSel.id = 'lang-select';
    var langNames = { en: 'English', zh: '中文' };
    var optAuto = document.createElement('option');
    optAuto.value = 'auto';
    optAuto.textContent = t('lang_auto');
    langSel.appendChild(optAuto);
    AVAILABLE_LOCALES.forEach(function(code) {
      var opt = document.createElement('option');
      opt.value = code;
      opt.textContent = langNames[code] || code;
      langSel.appendChild(opt);
    });
    // Set current value
    var storedLang = localStorage.getItem('lang');
    langSel.value = storedLang || 'auto';
    langSel.addEventListener('change', function() {
      if (this.value === 'auto') {
        localStorage.removeItem('lang');
      } else {
        localStorage.setItem('lang', this.value);
      }
      // Force route to detect lang change
      _lastLang = '';
      route();
    });

    // Theme button
    var themeBtn = document.createElement('button');
    themeBtn.id = 'theme-toggle';
    var schemeLabels = { auto: t('theme_auto'), light: t('theme_light'), dark: t('theme_dark') };
    var currentScheme = getScheme();
    var symbols = { auto: '◐', light: '☀', dark: '☾' };
    themeBtn.textContent = symbols[currentScheme] + ' ' + schemeLabels[currentScheme];
    themeBtn.addEventListener('click', function() {
      var order = ['auto', 'light', 'dark'];
      var cur = getScheme();
      var next = order[(order.indexOf(cur) + 1) % 3];
      setScheme(next);
      var labels = { auto: t('theme_auto'), light: t('theme_light'), dark: t('theme_dark') };
      var syms = { auto: '◐', light: '☀', dark: '☾' };
      themeBtn.textContent = syms[next] + ' ' + labels[next];
    });

    bar.appendChild(langLabel);
    bar.appendChild(langSel);
    bar.appendChild(themeBtn);
    header.appendChild(bar);
  }

  // --- Render functions ---
  async function renderHeader() {
    if (!siteData) siteData = await api('/api/site');
    var s = siteData;
    var html = '<div class="header-banner-area">';
    if (s.avatar) {
      html += '<img src="' + esc(s.avatar) + '" alt="" class="avatar-banner">';
    }
    html += '</div>';
    html += '<h1><a href="/">' + esc(locale.title || 'Blog') + '</a></h1>';
    if (locale.subtitle) html += '<p class="subtitle">' + esc(locale.subtitle) + '</p>';
    if (s.links && s.links.length) {
      html += '<nav class="nav-links">';
      for (var i = 0; i < s.links.length; i++) {
        var l = s.links[i];
        html += '<a href="' + esc(l.URL) + '">' + esc(l.Title) + '</a>';
      }
      html += '</nav>';
    }
    document.getElementById('header').innerHTML = html;
    initSettingsBar();
    document.title = locale.title || 'Blog';
    document.documentElement.lang = s.lang || 'en';
  }

  async function renderFooter(isIndex) {
    if (!siteData) siteData = await api('/api/site');
    var html = '';
    if (!isIndex) {
      html += '<p><a href="/">' + t('back_to_index') + '</a></p>';
    }
    var footer = locale.footer_html || '';
    if (footer) {
      if (footer.startsWith('/')) {
        try {
          var res = await fetch(footer);
          if (res.ok) html += await res.text();
        } catch (e) {}
      } else {
        html += footer;
      }
    }
    document.getElementById('footer').innerHTML = html;
  }

  async function renderIndex() {
    var data = await fetchArticles();
    var articles = data.articles || [];
    if (!siteData) siteData = await api('/api/site');
    document.title = locale.title || 'Blog';

    var html = '';
    if (!articles || articles.length === 0) {
      var email = siteData.email_local + '@' + siteData.email_domain;
      html = '<table class="article-list"><tbody><tr><td colspan="2" class="empty">' + t('no_articles', {email: email}) + '</td></tr></tbody></table>';
    } else {
      html = '<table class="article-list"><tbody>';
      for (var i = 0; i < articles.length; i++) {
        var a = articles[i];
        var link = a.slug || a.uniqueid;
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
    var article;
    try {
      article = await api('/api/article/' + id);
    } catch (e) {
      document.getElementById('app').innerHTML = '<h2>' + t('not_found') + '</h2><p>' + t('article_not_found') + '</p>';
      document.title = t('not_found');
      return;
    }

    var baseSlug = article.slug || article.uniqueid;
    _basePath = '/' + baseSlug;
    if (!siteData) siteData = await api('/api/site');

    document.title = article.subject + ' - ' + (locale.title || 'Blog');

    // Update banner if article has one
    var bannerArea = document.querySelector('.header-banner-area');
    if (bannerArea && article.banner) {
      bannerArea.innerHTML = '<a href="' + _basePath + '/' + article.banner + '" target="_blank" rel="noopener"><img src="' + _basePath + '/' + article.banner + '" alt="" class="avatar-banner"></a>';
    }

    var html = '<article class="article-full">';
    html += '<h2 class="article-subject">' + esc(article.subject) + '</h2>';
    html += '<div class="article-meta-bar">';
    html += '<span class="author" title="' + esc(authorTooltip(article.author_hash, article.author_email)) + '">' + esc(article.author) + '</span>';
    var tz = getTzOffset(article.date) || '';
    html += '<time class="date" datetime="' + datetimeISO(article.date) + '" data-orig="' + esc(article.date) + '" data-tz="' + esc(tz) + '">' + fmtDate(article.date) + '</time>';
    html += '<a href="/' + esc(baseSlug) + '" class="uid" title="permalink" data-address="/' + esc(baseSlug) + '">#' + esc(article.uniqueid) + '</a>';
    html += '</div>';
    html += '<div class="article-body">' + renderArticleBody(article.body_html) + '</div>';

    if (article.images && article.images.length) {
      var refs = (article.body.match(/!\[[^\]]*\]\(([^)]+)\)/g) || []).map(function(m) {
        return m.match(/\]\(([^)]+)\)/)[1].replace(/\.[^.]+$/, '');
      });
      var unreferenced = article.images.filter(function(img) {
        var name = img.replace(/\.[^.]+$/, '');
        return refs.indexOf(name) === -1 && !img.startsWith('c_');
      });
      if (unreferenced.length) {
        html += '<div class="attachments-section"><hr><h4 class="attachments-title">' + t('attachments') + '</h4><div class="img-tiles">';
        for (var j = 0; j < unreferenced.length; j++) {
          html += '<a href="' + _basePath + '/' + unreferenced[j] + '" target="_blank" rel="noopener"><img src="' + _basePath + '/' + unreferenced[j] + '" width="150" height="150" loading="lazy"></a>';
        }
        html += '</div></div>';
      }
    }
    html += '</article>';

    // Comments section
    html += '<section class="comments-section">';
    var replyAddr = article.email_local + '+' + article.uniqueid + '@' + article.email_domain;
    var mailtoLink = makeMailto(article.uniqueid, article.subject, article.email_local, article.email_domain, article.body, article.author, fmtDate(article.date), false);
    html += '<h3>' + t('comments') + ' <a href="' + esc(mailtoLink) + '" class="reply-link">' + t('reply') + '</a>';
    html += ' <a href="#" class="copy-link" data-address="' + esc(replyAddr) + '">' + t('copy_address') + '</a></h3>';

    try {
      var comments = await api('/api/article/' + id + '/comments');
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
    initCodeCopyButtons();
  }

  function renderComments(comments, article) {
    var commentMap = {};
    comments.forEach(function(c) { commentMap[c.uniqueid] = c; });

    var top = [];
    var replies = {};
    comments.forEach(function(c) {
      if (!c.reply_to || c.reply_to === article.uniqueid) {
        top.push(c);
      } else {
        if (!replies[c.reply_to]) replies[c.reply_to] = [];
        replies[c.reply_to].push(c);
      }
    });

    var html = '';
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
    var cls = 'comment' + (depth === 1 ? ' comment-reply' : '') + (c.deleted ? ' comment-deleted' : '');
    var baseSlug = article.slug || article.uniqueid;
    var html = '<div id="c-' + c.uniqueid + '" class="' + cls + '">';
    html += '<div class="comment-header">';
    html += '<span class="author" title="' + esc(authorTooltip(c.author_hash, c.author_email)) + '">' + esc(c.author) + '</span>';
    var ctz = getTzOffset(c.date) || '';
    html += '<time class="date" datetime="' + datetimeISO(c.date) + '" data-orig="' + esc(c.date) + '" data-tz="' + esc(ctz) + '">' + fmtDate(c.date) + '</time>';
    html += ' <a href="/' + esc(baseSlug) + '#c-' + c.uniqueid + '" class="uid" title="permalink" data-address="/' + esc(baseSlug) + '#c-' + c.uniqueid + '">#' + esc(c.uniqueid) + '</a>';
    if (!c.deleted) {
      var mailtoLink = makeMailto(c.uniqueid, article.subject, article.email_local, article.email_domain, c.body, c.author, fmtDate(c.date), true);
      html += ' <a href="' + esc(mailtoLink) + '" class="reply-link">' + t('reply') + '</a>';
      var copyAddr = article.email_local + '+' + c.uniqueid + '@' + article.email_domain;
      html += ' <a href="#" class="copy-link" data-address="' + esc(copyAddr) + '">' + t('copy_address') + '</a>';
    }
    html += '</div>';
    if (c.reply_to && c.reply_to !== article.uniqueid && commentMap[c.reply_to]) {
      var parent = commentMap[c.reply_to];
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
        var addr = this.getAttribute('data-address');
        navigator.clipboard.writeText(addr).then(function() {
          var orig = btn.textContent;
          btn.textContent = t('copied');
          setTimeout(function() { btn.textContent = orig; }, 1500);
        }).catch(function() {
          var orig = btn.textContent;
          btn.textContent = t('copy_error');
          setTimeout(function() { btn.textContent = orig; }, 1500);
        });
      });
    });
  }

  function initReplyTargetLinks() {
    document.querySelectorAll('.reply-target-link').forEach(function(link) {
      link.addEventListener('click', function(e) {
        e.preventDefault();
        var hash = this.getAttribute('href');
        var target = document.querySelector(hash);
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
      var target = document.querySelector(window.location.hash);
      if (target) {
        target.classList.add('highlight');
        setTimeout(function() { target.scrollIntoView({block: 'center'}); }, 100);
      }
    }
  }

  function initCodeCopyButtons() {
    document.querySelectorAll('.article-body pre, .comment-body pre').forEach(function(pre) {
      if (pre.parentNode.classList.contains('code-block-wrap')) return;
      var wrap = document.createElement('div');
      wrap.className = 'code-block-wrap';
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);
      var btn = document.createElement('button');
      btn.className = 'code-copy-btn';
      btn.textContent = 'copy';
      btn.addEventListener('click', function() {
        navigator.clipboard.writeText(pre.textContent).then(function() {
          btn.textContent = 'copied!';
          setTimeout(function() { btn.textContent = 'copy'; }, 1500);
        }).catch(function() {
          btn.textContent = 'error';
          setTimeout(function() { btn.textContent = 'copy'; }, 1500);
        });
      });
      wrap.appendChild(btn);
    });
  }

  function getPath() {
    return window.location.pathname;
  }

  var _lastLang = '';
  var _initialLoad = true;

  async function route() {
    var path = getPath();
    var lang = getLang();
    var langChanged = lang !== _lastLang;

    if (!siteData) siteData = await api('/api/site');
    await loadTheme();

    // Only reload locale if language changed
    if (langChanged) {
      await loadLocale(lang);
      _lastLang = lang;
    }

    // Re-render header on initial load or language change
    if (_initialLoad || langChanged) {
      await renderHeader();
    }

    var isIndex = (path === '/' || path === '');

    // Footer only on non-index
    if (!isIndex || _initialLoad || langChanged) {
      await renderFooter(isIndex);
    }

    // Content transition: fade out, swap, fade in
    var appEl = document.getElementById('app');
    if (!_initialLoad) {
      appEl.classList.add('route-enter');
      await new Promise(function(r) { setTimeout(r, 80); });
    }

    if (isIndex) {
      await renderIndex();
    } else {
      var id = path.replace(/^\//, '').replace(/\/$/, '');
      await renderArticle(id);
    }

    appEl.classList.remove('route-enter');
    appEl.classList.add('route-active');
    _initialLoad = false;
  }

  document.addEventListener('click', function(e) {
    var a = e.target.closest('a');
    if (!a) return;
    var href = a.getAttribute('href');
    if (!href) return;
    if (href.startsWith('http') || href.startsWith('mailto:') || href.startsWith('#') || href.startsWith('/static/') || href.startsWith('/feed') || href.startsWith('/sitemap') || href.startsWith('/settings')) return;
    e.preventDefault();
    history.pushState(null, '', href);
    route();
  });

  window.addEventListener('popstate', route);

  // Apply stored scheme on load (before first paint)
  applyScheme(getScheme());
  route();
})();
