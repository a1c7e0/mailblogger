package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/mail"
	"net/http"
	"strings"
	"time"

	"mailblogger/blog"
	"mailblogger/email"
)

type APIArticleRequest struct {
	From     string     `json:"from"`
	To       string     `json:"to"`
	Subject  string     `json:"subject"`
	Body     string     `json:"body"`
	HTMLBody string     `json:"html_body,omitempty"`
	Images   []APIImage `json:"images,omitempty"`
	Date     string     `json:"date,omitempty"`
}

type APICommentRequest struct {
	From    string     `json:"from"`
	To      string     `json:"to"`
	Subject string     `json:"subject"`
	Body    string     `json:"body"`
	ReplyTo string     `json:"reply_to,omitempty"`
	Images  []APIImage `json:"images,omitempty"`
	Date    string     `json:"date,omitempty"`
}

type APIImage struct {
	Data        string `json:"data"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename,omitempty"`
}

type APIResponse struct {
	OK    bool   `json:"ok"`
	ID    string `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *Server) handleAPIArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB limit

	var req APIArticleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid json: " + err.Error()})
		return
	}

	raw, err := s.buildRawMessage(req.From, req.To, req.Subject, req.Body, req.HTMLBody, req.Images, req.Date)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, APIResponse{Error: err.Error()})
		return
	}

	processor := s.newProcessor()
	if err := processor.ProcessMessage(raw); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, APIResponse{Error: err.Error()})
		return
	}

	uniqueID := blog.GenUniqueID(raw.MessageID)
	s.writeJSON(w, http.StatusOK, APIResponse{OK: true, ID: uniqueID, Type: "article"})
}

func (s *Server) handleAPIComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB limit

	var req APICommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSON(w, http.StatusBadRequest, APIResponse{Error: "invalid json: " + err.Error()})
		return
	}

	toAddr := req.To
	if toAddr == "" && req.ReplyTo != "" {
		toAddr = fmt.Sprintf("%s+%s@%s", s.EmailLocal, req.ReplyTo, s.EmailDomain)
	}

	raw, err := s.buildRawMessage(req.From, toAddr, req.Subject, req.Body, "", req.Images, req.Date)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, APIResponse{Error: err.Error()})
		return
	}

	processor := s.newProcessor()
	if err := processor.ProcessMessage(raw); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, APIResponse{Error: err.Error()})
		return
	}

	addr := ""
	if raw.From != nil {
		addr = raw.From.Address
	}
	parentID := req.ReplyTo
	if parentID == "" {
		parts := strings.SplitN(toAddr, "+", 2)
		if len(parts) == 2 {
			parentID = strings.SplitN(parts[1], "@", 2)[0]
		}
	}
	uniqueID := blog.GenUniqueID(fmt.Sprintf("%s-%s-%s%s", addr, raw.Subject, raw.Date, parentID))

	s.writeJSON(w, http.StatusOK, APIResponse{OK: true, ID: uniqueID, Type: "comment"})
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"host":   s.Host,
	})
}

// GET /api/site.json
func (s *Server) handleAPISite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"lang":         s.Site.Lang,
		"show_author":  s.Site.ShowAuthor,
		"avatar":       s.Site.Avatar,
		"width":        s.Site.Width,
		"links":        s.Site.Links,
		"email_local":  s.EmailLocal,
		"email_domain": s.EmailDomain,
	})
}

// GET /api/articles.json
func (s *Server) handleAPIArticles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	articles, err := s.Store.ListArticles()
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, APIResponse{Error: err.Error()})
		return
	}
	type articleSummary struct {
		UniqueID   string `json:"uniqueid"`
		Slug       string `json:"slug,omitempty"`
		Subject    string `json:"subject"`
		Author     string `json:"author"`
		AuthorHash string `json:"author_hash"`
		Date       string `json:"date"`
		Banner     string `json:"banner,omitempty"`
		Excerpt    string `json:"excerpt"`
	}
	var result []articleSummary
	for _, a := range articles {
		result = append(result, articleSummary{
			UniqueID:   a.UniqueID,
			Slug:       a.Slug,
			Subject:    a.Subject,
			Author:     a.Author,
			AuthorHash: a.AuthorHash,
			Date:       a.Date.Format(time.RFC3339),
			Banner:     a.Banner,
			Excerpt:    excerpt(a.Body, 160),
		})
	}
	s.writeJSON(w, http.StatusOK, result)
}

// GET /api/article/{id}.json
func (s *Server) handleAPIArticleDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	article, err := s.Store.GetArticle(id)
	if err != nil {
		article, err = s.Store.GetArticleBySlug(id)
	}
	if err != nil {
		s.writeJSON(w, http.StatusNotFound, APIResponse{Error: "article not found"})
		return
	}
	images, _ := s.Store.ListImages(article.UniqueID)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"uniqueid":     article.UniqueID,
		"slug":         article.Slug,
		"subject":      article.Subject,
		"author":       article.Author,
		"author_hash":  article.AuthorHash,
		"author_email": article.AuthorEmail,
		"date":         article.Date.Format(time.RFC3339),
		"banner":       article.Banner,
		"body":         article.Body,
		"images":       images,
		"email_local":  s.EmailLocal,
		"email_domain": s.EmailDomain,
	})
}

// GET /api/article/{id}/comments.json
func (s *Server) handleAPIArticleComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	article, err := s.Store.GetArticle(id)
	if err != nil {
		article, err = s.Store.GetArticleBySlug(id)
	}
	if err != nil {
		s.writeJSON(w, http.StatusNotFound, APIResponse{Error: "article not found"})
		return
	}
	comments, err := s.Store.GetFilteredComments(article.UniqueID, blog.FilterOptions{
		ShowDeleted: s.Store.History.ShowDeleted,
		ShowReplies: s.Store.History.ShowReplies,
	})
	if err != nil {
		comments = nil
	}
	s.writeJSON(w, http.StatusOK, comments)
}

func (s *Server) handleAPIRawEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.configGetter()
	if cfg.Webhook.Secret == "" {
		s.writeJSON(w, http.StatusForbidden, APIResponse{Error: "webhook not configured"})
		return
	}
	if r.Header.Get("X-Webhook-Secret") != cfg.Webhook.Secret {
		s.writeJSON(w, http.StatusForbidden, APIResponse{Error: "invalid secret"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 35<<20) // 35 MiB for emails with attachments
	rawBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, APIResponse{Error: "read body: " + err.Error()})
		return
	}

	raw, err := email.ParseRawEmail(rawBytes)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, APIResponse{Error: err.Error()})
		return
	}

	processor := s.newProcessor()
	if err := processor.ProcessMessage(raw); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, APIResponse{Error: err.Error()})
		return
	}

	uniqueID := blog.GenUniqueID(raw.MessageID)
	s.writeJSON(w, http.StatusOK, APIResponse{OK: true, ID: uniqueID, Type: "email"})
}

func (s *Server) buildRawMessage(from, to, subject, body, htmlBody string, images []APIImage, dateStr string) (*email.RawMessage, error) {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}

	var toAddrs []*mail.Address
	if to != "" {
		toAddr, err := mail.ParseAddress(to)
		if err != nil {
			return nil, fmt.Errorf("invalid to address: %w", err)
		}
		toAddrs = append(toAddrs, toAddr)
	}

	date := time.Now()
	if dateStr != "" {
		if t, parseErr := time.Parse(time.RFC3339, dateStr); parseErr == nil {
			date = t
		}
	}

	var imgData []email.ImageData
	for i, img := range images {
		decoded, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			log.Printf("api: decode image %d: %v", i, err)
			continue
		}
		ct := img.ContentType
		if ct == "" {
			ct = "image/png"
		}
		imgData = append(imgData, email.ImageData{
			CID:         "",
			OriginalName: img.Filename,
			Data:        decoded,
			ContentType: ct,
			PartOrder:   i + 1,
		})
	}

	return &email.RawMessage{
		From:      addr,
		To:        toAddrs,
		Subject:   subject,
		Date:      date.Format("Mon, 02 Jan 2006 15:04:05 -0700"),
		MessageID: fmt.Sprintf("%d-%s@api", time.Now().UnixNano(), addr.Address),
		Body:      body,
		HTMLBody:  htmlBody,
		Images:    imgData,
	}, nil
}

func (s *Server) newProcessor() *email.Processor {
	cfg := s.configGetter()
	sender := &email.SMTPSender{}
	if cfg.Mail.SMTP.Server != "" && cfg.Mail.SMTP.Password != "" {
		sender = email.NewSMTPSender(cfg.Mail.SMTP.Server, cfg.Mail.SMTP.Port, cfg.Mail.SMTP.Username, cfg.Mail.SMTP.Password)
	}
	return email.NewProcessor(s.Store, s.EmailLocal, s.EmailDomain, s.Host, s.Scheme, nil, sender)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
