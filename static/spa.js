(function() {
  function getEmailLocal() {
    var meta = document.querySelector('meta[name="email-local"]');
    return meta ? meta.content : '';
  }

  function initPage() {
    var pad = function(n) { return String(n).padStart(2, '0'); };
    document.querySelectorAll('time[datetime]').forEach(function(el) {
      var d = new Date(el.getAttribute('datetime'));
      if (isNaN(d)) return;
      var off = -d.getTimezoneOffset();
      var sign = off >= 0 ? '+' : '-';
      var hh = pad(Math.floor(Math.abs(off) / 60));
      var mm = pad(Math.abs(off) % 60);
      if (el.getAttribute('data-format') === 'date') {
        el.textContent = d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate());
        return;
      }
      el.textContent = d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate())
        + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ' '
        + sign + hh + mm;
    });
    document.querySelectorAll('.reply-link').forEach(function(link) {
      var href = link.getAttribute('href');
      var pos = href.indexOf('&body=');
      if (pos === -1) return;
      var body = decodeURIComponent(href.substring(pos + 6));
      var match = body.match(/On (\d{4}-\d{2}-\d{2} \d{2}:\d{2}) UTC,/);
      if (!match) return;
      var d = new Date(match[1] + ' UTC');
      if (isNaN(d)) return;
      var off = -d.getTimezoneOffset();
      var sign = off >= 0 ? '+' : '-';
      var local = d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate())
        + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes()) + ' '
        + sign + pad(Math.floor(Math.abs(off) / 60)) + pad(Math.abs(off) % 60);
      body = body.replace('On ' + match[1] + ' UTC,', 'On ' + local + ',');
      link.setAttribute('href', href.substring(0, pos + 6) + encodeURIComponent(body));
    });
    var emailLocal = getEmailLocal();
	    document.querySelectorAll('.code-copy-btn[data-code-copy]').forEach(function(btn) {
	      btn.removeEventListener('click', btn._codeCopyHandler);
	      btn._codeCopyHandler = function() {
	        var block = this.closest('.code-block');
	        var pre = block && block.querySelector('pre');
	        if (!pre) return;
	        var original = this.textContent;
	        navigator.clipboard.writeText(pre.textContent).then(function() {
	          this.textContent = 'copied!';
	          var that = this;
	          setTimeout(function() { that.textContent = original; }, 1500);
	        }.bind(this)).catch(function() {
	          this.textContent = 'error';
	          var that = this;
	          setTimeout(function() { that.textContent = original; }, 1500);
	        }.bind(this));
	      };
	      btn.addEventListener('click', btn._codeCopyHandler);
	    });
    document.querySelectorAll('.copy-link,.uid[data-address]').forEach(function(btn) {
      btn.removeEventListener('click', btn._copyHandler);
      btn._copyHandler = function(e) {
        e.preventDefault();
        e.stopPropagation();
        var addr = this.getAttribute('data-address');
        if (emailLocal && addr.indexOf(emailLocal + '+') !== 0 && addr.indexOf('/') === 0) {
          addr = window.location.origin + addr;
        }
        navigator.clipboard.writeText(addr).then(function() {
          var orig = this.textContent;
          this.textContent = '[copied]';
          var that = this;
          setTimeout(function() { that.textContent = orig; }, 1500);
        }.bind(this)).catch(function() {
          var orig = this.textContent;
          this.textContent = '[error]';
          var that = this;
          setTimeout(function() { that.textContent = orig; }, 1500);
        }.bind(this));
      };
      btn.addEventListener('click', btn._copyHandler);
    });
    document.querySelectorAll('.reply-target-link').forEach(function(link) {
      link.removeEventListener('click', link._replyHandler);
      link._replyHandler = function(e) {
        e.preventDefault();
        var hash = this.getAttribute('href');
        var target = document.querySelector(hash);
        if (target) {
          document.querySelectorAll('.comment').forEach(function(c) { c.classList.remove('highlight'); });
          target.classList.add('highlight');
          target.scrollIntoView({behavior: 'smooth', block: 'center'});
        }
      };
      link.addEventListener('click', link._replyHandler);
    });
    if (window.location.hash) {
      var target = document.querySelector(window.location.hash);
      if (target) {
        target.classList.add('highlight');
        setTimeout(function() { target.scrollIntoView({block: 'center'}); }, 100);
      }
    }
  }

  window.initPage = initPage;
  document.addEventListener('DOMContentLoaded', initPage);

  function isInternalLink(link) {
    var href = link.getAttribute('href');
    if (!href) return false;
    if (href.startsWith('mailto:') || href.startsWith('http') || href.startsWith('/feed') || href.startsWith('/static/')) return false;
    if (link.hasAttribute('target') && link.getAttribute('target') === '_blank') return false;
    if (/\.\w{2,5}$/.test(href)) return false;
    return true;
  }

  document.addEventListener('click', function(e) {
    var link = e.target.closest('a');
    if (!link || !isInternalLink(link)) return;
    e.preventDefault();
    var href = link.getAttribute('href');
    fetch(href, { headers: { 'Accept': 'text/html' } })
      .then(function(r) { return r.text(); })
      .then(function(html) {
        var doc = new DOMParser().parseFromString(html, 'text/html');
        var newMain = doc.querySelector('main');
        var curMain = document.querySelector('main');
        if (newMain && curMain) curMain.replaceWith(newMain);
        var newTitle = doc.querySelector('title');
        if (newTitle) document.title = newTitle.textContent;
        var newBase = doc.querySelector('base');
        var curBase = document.querySelector('base');
        if (newBase) {
          if (curBase) curBase.href = newBase.href;
          else document.head.appendChild(newBase.cloneNode(true));
        } else if (curBase) {
          curBase.remove();
        }
        history.pushState({}, '', href);
        var newBanner = doc.querySelector('.header-banner-area');
        var curBanner = document.querySelector('.header-banner-area');
        if (newBanner && curBanner) curBanner.innerHTML = newBanner.innerHTML;
        else if (curBanner) curBanner.innerHTML = '';
        initPage();
      });
  });

  window.addEventListener('popstate', function() { location.reload(); });
})();
