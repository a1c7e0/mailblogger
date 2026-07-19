package web
import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/binary"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mailblogger/blog"
	"mailblogger/config"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"golang.org/x/image/draw"
)

//go:embed templates/*
var templateFS embed.FS

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		extension.DefinitionList,
	),
)

type Server struct {
	Store       *blog.Store
	Host        string
	Scheme      string
	EmailLocal  string
	EmailDomain string
	HideEmail   bool
	Site        config.SiteConfig
	Port        int
	Addr        string
	tmpl        *template.Template
	configGetter func() *config.Config
	avatarFile  string
	cachedFaviconSVG []byte
	cachedFaviconICO []byte
}

func NewServer(store *blog.Store, host, scheme, emailLocal, emailDomain string, hideEmail bool, site config.SiteConfig, listenHost string, port int) (*Server, error) {
	funcMap := template.FuncMap{
		"renderMD":  renderMarkdown,
		"renderPlaintext": renderPlaintext,
		"mailto":    makeMailto,
		"rawHTML":   func(s string) template.HTML { return template.HTML(s) },
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
		"fmtDate":      fmtDate,
		"fmtDateTitle": fmtDateTitle,
		"datetimeISO":  datetimeISO,
		"truncate":  truncate,
		"excerpt":  excerpt,
		"urlencode": url.QueryEscape,
		"commentImages": func(articleID, commentUID string) []string {
			imgs, _ := store.ListCommentImages(articleID, commentUID)
			return imgs
		},
		"authorTooltip": func(authorHash, authorEmail string) string {
			return authorTooltipFn(store, hideEmail, authorHash, authorEmail)
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	srv := &Server{
		Store:       store,
		Host:        host,
		Scheme:      scheme,
		EmailLocal:  emailLocal,
		EmailDomain: emailDomain,
		HideEmail:   hideEmail,
		Site:        site,
		Port:        port,
		Addr:        listenHost,
		tmpl:        tmpl,
	}
	srv.detectAssets()
	return srv, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// New SPA API endpoints
	mux.HandleFunc("GET /api/site", s.handleAPISite)
	mux.HandleFunc("GET /api/articles", s.handleAPIArticles)
	mux.HandleFunc("GET /api/article/{id}", s.handleAPIArticleDetail)
	mux.HandleFunc("GET /api/article/{id}/comments", s.handleAPIArticleComments)
	// Existing API endpoints
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/api/article", s.handleAPIArticle)
	mux.HandleFunc("/api/comment", s.handleAPIComment)
	mux.HandleFunc("/api/raw-email", s.handleAPIRawEmail)
	// Static and legacy
	mux.HandleFunc("/static/", s.handleStatic)
	mux.HandleFunc("/favicon.ico", s.handleFaviconICO)
	mux.HandleFunc("/feed.xml", s.handleFeed)
	mux.HandleFunc("/feed-full.xml", s.handleFeedFull)
	mux.HandleFunc("/sitemap.xml", s.handleSitemap)
	mux.HandleFunc("/robots.txt", s.handleRobotsTXT)
	mux.HandleFunc("/settings", s.handleSettings)
	// SPA catch-all: serve static files, fallback to index.html
	mux.HandleFunc("/", s.handleSPA)
	return mux
}

func (s *Server) UpdateConfig(site config.SiteConfig) {
	s.Site = site
}

func (s *Server) SetConfigGetter(fn func() *config.Config) {
	s.configGetter = fn
}

func (s *Server) InvalidateFeedCache() {
	globalFeedCache.invalidate()
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

func (s *Server) detectAssets() {
	s.avatarFile = s.Store.DetectAvatar()
	if s.avatarFile != "" {
		s.Site.Avatar = "/static/" + s.avatarFile
	}
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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	perPage := 20

	articles, total, err := s.Store.ListArticlesPaged(page, perPage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	data := map[string]interface{}{
		"Host":        s.Host,
		"Scheme":      s.Scheme,
		"EmailLocal":  s.EmailLocal,
		"EmailDomain": s.EmailDomain,
		"Site":        s.Site,
		"Articles":    articles,
		"Page":       page,
		"TotalPages": totalPages,
		"HasPrev":    page > 1,
		"HasNext":    page < totalPages,
		"PrevPage":   page - 1,
		"NextPage":   page + 1,
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handleArticle(w http.ResponseWriter, r *http.Request, id string) {
	article, err := s.Store.GetArticle(id)
	if err != nil {
		article, err = s.Store.GetArticleBySlug(id)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderArticleBody(w, article)
}

// handleSPA serves static files from static/ directory, falls back to index.html
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		// Root: try static/index.html first, fall back to SSR index
	indexPath := filepath.Join("static", "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}
	s.handleIndex(w, r)
	return
	}

	// Try to serve file from static/ directory
	filePath := filepath.Join("static", filepath.FromSlash(path))
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// SPA fallback: serve static/index.html
	indexPath := filepath.Join("static", "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}

	// Final fallback: SSR article rendering
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 1 && parts[1] != "" {
		editParts := strings.SplitN(parts[1], "/", 2)
		if strings.HasPrefix(editParts[0], "edit_") {
			if !s.Store.History.ArticleVisible {
				http.NotFound(w, r)
				return
			}
			if len(editParts) > 1 && editParts[1] != "" {
				s.serveHistoryFile(w, r, parts[0], editParts[0], editParts[1])
			} else {
				s.handleHistoryArticle(w, r, parts[0], editParts[0])
			}
			return
		}
		s.serveArticleFile(w, r, parts[0], parts[1])
		return
	}
	s.handleArticle(w, r, path)
}

func (s *Server) handleHistoryArticle(w http.ResponseWriter, r *http.Request, id, editDir string) {
	dir, err := s.Store.GetArticleDir(id)
	if err != nil {
		a, err2 := s.Store.GetArticleBySlug(id)
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		dir, err = s.Store.GetArticleDir(a.UniqueID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	editPath := filepath.Join(dir, editDir)
	indexPath := filepath.Join(editPath, "index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse the historical article
	meta, body, err := blog.ParseFrontmatterExport(data)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	article := &blog.Article{
		UniqueID:    meta["uniqueid"],
		Subject:     meta["subject"],
		Author:      meta["author"],
		AuthorHash:  meta["author_hash"],
		AuthorEmail: meta["author_email"],
		Banner:      meta["banner"],
		Body:        body,
	}
	if ds := meta["date"]; ds != "" {
		article.Date, _ = time.Parse(time.RFC3339, ds)
	}
	article.Slug = blog.ParseDirSlugExport(filepath.Base(dir))

	// Read historical comments
	commentsPath := filepath.Join(editPath, "comments.json")
	commentsData, err := os.ReadFile(commentsPath)
	var comments []*blog.Comment
	if err == nil {
		json.Unmarshal(commentsData, &comments)
	}

	s.renderArticleBodyWithComments(w, article, comments)
}

func (s *Server) serveHistoryFile(w http.ResponseWriter, r *http.Request, id, editDir, filename string) {
	filename = filepath.Base(filename)
	if filename == "." || filename == "/" {
		http.NotFound(w, r)
		return
	}

	dir, err := s.Store.GetArticleDir(id)
	if err != nil {
		a, err2 := s.Store.GetArticleBySlug(id)
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		dir, err = s.Store.GetArticleDir(a.UniqueID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	editPath := filepath.Join(dir, editDir)
	if _, err := os.Stat(editPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	if !strings.Contains(filename, ".") {
		entries, err := filepath.Glob(filepath.Join(editPath, filename+".*"))
		if err != nil || len(entries) == 0 {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, entries[0])
		return
	}
	http.ServeFile(w, r, filepath.Join(editPath, filename))
}

func (s *Server) renderArticleBody(w http.ResponseWriter, article *blog.Article) {
	comments, err := s.Store.GetComments(article.UniqueID)
	if err != nil {
		comments = nil
	}
	s.renderArticleBodyWithComments(w, article, comments)
}

func (s *Server) renderArticleBodyWithComments(w http.ResponseWriter, article *blog.Article, comments []*blog.Comment) {
	// Filter deleted comments based on config
	var filtered []*blog.Comment
	deletedIDs := make(map[string]bool)
	for _, c := range comments {
		if c.Deleted {
			deletedIDs[c.UniqueID] = true
			if s.Store.History.ShowDeleted {
				filtered = append(filtered, c)
			}
		} else {
			filtered = append(filtered, c)
		}
	}

	// Filter replies to deleted comments if ShowReplies is false
	if !s.Store.History.ShowReplies {
		var withReplies []*blog.Comment
		for _, c := range filtered {
			if c.ReplyTo != "" && c.ReplyTo != article.UniqueID && deletedIDs[c.ReplyTo] {
				continue
			}
			withReplies = append(withReplies, c)
		}
		filtered = withReplies
	}
	comments = filtered

	commentMap := make(map[string]*blog.Comment)
	for _, c := range comments {
		commentMap[c.UniqueID] = c
	}

	type displayC struct {
		Comment     *blog.Comment
		Depth       int
		ReplyTarget string
	}
	var displayComments []displayC
	ordered := make(map[string][]*blog.Comment)
	for _, c := range comments {
		if c.ReplyTo == "" || c.ReplyTo == article.UniqueID {
			c.Depth = 0
			displayComments = append(displayComments, displayC{Comment: c, Depth: 0})
		} else {
			ordered[c.ReplyTo] = append(ordered[c.ReplyTo], c)
		}
	}
	var merged []displayC
	for _, dc := range displayComments {
		merged = append(merged, dc)
		for _, r := range ordered[dc.Comment.UniqueID] {
			r.Depth = 1
			target := ""
			if t, ok := commentMap[r.ReplyTo]; ok {
				target = t.Author
			}
			merged = append(merged, displayC{Comment: r, Depth: 1, ReplyTarget: target})
		}
	}
	displayComments = merged

	allImages, _ := s.Store.ListImages(article.UniqueID)
	refs := findImageRefs(article.Body)
	normalizedRefs := make(map[string]bool)
	for ref := range refs {
		normalizedRefs[strings.TrimSuffix(ref, filepath.Ext(ref))] = true
	}
	var unreferenced []string
	for _, img := range allImages {
		if strings.HasPrefix(img, "c_") {
			continue
		}
		name := strings.TrimSuffix(img, filepath.Ext(img))
		if !normalizedRefs[name] {
			unreferenced = append(unreferenced, img)
		}
	}

	data := map[string]interface{}{
		"Host":         s.Host,
		"Scheme":       s.Scheme,
		"EmailLocal":   s.EmailLocal,
		"EmailDomain":  s.EmailDomain,
		"Site":         s.Site,
		"Article":      article,
		"Comments":     displayComments,
		"UnreferencedImages": unreferenced,
		"ShowDeleted":  s.Store.History.ShowDeleted,
		"CommentVisible": s.Store.History.CommentVisible,
	}

	if err := s.tmpl.ExecuteTemplate(w, "article.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) serveArticleFile(w http.ResponseWriter, r *http.Request, articleID, filename string) {
	filename = filepath.Base(filename)
	if filename == "." || filename == "/" {
		http.NotFound(w, r)
		return
	}

	dir, err := s.Store.GetArticleDir(articleID)
	if err != nil {
		// Try slug
		a, err2 := s.Store.GetArticleBySlug(articleID)
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		dir, err = s.Store.GetArticleDir(a.UniqueID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	if !strings.Contains(filename, ".") {
		entries, err := filepath.Glob(filepath.Join(dir, filename+".*"))
		if err != nil || len(entries) == 0 {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, entries[0])
		return
	}
	http.ServeFile(w, r, filepath.Join(dir, filename))
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"]+|www\.[^\s<>"]+`)

func renderPlaintext(input string) template.HTML {
	escaped := template.HTMLEscapeString(input)
	escaped = urlRe.ReplaceAllStringFunc(escaped, func(rawURL string) string {
		href := rawURL
		if !strings.HasPrefix(href, "http") {
			href = "https://" + href
		}
		safeHref := template.HTMLEscapeString(href)
		safeText := template.HTMLEscapeString(rawURL)
		return fmt.Sprintf(`<a href="%s">%s</a>`, safeHref, safeText)
	})
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return template.HTML(escaped)
}

func findImageRefs(body string) map[string]bool {
	refs := make(map[string]bool)
	for i := 0; i < len(body); {
		start := strings.Index(body[i:], "![")
		if start < 0 {
			break
		}
		start += i
		paren := strings.Index(body[start:], "](")
		if paren < 0 {
			i = start + 2
			continue
		}
		paren += start + 2
		end := strings.Index(body[paren:], ")")
		if end < 0 {
			break
		}
		name := body[paren : paren+end]
		refs[name] = true
		i = paren + end + 1
	}
	return refs
}

func renderMarkdown(input string) template.HTML {
	input = ensureImageBreaks(input)
	var buf strings.Builder
	if err := md.Convert([]byte(input), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(input))
	}
	return template.HTML(wrapImages(buf.String()))
}

var imgTagReFull = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"[^>]*alt="([^"]*)"[^>]*>`)
var imgTagReAltFirst = regexp.MustCompile(`<img\s+[^>]*alt="([^"]*)"[^>]*src="([^"]+)"[^>]*>`)
var imgTagReNoAlt = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"[^>]*>`)

func wrapImages(html string) string {
	html = imgTagReAltFirst.ReplaceAllStringFunc(html, func(match string) string {
		m := imgTagReAltFirst.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		alt, src := m[1], m[2]
		fig := fmt.Sprintf(`<figure><a href="%s" target="_blank" rel="noopener">%s</a>`, src, match)
		if alt != "" {
			fig += fmt.Sprintf(`<figcaption>%s</figcaption>`, alt)
		}
		fig += `</figure>`
		return fig
	})
	html = imgTagReFull.ReplaceAllStringFunc(html, func(match string) string {
		if strings.Contains(match, "<figure") {
			return match
		}
		m := imgTagReFull.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		src, alt := m[1], m[2]
		fig := fmt.Sprintf(`<figure><a href="%s" target="_blank" rel="noopener">%s</a>`, src, match)
		if alt != "" {
			fig += fmt.Sprintf(`<figcaption>%s</figcaption>`, alt)
		}
		fig += `</figure>`
		return fig
	})
	html = imgTagReNoAlt.ReplaceAllStringFunc(html, func(match string) string {
		if strings.Contains(match, "<figure") {
			return match
		}
		m := imgTagReNoAlt.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		src := m[1]
		return fmt.Sprintf(`<figure><a href="%s" target="_blank" rel="noopener">%s</a></figure>`, src, match)
	})
	return html
}

func ensureImageBreaks(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isImage := strings.HasPrefix(trimmed, "![") && strings.Contains(trimmed, "](") && strings.HasSuffix(trimmed, ")")
		if isImage {
			if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
				result = append(result, "")
			}
			result = append(result, line)
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				result = append(result, "")
			}
		} else {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func makeMailto(uniqueID, subject, emailLocal, emailDomain, body, parentAuthor, parentDate, parentUID, parentHash string, isComment bool) string {
	addr := fmt.Sprintf("%s+%s@%s", emailLocal, uniqueID, emailDomain)
	subjPrefix := "Re: " + subject
	if isComment {
		subjPrefix += " - Comment"
	}
	subjEnc := strings.ReplaceAll(url.QueryEscape(subjPrefix+" #"+parentUID), "+", "%20")
	link := fmt.Sprintf("mailto:%s?subject=%s", addr, subjEnc)
	if parentAuthor != "" {
		rawBody := strings.ReplaceAll(body, "\r\n", "\n")
		quoted := "> " + strings.ReplaceAll(rawBody, "\n", "\n> ")
		if len(quoted) > 1200 {
			quoted = quoted[:1200] + "\n> ..."
		}
		var ref strings.Builder
		ref.WriteString("\n\n\n> ---\n> Write your reply above this line. Only text above will be saved.\n>\n")
		ref.WriteString(fmt.Sprintf("> On %s, %s (%s) wrote:\n>\n", parentDate, parentAuthor, parentHash))
		ref.WriteString(quoted)
		ref.WriteString("\n")
		enc := strings.ReplaceAll(url.QueryEscape(ref.String()), "+", "%20")
		link += "&body=" + enc
	}
	return link
}

func toTime(v interface{}) time.Time {
	switch val := v.(type) {
	case *blog.Article:
		return val.Date
	case blog.Article:
		return val.Date
	case *blog.Comment:
		return val.Date
	case blog.CommentEdit:
		return val.Date
	case *blog.CommentEdit:
		return val.Date
	case time.Time:
		return val
	}
	return time.Time{}
}

func fmtDate(d interface{}) string {
	t := toTime(d)
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func fmtDateTitle(d interface{}) string {
	t := toTime(d)
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (Unix: %d)", t.Format("2006-01-02 15:04:05 -0700"), t.Unix())
}

func datetimeISO(d interface{}) string {
	t := toTime(d)
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var mdStripRe = regexp.MustCompile(`[#*\[\]!\(\)>|_~` + "`" + `]`)

func excerpt(input string, maxLen int) string {
	s := mdStripRe.ReplaceAllString(input, "")
	s = strings.Join(strings.Fields(s), " ")
	return truncate(s, maxLen)
}

func authorTooltipFn(store *blog.Store, globalHideEmail bool, authorHash, authorEmail string) string {
	hide := store.ShouldHideEmail(authorHash, globalHideEmail)
	if hide {
		return "hash: " + authorHash
	}
	if authorEmail != "" {
		return authorEmail + "\nhash: " + authorHash
	}
	return "hash: " + authorHash
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("t")
	if tokenStr == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	token, err := s.Store.GetToken(tokenStr)
	if err != nil || token == nil {
		http.Error(w, "invalid or expired link", http.StatusForbidden)
		return
	}

	// Handle POST (save)
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		// Validate CSRF token
		csrfToken := r.FormValue("csrf_token")
		cookie, _ := r.Cookie("csrf_token")
		if csrfToken == "" || cookie == nil || csrfToken != cookie.Value {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}

		prefs := &blog.UserPrefs{
			ArticleNotify: r.FormValue("article_notify") == "on",
			CommentNotify: r.FormValue("comment_notify") == "on",
			HideEmail:     r.FormValue("hide_email") == "on",
		}
		if err := s.Store.SavePrefs(token.AuthorHash, prefs); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/settings?t="+tokenStr+"&saved=1", http.StatusFound)
		return
	}

	// GET: show form
	prefs, _ := s.Store.GetPrefs(token.AuthorHash)
	if prefs == nil {
		prefs = &blog.UserPrefs{ArticleNotify: true, CommentNotify: false, HideEmail: s.HideEmail}
	}

	articles := s.Store.ListArticlesByAuthor(token.AuthorHash)

	remaining := time.Until(token.ExpiresAt)
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	expiresIn := fmt.Sprintf("%dh %dm", hours, minutes)

	saved := r.URL.Query().Get("saved") == "1"

	// Generate CSRF token
	csrfBytes := make([]byte, 32)
	rand.Read(csrfBytes)
	csrfToken := hex.EncodeToString(csrfBytes)

	// Set CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600, // 1 hour
	})

	data := map[string]interface{}{
		"Site":        s.Site,
		"Token":       tokenStr,
		"AuthorName":  token.AuthorName,
		"Prefs":       prefs,
		"Articles":    articles,
		"ExpiresIn":   expiresIn,
		"Saved":       saved,
		"EmailDomain": s.EmailDomain,
		"CSRFToken":   csrfToken,
	}

	if err := s.tmpl.ExecuteTemplate(w, "settings.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
