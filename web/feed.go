package web

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"mailblogger/blog"
)

const feedCacheTTL = 5 * time.Minute

type feedCache struct {
	data      []byte
	expiresAt time.Time
}

type feedCacheStore struct {
	mu    sync.RWMutex
	items map[string]*feedCache
}

var globalFeedCache = &feedCacheStore{items: make(map[string]*feedCache)}

func (fc *feedCacheStore) get(key string) []byte {
	fc.mu.RLock()
	c, ok := fc.items[key]
	fc.mu.RUnlock()
	if !ok || time.Now().After(c.expiresAt) {
		return nil
	}
	return c.data
}

func (fc *feedCacheStore) set(key string, data []byte) {
	fc.mu.Lock()
	fc.items[key] = &feedCache{data: data, expiresAt: time.Now().Add(feedCacheTTL)}
	fc.mu.Unlock()
}

func (fc *feedCacheStore) invalidate() {
	fc.mu.Lock()
	fc.items = make(map[string]*feedCache)
	fc.mu.Unlock()
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	cacheKey := "feed:" + s.Host
	if cached := globalFeedCache.get(cacheKey); cached != nil {
		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		w.Write(cached)
		return
	}

	articles, _ := s.Store.ListArticles()
	var buf strings.Builder
	s.writeFeed(&buf, articles, "feed", false)
	data := []byte(buf.String())
	globalFeedCache.set(cacheKey, data)

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleFeedFull(w http.ResponseWriter, r *http.Request) {
	cacheKey := "feed-full:" + s.Host
	if cached := globalFeedCache.get(cacheKey); cached != nil {
		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		w.Write(cached)
		return
	}

	articles, _ := s.Store.ListArticles()
	var buf strings.Builder
	s.writeFeed(&buf, articles, "feed-full", true)
	data := []byte(buf.String())
	globalFeedCache.set(cacheKey, data)

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write(data)
}

func (s *Server) writeFeed(buf *strings.Builder, articles []*blog.Article, feedID string, includeComments bool) {
	baseURL := fmt.Sprintf("%s://%s", s.Scheme, s.Host)
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	buf.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom">`)
	fmt.Fprintf(buf, `<title>%s</title>`, xmlEscape(s.Host))
	fmt.Fprintf(buf, `<link href="%s/"/>`, baseURL)
	fmt.Fprintf(buf, `<id>tag:%s,2026:%s</id>`, s.Host, feedID)
	fmt.Fprintf(buf, `<updated>%s</updated>`, time.Now().UTC().Format(time.RFC3339))
	for _, a := range articles {
		if includeComments {
			comments, _ := s.Store.GetComments(a.UniqueID)
			s.writeFeedEntry(buf, a, baseURL, comments)
		} else {
			s.writeFeedEntry(buf, a, baseURL, nil)
		}
	}
	buf.WriteString(`</feed>`)
}

func (s *Server) writeFeedEntry(buf *strings.Builder, a *blog.Article, baseURL string, comments []*blog.Comment) {
	buf.WriteString(`<entry>`)
	fmt.Fprintf(buf, `<title>%s</title>`, xmlEscape(a.Subject))
	fmt.Fprintf(buf, `<link href="%s/%s"/>`, baseURL, a.UniqueID)
	fmt.Fprintf(buf, `<id>tag:%s,%s:%s</id>`, s.Host, a.Date.Format("2006-01-02"), a.UniqueID)
	fmt.Fprintf(buf, `<author><name>%s</name></author>`, xmlEscape(a.Author))
	fmt.Fprintf(buf, `<published>%s</published>`, a.Date.UTC().Format(time.RFC3339))
	fmt.Fprintf(buf, `<updated>%s</updated>`, a.Date.UTC().Format(time.RFC3339))

	var content strings.Builder
	var bodyBuf strings.Builder
	md.Convert([]byte(a.Body), &bodyBuf)
	content.WriteString(bodyBuf.String())
	for _, c := range comments {
		content.WriteString(`<hr/>`)
		var cb strings.Builder
		md.Convert([]byte(c.Body), &cb)
		content.WriteString(cb.String())
	}
	fmt.Fprintf(buf, `<content type="html">%s</content>`, xmlEscape(s.rewriteFeedImages(content.String(), a.UniqueID)))
	buf.WriteString(`</entry>`)
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

var feedImgRe = regexp.MustCompile(`<img src="([^"]+)"`)

func (s *Server) rewriteFeedImages(html, articleID string) string {
	return feedImgRe.ReplaceAllStringFunc(html, func(match string) string {
		idx := strings.Index(match, `src="`)
		src := match[idx+5:]
		src = src[:strings.Index(src, `"`)]
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "/") {
			return match
		}
		return fmt.Sprintf(`<img src="%s://%s/%s/%s"`, s.Scheme, s.Host, articleID, src)
	})
}
