package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
	Site        config.SiteConfig
	Theme       config.ThemeConfig
}

func NewServer(cfg ServerConfig) (*Server, error) {
	funcMap := template.FuncMap{
		"renderMD":  renderMarkdown,
		"renderPlaintext": renderPlaintext,
		"mailto":    makeMailto,
		"rawHTML":   func(s string) template.HTML { return template.HTML(s) },
		"fmtDate":      fmtDate,
		"fmtDateTitle": fmtDateTitle,
		"datetimeISO":  datetimeISO,
		"excerpt":  excerpt,
		"commentImages": func(articleID, commentUID string) []string {
			imgs, _ := cfg.Store.ListCommentImages(articleID, commentUID)
			return imgs
		},
		"authorTooltip": func(authorHash, authorEmail string) string {
			return authorTooltipFn(cfg.Store, cfg.Site.ShowAuthor, authorHash, authorEmail)
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
		tmpl:        tmpl,
	}
	srv.detectAssets()
	return srv, nil
}

func (s *Server) getConfig() *config.Config {
	if s.configGetter != nil {
		return s.configGetter()
	}
	return &config.Config{}
}

func (s *Server) getSite() config.SiteConfig {
	return s.getConfig().Site
}

func (s *Server) getTheme() config.ThemeConfig {
	return s.getConfig().Theme
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// New SPA API endpoints
	mux.HandleFunc("GET /api/site", s.handleAPISite)
	mux.HandleFunc("GET /api/articles", s.handleAPIArticles)
	mux.HandleFunc("GET /api/article/{id}", s.handleAPIArticleDetail)
	mux.HandleFunc("GET /api/article/{id}/comments", s.handleAPIArticleComments)
	mux.HandleFunc("GET /api/locale", s.handleAPILocale)
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
	// Site config is read from configGetter, nothing to update locally
}

func (s *Server) SetConfigGetter(fn func() *config.Config) {
	s.configGetter = fn
}

func (s *Server) InvalidateFeedCache() {
	globalFeedCache.invalidate()
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

	site := s.getSite()
	data := map[string]interface{}{
		"Host":        s.Host,
		"Scheme":      s.Scheme,
		"EmailLocal":  s.EmailLocal,
		"EmailDomain": s.EmailDomain,
		"Site":        site,
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
		// Theme routes are normally served by the SPA shell, but article media
		// must remain directly accessible at /<article>/<filename>.
		if s.serveExistingArticleFile(w, r, path) {
			return
		}
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
	themeCfg := s.getTheme()
	site := s.getSite()

	// If per-language themes are configured and auto_lang is enabled
	if site.AutoLang && len(themeCfg.Themes) > 0 {
		lang := parseAcceptLanguage(r.Header.Get("Accept-Language"))
		if lang != "" {
			if theme, ok := themeCfg.Themes[lang]; ok {
				return theme
			}
		}
	}
	// Fallback to resolved theme (single theme or first in map)
	return themeCfg.ResolveTheme(site.Lang)
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

	site := s.getSite()
	data := map[string]interface{}{
		"Host":         s.Host,
		"Scheme":       s.Scheme,
		"EmailLocal":   s.EmailLocal,
		"EmailDomain":  s.EmailDomain,
		"Site":         site,
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
	if s.serveExistingArticleFile(w, r, strings.TrimPrefix(articleID+"/"+filename, "/")) {
		return
	}
	http.NotFound(w, r)
}

func (s *Server) serveExistingArticleFile(w http.ResponseWriter, r *http.Request, path string) bool {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return false
	}
	articleID, filename := parts[0], filepath.Base(parts[1])
	if filename == "." || filename == "/" {
		return false
	}

	dir, err := s.Store.GetArticleDir(articleID)
	if err != nil {
		// Try slug
		a, err2 := s.Store.GetArticleBySlug(articleID)
		if err2 != nil {
			return false
		}
		dir, err = s.Store.GetArticleDir(a.UniqueID)
		if err != nil {
			return false
		}
	}
	filePath := filepath.Join(dir, filename)
	if info, err := os.Stat(filePath); err != nil || info.IsDir() {
		return false
	}
	http.ServeFile(w, r, filePath)
	return true
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
		prefs = &blog.UserPrefs{ArticleNotify: true, CommentNotify: false, HideEmail: true}
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

	site := s.getSite()
	data := map[string]interface{}{
		"Site":        site,
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
