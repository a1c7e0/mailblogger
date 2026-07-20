package web
import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mailblogger/blog"
	"mailblogger/config"
)

//go:embed templates/*
var templateFS embed.FS

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
	Theme       string
	ThemeCfg    config.ThemeConfig
	AutoLang    bool
	tmpl        *template.Template
	configGetter func() *config.Config
	avatarFile  string
	cachedFaviconSVG []byte
	cachedFaviconICO []byte
}

// ServerConfig holds the configuration for creating a new Server.
type ServerConfig struct {
	Store       *blog.Store
	Host        string
	Scheme      string
	EmailLocal  string
	EmailDomain string
	HideEmail   bool
	Site        config.SiteConfig
	ListenHost  string
	Port        int
	Theme       config.ThemeConfig
}

func NewServer(cfg ServerConfig) (*Server, error) {
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
			imgs, _ := cfg.Store.ListCommentImages(articleID, commentUID)
			return imgs
		},
		"authorTooltip": func(authorHash, authorEmail string) string {
			return authorTooltipFn(cfg.Store, cfg.HideEmail, authorHash, authorEmail)
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	srv := &Server{
		Store:       cfg.Store,
		Host:        cfg.Host,
		Scheme:      cfg.Scheme,
		EmailLocal:  cfg.EmailLocal,
		EmailDomain: cfg.EmailDomain,
		HideEmail:   cfg.HideEmail,
		Site:        cfg.Site,
		Port:        cfg.Port,
		Addr:        cfg.ListenHost,
		Theme:       cfg.Theme.Theme,
		ThemeCfg:    cfg.Theme,
		AutoLang:    cfg.Site.AutoLang,
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

// handleSPA serves static files from static/ directory, falls back to SSR or theme
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/")

	// Try to serve file from static/ directory first
	if path != "" {
		filePath := filepath.Join("static", filepath.FromSlash(path))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, filePath)
			return
		}
	}

	// Resolve theme based on language
	theme := s.resolveTheme(r)
	if theme != "" {
		themeDir := filepath.Join("themes", theme)
		indexPath := filepath.Join(themeDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			// Try serving theme static files
			if path != "" {
				themeFile := filepath.Join(themeDir, filepath.FromSlash(path))
				if info, err := os.Stat(themeFile); err == nil && !info.IsDir() {
					http.ServeFile(w, r, themeFile)
					return
				}
			}
			http.ServeFile(w, r, indexPath)
			return
		}
	}

	// SSR fallback
	if path == "" {
		s.handleIndex(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 1 && parts[1] != "" {
		editParts := strings.SplitN(parts[1], "/", 2)
		if strings.HasPrefix(editParts[0], "edit_") {
			if !s.Store.History.Article.Visible {
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

// resolveTheme determines which theme to use based on Accept-Language and config
func (s *Server) resolveTheme(r *http.Request) string {
	// If per-language themes are configured and auto_lang is enabled
	if s.AutoLang && len(s.ThemeCfg.Themes) > 0 {
		lang := parseAcceptLanguage(r.Header.Get("Accept-Language"))
		if lang != "" {
			if theme, ok := s.ThemeCfg.Themes[lang]; ok {
				return theme
			}
		}
	}
	// Fallback to resolved theme (single theme or first in map)
	return s.ThemeCfg.ResolveTheme(s.Site.Lang)
}

func (s *Server) handleHistoryArticle(w http.ResponseWriter, r *http.Request, id, editDir string) {
	article, comments, err := s.Store.GetArticleVersion(id, editDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderArticleBodyWithComments(w, article, comments)
}

func (s *Server) serveHistoryFile(w http.ResponseWriter, r *http.Request, id, editDir, filename string) {
	filename = filepath.Base(filename)
	if filename == "." || filename == "/" {
		http.NotFound(w, r)
		return
	}

	path, err := s.Store.GetArticleVersionFilePath(id, editDir, filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Server) renderArticleBody(w http.ResponseWriter, article *blog.Article) {
	comments, err := s.Store.GetFilteredComments(article.UniqueID, blog.FilterOptions{
		ShowDeleted: s.Store.History.ShowDeleted,
		ShowReplies: s.Store.History.ShowReplies,
	})
	if err != nil {
		comments = nil
	}
	s.renderArticleBodyWithComments(w, article, comments)
}

func (s *Server) renderArticleBodyWithComments(w http.ResponseWriter, article *blog.Article, comments []*blog.Comment) {
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
		"CommentVisible": s.Store.History.Comment.Visible,
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
