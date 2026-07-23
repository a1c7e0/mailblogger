package web

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mailblogger/blog"

	"golang.org/x/image/draw"
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

func (s *Server) detectAssets() {
	s.avatarFile = s.Store.DetectAvatar()
	faviconSVG := filepath.Join(s.Store.ContentDir, "favicon.svg")
	faviconICO := filepath.Join(s.Store.ContentDir, "favicon.ico")
	svgExists := false
	if _, err := os.Stat(faviconSVG); err == nil {
		svgExists = true
	}
	icoExists := false
	if _, err := os.Stat(faviconICO); err == nil {
		icoExists = true
	}
	if svgExists && icoExists {
		return
	}
	if s.avatarFile == "" {
		return
	}
	avatarPath := filepath.Join(s.Store.ContentDir, s.avatarFile)
	data, err := os.ReadFile(avatarPath)
	if err != nil {
		return
	}
	ext := s.avatarFile[strings.LastIndex(s.avatarFile, ".")+1:]
	mime := "image/" + ext
	if ext == "jpg" {
		mime = "image/jpeg"
	}
	if !svgExists {
		s.cachedFaviconSVG = []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><image href="data:%s;base64,%s" width="256" height="256"/></svg>`,
			mime, encodeBase64(data)))
	}
	if !icoExists {
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			s.cachedFaviconICO = generateICO(img)
		}
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	base := filepath.Base(r.URL.Path)
	if base == "favicon.svg" {
		p := filepath.Join(s.Store.ContentDir, "favicon.svg")
		if _, err := os.Stat(p); err == nil {
			http.ServeFile(w, r, p)
			return
		}
		if len(s.cachedFaviconSVG) > 0 {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write(s.cachedFaviconSVG)
			return
		}
		http.NotFound(w, r)
		return
	}
	if base == s.avatarFile && s.avatarFile != "" {
		http.ServeFile(w, r, filepath.Join(s.Store.ContentDir, base))
		return
	}
	path := filepath.Join("static", strings.TrimPrefix(r.URL.Path, "/static/"))
	http.ServeFile(w, r, path)
}

func (s *Server) handleFaviconICO(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(s.Store.ContentDir, "favicon.ico")
	if _, err := os.Stat(p); err == nil {
		http.ServeFile(w, r, p)
		return
	}
	if len(s.cachedFaviconICO) > 0 {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(s.cachedFaviconICO)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleRobotsTXT(w http.ResponseWriter, r *http.Request) {
	p := filepath.Join(s.Store.ContentDir, "robots.txt")
	if _, err := os.Stat(p); err == nil {
		http.ServeFile(w, r, p)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s://%s/sitemap.xml\n", s.Scheme, s.Host)
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
	fmt.Fprintf(buf, `<content type="html">%s</content>`, xmlEscape(rewriteFeedImages(content.String(), a.UniqueID, s.Scheme, s.Host)))
	buf.WriteString(`</entry>`)
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	cacheKey := "sitemap:" + s.Host
	if cached := globalFeedCache.get(cacheKey); cached != nil {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write(cached)
		return
	}

	baseURL := fmt.Sprintf("%s://%s", s.Scheme, s.Host)
	articles, _ := s.Store.ListArticles()

	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	fmt.Fprintf(&buf, `<url><loc>%s/</loc></url>`, baseURL)
	for _, a := range articles {
		id := a.UniqueID
		if a.Slug != "" {
			id = a.Slug
		}
		fmt.Fprintf(&buf, `<url><loc>%s/%s</loc><lastmod>%s</lastmod></url>`,
			baseURL, id, a.Date.Format(time.RFC3339))
	}
	buf.WriteString(`</urlset>`)

	data := []byte(buf.String())
	globalFeedCache.set(cacheKey, data)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write(data)
}

func generateICO(src image.Image) []byte {
	const size = 32
	resized := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.NearestNeighbor.Scale(resized, resized.Bounds(), src, src.Bounds(), draw.Over, nil)
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, resized)
	pngData := pngBuf.Bytes()
	pngSize := uint32(len(pngData))
	headerSize := uint32(6 + 16)
	ico := make([]byte, headerSize+pngSize)
	ico[0] = 0
	ico[1] = 0
	ico[2] = 1
	ico[3] = 0
	ico[4] = 1
	ico[5] = 0
	ico[6] = 0
	ico[7] = 0
	ico[8] = 32
	ico[9] = 0
	ico[10] = 1
	ico[11] = 0
	ico[12] = 32
	ico[13] = 0
	binary.LittleEndian.PutUint32(ico[14:18], pngSize)
	binary.LittleEndian.PutUint32(ico[18:22], headerSize)
	copy(ico[headerSize:], pngData)
	return ico
}

func encodeBase64(data []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf strings.Builder
	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}
		buf.WriteByte(tbl[b0>>2])
		buf.WriteByte(tbl[((b0&3)<<4)|(b1>>4)])
		if i+1 < len(data) {
			buf.WriteByte(tbl[((b1&15)<<2)|(b2>>6)])
		} else {
			buf.WriteByte('=')
		}
		if i+2 < len(data) {
			buf.WriteByte(tbl[b2&63])
		} else {
			buf.WriteByte('=')
		}
	}
	return buf.String()
}
