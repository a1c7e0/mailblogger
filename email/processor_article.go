package email

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mailblogger/blog"
)

func (p *Processor) processArticle(raw *RawMessage) error {
	addr, _, displayName, authorHash := extractAuthorInfo(raw)

	uniqueID := genID(raw, addr, "")
	if p.Store.ArticleExists(uniqueID) {
		log.Printf("article %s already exists", uniqueID)
		return nil
	}

	date, _ := parseEmailDate(raw.Date)
	cleanSubj := cleanSubject(raw.Subject)
	slug := ""

	cleanSubj = stripNotifyTags(cleanSubj)

	bodyCfg, body, _ := parseBodyConfig(raw.Body)
	body = stripEmailQuotes(body)
	banner := ""
	notifyTag := ""
	if bodyCfg != nil {
		if s, ok := bodyCfg["slug"]; ok {
			s = strings.ToLower(strings.TrimSpace(s))
			if !validSlugRe.MatchString(s) {
				return p.sendErrorReply(raw, fmt.Sprintf("Invalid slug %q. Use lowercase letters, digits, dashes only.", s))
			}
			if _, err := p.Store.FindBySlug(s); err == nil {
				return p.sendErrorReply(raw, fmt.Sprintf("Slug %q is already in use.", s))
			}
			slug = s
		}
		if b, ok := bodyCfg["banner"]; ok {
			banner = strings.TrimSpace(b)
			if n, err := strconv.Atoi(banner); err != nil || n <= 0 {
				return p.sendErrorReply(raw, fmt.Sprintf("Invalid banner value %q. Must be a positive number.", banner))
			}
		}
		if n, ok := bodyCfg["notify"]; ok {
			notifyTag = strings.TrimSpace(n)
		}
		if t, ok := bodyCfg["title"]; ok {
			t = strings.TrimSpace(t)
			if t != "" {
				cleanSubj = t
			}
		}
	}

	article := &blog.Article{
		UniqueID:    uniqueID,
		Slug:        slug,
		Subject:     cleanSubj,
		Author:      displayName,
		AuthorHash:  authorHash,
		AuthorEmail: addr,
		Date:        date,
		Banner:      banner,
		Body:        body,
	}

	if !p.isWhitelisted(addr) {
		if err := p.Store.SaveDraft(article); err != nil {
			return fmt.Errorf("save draft: %w", err)
		}
		log.Printf("saved draft %s: %s", uniqueID, article.Subject)
		return p.sendDraftReply(raw, uniqueID)
	}

	// Create article directory before saving anything
	articleDir := p.Store.GetArticleDirName(article)
	if err := os.MkdirAll(articleDir, 0755); err != nil {
		return fmt.Errorf("create article dir: %w", err)
	}

	if len(raw.Images) > 0 {
		_, cidMap := saveArticleImages(p.Store, uniqueID, raw.Images, articleDir)
		if len(cidMap) > 0 {
			// Prefer plain text; if empty, convert HTML (which preserves CID image tags)
			if strings.TrimSpace(body) == "" && raw.HTMLBody != "" {
				body = htmlToMarkdown(raw.HTMLBody)
			}
			body = replaceCIDInBody(body, cidMap)
		}
	}

	// Resolve numeric image references to full filenames
	body = resolveImageNumbers(body, articleDir)
	article.Body = body

	if err := p.Store.SaveArticle(article); err != nil {
		return fmt.Errorf("save article: %w", err)
	}

	if notifyTag != "" {
		switch strings.ToLower(notifyTag) {
		case "on", "true", "watch":
			p.Store.AddWatcher(uniqueID, authorHash)
		case "off", "false", "mute":
			p.Store.AddMuter(uniqueID, authorHash)
		}
	}

	log.Printf("published article %s: %s", uniqueID, article.Subject)
	return nil
}

func (p *Processor) handleDeleteCommand(raw *RawMessage, article *blog.Article) error {
	if p.Store.History.Article.Keep {
		if err := p.Store.ArchiveArticle(article.UniqueID); err != nil {
			log.Printf("archive article %s failed: %v", article.UniqueID, err)
			return err
		}
		log.Printf("archived article %s by %s", article.UniqueID, raw.From.Address)
	} else {
		if p.Store.DeleteArticle(article.UniqueID) {
			log.Printf("deleted article %s by %s", article.UniqueID, raw.From.Address)
		}
	}
	return nil
}

func (p *Processor) handleEditCommand(raw *RawMessage, article *blog.Article) error {
	articleDir := p.Store.GetArticleDirName(article)

	// Archive current version before editing
	if p.Store.History.Article.Keep {
		if err := p.Store.ArchiveArticleVersion(article.UniqueID); err != nil {
			log.Printf("archive version %s failed: %v", article.UniqueID, err)
		}
	}

	bodyCfg, body, _ := parseBodyConfig(raw.Body)
	body = stripEmailQuotes(body)
	if bodyCfg != nil {
		if b, ok := bodyCfg["banner"]; ok {
			article.Banner = strings.TrimSpace(b)
		}
		if s, ok := bodyCfg["slug"]; ok {
			s = strings.ToLower(strings.TrimSpace(s))
			if validSlugRe.MatchString(s) {
				if _, err := p.Store.FindBySlug(s); err != nil {
					article.Slug = s
				}
			}
		}
		if t, ok := bodyCfg["title"]; ok {
			t = strings.TrimSpace(t)
			if t != "" {
				article.Subject = t
			}
		}
	}

	// Always delete old images, then save new ones if any
	existingImages, _ := p.Store.ListImages(article.UniqueID)
	for _, img := range existingImages {
		os.Remove(filepath.Join(articleDir, img))
	}

	if len(raw.Images) > 0 {
		_, cidMap := saveArticleImages(p.Store, article.UniqueID, raw.Images, articleDir)
		if len(cidMap) > 0 {
			if strings.TrimSpace(body) == "" && raw.HTMLBody != "" {
				body = htmlToMarkdown(raw.HTMLBody)
			}
			body = replaceCIDInBody(body, cidMap)
		}
	}

	// Resolve numeric image references to full filenames
	body = resolveImageNumbers(body, articleDir)
	article.Body = body

	if err := p.Store.SaveArticle(article); err != nil {
		log.Printf("edit article %s failed: %v", article.UniqueID, err)
		return err
	}

	// Handle notify preference from body config
	if bodyCfg != nil {
		if n, ok := bodyCfg["notify"]; ok {
			_, _, _, authorHash := extractAuthorInfo(raw)
			switch strings.ToLower(strings.TrimSpace(n)) {
			case "on", "true", "watch":
				p.Store.AddWatcher(article.UniqueID, authorHash)
			case "off", "false", "mute":
				p.Store.AddMuter(article.UniqueID, authorHash)
			}
		}
	}

	log.Printf("edited article %s by %s", article.UniqueID, raw.From.Address)
	return nil
}
