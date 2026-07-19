package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

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
