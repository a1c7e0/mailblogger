package email

import (
	"fmt"
	"log"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mailblogger/blog"
)

var plusRe = regexp.MustCompile(`^([^+]+)\+([^@]+)@.+$`)
var validSlugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
var settingsRe = regexp.MustCompile(`(?i)^(?:re|fwd?)?:?\s*settings$`)

const (
	maxQuoteLineLen   = 200
	maxNotifyBodyLen  = 6000
	maxAncestorDepth  = 4
	settingsTokenTTL  = 24 * time.Hour
)

type Processor struct {
	Store       *blog.Store
	EmailLocal  string
	EmailDomain string
	Host        string
	Scheme      string
	Whitelist   []string
	Sender      *SMTPSender
}

func NewProcessor(store *blog.Store, emailLocal, emailDomain, host, scheme string, whitelist []string, sender *SMTPSender) *Processor {
	return &Processor{
		Store:       store,
		EmailLocal:  emailLocal,
		EmailDomain: emailDomain,
		Host:        host,
		Scheme:      scheme,
		Whitelist:   whitelist,
		Sender:      sender,
	}
}

func (p *Processor) ProcessMessage(raw *RawMessage) error {
	if raw.RawBody != nil {
		ok, domain, err := VerifyDKIM(raw.RawBody)
		if err != nil {
			log.Printf("DKIM: %s = error (%v)", domain, err)
			p.sendErrorReply(raw, fmt.Sprintf("Email rejected: DKIM verification failed for %s.", domain))
			return fmt.Errorf("DKIM verification failed for %s: %w", domain, err)
		}
		if ok {
			log.Printf("DKIM: %s = pass", domain)
		} else if domain != "" {
			log.Printf("DKIM: %s = fail, rejected", domain)
			p.sendErrorReply(raw, fmt.Sprintf("Email rejected: DKIM signature invalid for %s.", domain))
			return fmt.Errorf("DKIM signature invalid for %s", domain)
		}
	}

	if p.isSettingsCommand(raw) {
		return p.handleSettingsCommand(raw)
	}

	if forwarded, err := p.handleHashForward(raw); err != nil {
		p.sendErrorReply(raw, "Failed to forward your email.")
		return err
	} else if forwarded {
		return nil
	}

	targetUID := p.parseTargetID(raw.To)
	if targetUID == "" {
		return p.processArticle(raw)
	}
	return p.processComment(raw, targetUID)
}

func (p *Processor) isSettingsCommand(raw *RawMessage) bool {
	subject := cleanSubject(raw.Subject)
	return settingsRe.MatchString(subject)
}

func (p *Processor) handleHashForward(raw *RawMessage) (bool, error) {
	if raw.From == nil || len(raw.To) == 0 {
		return false, nil
	}
	for _, to := range raw.To {
		local, domain, ok := parseEmail(to.Address)
		if !ok || domain != p.EmailDomain {
			continue
		}
		// Skip plus-addressed (handled by parseTargetID)
		if strings.Contains(local, "+") {
			continue
		}
		// Check if local part is an author hash
		_, found := p.Store.FindEmailByHash(local)
		if !found {
			continue
		}
		// Forward as plain passthrough — reconstruct as blog+hash@domain
		forwardTo := fmt.Sprintf("%s+%s@%s", p.EmailLocal, local, p.EmailDomain)

		msg := buildEmailMessage(raw.From.Address, forwardTo, raw.Subject, raw.From.Address, raw.Body)

		if err := p.Sender.Send(raw.From.Address, forwardTo, msg); err != nil {
			log.Printf("forward: failed to %s: %v", forwardTo, err)
			return false, fmt.Errorf("forward email: %w", err)
		}
		log.Printf("forward: %s → %s", raw.From.Address, forwardTo)
		return true, nil
	}
	return false, nil
}

func parseEmail(addr string) (local, domain string, ok bool) {
	parts := strings.SplitN(addr, "@", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (p *Processor) parseTargetID(toAddrs []*mail.Address) string {
	for _, addr := range toAddrs {
		match := plusRe.FindStringSubmatch(addr.Address)
		if len(match) == 3 {
			return match[2]
		}
	}
	return ""
}

func extractAuthorInfo(raw *RawMessage) (addr, name, displayName, authorHash string) {
	if raw.From != nil {
		addr = raw.From.Address
		name = raw.From.Name
	}
	displayName = blog.GenDisplayName(name, addr)
	authorHash = blog.GenAuthorHash(addr)
	return
}

func genID(raw *RawMessage, addr string, extra string) string {
	idInput := raw.MessageID
	if idInput == "" {
		idInput = fmt.Sprintf("%s-%s-%s", addr, raw.Subject, raw.Date)
	}
	return blog.GenUniqueID(idInput + extra)
}

func isDashLine(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 {
		return false
	}
	for _, c := range s {
		if c != '-' {
			return false
		}
	}
	return true
}

var knownConfigKeys = map[string]bool{
	"banner": true,
	"slug":   true,
	"notify": true,
	"title":  true,
}

func parseBodyConfig(body string) (cfg map[string]string, cleanBody string, err error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) < 3 || !isDashLine(lines[0]) {
		return nil, body, nil
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isDashLine(trimmed) {
			endIdx = i
			break
		}
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 || strings.Contains(parts[0], " ") || parts[0] == "" {
			return nil, body, nil
		}
		key := strings.TrimSpace(parts[0])
		if !knownConfigKeys[key] {
			return nil, body, nil
		}
	}
	if endIdx < 1 {
		return nil, body, nil
	}

	cfg = make(map[string]string)
	for i := 1; i < endIdx; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		cfg[key] = val
	}
	if len(cfg) == 0 {
		return nil, body, nil
	}

	rest := strings.Join(lines[endIdx+1:], "\n")
	return cfg, strings.TrimLeft(rest, "\n"), nil
}

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
	if slug != "" {
		if _, err := p.Store.FindBySlug(slug); err == nil {
			return p.sendErrorReply(raw, fmt.Sprintf("Slug %q is already in use.", slug))
		}
	}

	cleanSubj = stripNotifyTags(cleanSubj)

	bodyCfg, body, _ := parseBodyConfig(raw.Body)
	banner := ""
	notifyTag := ""
	if bodyCfg != nil {
		if s, ok := bodyCfg["slug"]; ok && slug == "" {
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
		if len(cidMap) > 0 && !strings.Contains(article.Body, "![") {
			if raw.HTMLBody != "" {
				article.Body = htmlToMarkdown(raw.HTMLBody)
			}
		}
		for cid, num := range cidMap {
			article.Body = strings.ReplaceAll(article.Body, "cid:"+cid, num)
		}
	}

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

func (p *Processor) processComment(raw *RawMessage, targetID string) error {
	addr, _, displayName, authorHash := extractAuthorInfo(raw)

	var parentID string
	var replyTo string

	article, err := p.Store.FindByUniqueID(targetID)
	if err == nil {
		parentID = article.UniqueID
		subj := strings.ToLower(cleanSubject(raw.Subject))
		if (subj == "delete" || subj == "edit") && raw.From != nil && raw.From.Address == article.AuthorEmail {
			if subj == "delete" {
				return p.handleDeleteCommand(raw, article)
			}
			return p.handleEditCommand(raw, article)
		}
	} else {
		exists, comment, articleID := p.Store.CommentExists(targetID)
		if exists {
			parentID = articleID
			subj := strings.ToLower(cleanSubject(raw.Subject))
			if (subj == "delete" || subj == "edit") && raw.From != nil && raw.From.Address == comment.AuthorEmail {
				if subj == "delete" {
					return p.handleDeleteCommentCommand(raw, comment, articleID)
				}
				return p.handleEditCommentCommand(raw, comment, articleID)
			}
			replyTo = comment.UniqueID
		} else {
			log.Printf("target ID %s not found, skipping comment", targetID)
			return p.sendErrorReply(raw, fmt.Sprintf("No article or comment found with ID %q.", targetID))
		}
	}

	if result := parseNotifyTag(raw.Subject); result.found {
		if result.notify {
			p.Store.AddWatcher(parentID, authorHash)
		} else {
			p.Store.AddMuter(parentID, authorHash)
		}
	}

	uniqueID := genID(raw, addr, parentID)
	if _, err := p.Store.FindComment(parentID, uniqueID); err == nil {
		log.Printf("comment %s already exists on article %s", uniqueID, parentID)
		return nil
	}

	date, _ := parseEmailDate(raw.Date)
	comment := &blog.Comment{
		UniqueID:    uniqueID,
		ParentID:    parentID,
		Author:      displayName,
		AuthorHash:  authorHash,
		AuthorEmail: addr,
		Date:        date,
		Body:        raw.Body,
		ReplyTo:     replyTo,
	}

	if len(raw.Images) > 0 {
		saveCommentImages(p.Store, parentID, uniqueID, raw.Images)
	}

	if err := p.Store.SaveComment(comment); err != nil {
		return fmt.Errorf("save comment: %w", err)
	}

	p.notifyReply(comment, parentID)

	log.Printf("saved comment %s on %s", uniqueID, parentID)
	return nil
}

func (p *Processor) isWhitelisted(addr string) bool {
	if len(p.Whitelist) == 0 {
		return true
	}
	for _, pattern := range p.Whitelist {
		if matchPattern(pattern, addr) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, addr string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == addr {
		return true
	}
	if strings.HasPrefix(pattern, "*@") {
		domain := pattern[2:]
		parts := strings.SplitN(addr, "@", 2)
		if len(parts) == 2 && parts[1] == domain {
			return true
		}
	}
	return false
}

func cleanSubject(subject string) string {
	prefixes := []string{"Re: ", "RE: ", "re: ", "Re:", "RE:", "re:", "Fwd: ", "FWD: ", "fwd: ", "Fwd:", "FWD:", "fwd:"}
	for {
		before := subject
		for _, prefix := range prefixes {
			if strings.HasPrefix(subject, prefix) {
				subject = subject[len(prefix):]
				break
			}
		}
		if subject == before {
			break
		}
	}
	return strings.TrimSpace(subject)
}

func parseEmailDate(dateStr string) (time.Time, error) {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700 (MST)",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 -0700",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, dateStr); err == nil {
			return t, nil
		}
	}
	return time.Now(), fmt.Errorf("cannot parse date: %s", dateStr)
}

func (p *Processor) notifyReply(c *blog.Comment, articleID string) {
	if c.ReplyTo == "" || p.Sender == nil || p.Sender.Server == "" {
		return
	}
	parentComment, err := p.Store.FindComment(articleID, c.ReplyTo)
	if err != nil || parentComment.AuthorEmail == "" {
		return
	}
	if parentComment.AuthorEmail == c.AuthorEmail {
		return
	}

	notify := p.Store.ShouldNotify(parentComment.AuthorHash, articleID, false)
	if !notify {
		return
	}

	subject := "Re: "
	if a, err := p.Store.GetArticle(articleID); err == nil {
		subject += a.Subject
	}

	replyAddr := fmt.Sprintf("%s+%s@%s", p.EmailLocal, c.UniqueID, p.EmailDomain)
	fromAddr := fmt.Sprintf("%s@%s", p.EmailLocal, p.EmailDomain)

	var body strings.Builder
	fmt.Fprintf(&body, "%s (#%s) wrote:\r\n", c.Author, c.UniqueID)
	writeQuoted(&body, c.Body)
	body.WriteString("\r\n")

	ancestors := p.collectAncestors(parentComment, maxAncestorDepth)
	for _, a := range ancestors {
		if body.Len() > maxNotifyBodyLen {
			body.WriteString("\r\n... (thread truncated)\r\n")
			break
		}
		fmt.Fprintf(&body, "\r\nIn reply to %s (#%s):\r\n", a.Author, a.UniqueID)
		writeQuoted(&body, a.Body)
	}

	body.WriteString("\r\n---\r\n")
	body.WriteString("Reply to this email to respond directly.\r\n")

	msg := buildEmailMessage(fromAddr, parentComment.AuthorEmail, subject, replyAddr, body.String())

	if err := p.Sender.Send(fromAddr, parentComment.AuthorEmail, msg); err != nil {
		log.Printf("notify failed: %v", err)
	} else {
		log.Printf("notified %s about reply %s", parentComment.AuthorEmail, c.UniqueID)
	}
}

func (p *Processor) sendErrorReply(raw *RawMessage, reason string) error {
	if raw.From == nil || p.Sender == nil || p.Sender.Server == "" {
		return nil
	}
	fromAddr := fmt.Sprintf("%s@%s", p.EmailLocal, p.EmailDomain)
	subject := "Re: " + cleanSubject(raw.Subject)
	body := reason + "\r\n\r\n---\r\nThis is an automated reply from MailBlogger.\r\n"
	msg := buildEmailMessage(fromAddr, raw.From.Address, subject, "", body)
	if err := p.Sender.Send(fromAddr, raw.From.Address, msg); err != nil {
		log.Printf("error reply to %s failed: %v", raw.From.Address, err)
	} else {
		log.Printf("error reply sent to %s: %s", raw.From.Address, reason)
	}
	return nil
}

func (p *Processor) sendDraftReply(raw *RawMessage, draftID string) error {
	if raw.From == nil || p.Sender == nil || p.Sender.Server == "" {
		return nil
	}
	fromAddr := fmt.Sprintf("%s@%s", p.EmailLocal, p.EmailDomain)
	subject := "Re: " + cleanSubject(raw.Subject)
	body := fmt.Sprintf("Your email was saved as a draft (ID: %s). The blog owner will review it.\r\n\r\n---\r\nThis is an automated reply from MailBlogger.\r\n", draftID)
	msg := buildEmailMessage(fromAddr, raw.From.Address, subject, "", body)
	if err := p.Sender.Send(fromAddr, raw.From.Address, msg); err != nil {
		log.Printf("draft reply failed: %v", err)
	}
	return nil
}

func (p *Processor) collectAncestors(start *blog.Comment, maxDepth int) []*blog.Comment {
	var result []*blog.Comment
	current := start
	for i := 0; i < maxDepth && current.ReplyTo != ""; i++ {
		parent, err := p.Store.FindComment(current.ParentID, current.ReplyTo)
		if err != nil {
			break
		}
		result = append(result, parent)
		current = parent
	}
	return result
}

func writeQuoted(buf *strings.Builder, body string) {
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if len(line) > maxQuoteLineLen {
			line = line[:maxQuoteLineLen] + "..."
		}
		buf.WriteString("> ")
		buf.WriteString(line)
		buf.WriteString("\r\n")
	}
}

func (p *Processor) handleDeleteCommand(raw *RawMessage, article *blog.Article) error {
	if p.Store.History.ArticleKeep {
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
	if p.Store.History.ArticleKeep {
		if err := p.Store.ArchiveArticleVersion(article.UniqueID); err != nil {
			log.Printf("archive version %s failed: %v", article.UniqueID, err)
		}
	}

	bodyCfg, body, _ := parseBodyConfig(raw.Body)
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
		if len(cidMap) > 0 && !strings.Contains(body, "![") {
			if raw.HTMLBody != "" {
				body = htmlToMarkdown(raw.HTMLBody)
			}
		}
		for cid, num := range cidMap {
			body = strings.ReplaceAll(body, "cid:"+cid, num)
		}
	}

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

func (p *Processor) handleEditCommentCommand(raw *RawMessage, comment *blog.Comment, articleID string) error {
	if err := p.Store.EditComment(articleID, comment.UniqueID, raw.Body); err != nil {
		log.Printf("edit comment %s failed: %v", comment.UniqueID, err)
		return err
	}
	log.Printf("edited comment %s by %s", comment.UniqueID, raw.From.Address)
	return nil
}

func (p *Processor) handleDeleteCommentCommand(raw *RawMessage, comment *blog.Comment, articleID string) error {
	if err := p.Store.DeleteComment(articleID, comment.UniqueID); err != nil {
		log.Printf("delete comment %s failed: %v", comment.UniqueID, err)
		return err
	}
	log.Printf("deleted comment %s by %s", comment.UniqueID, raw.From.Address)
	return nil
}

func (p *Processor) handleSettingsCommand(raw *RawMessage) error {
	addr, name, displayName, authorHash := extractAuthorInfo(raw)

	p.Store.CleanExpiredTokens()

	token, err := p.Store.CreateToken(authorHash, addr, displayName, settingsTokenTTL)
	if err != nil {
		log.Printf("settings: create token: %v", err)
		return fmt.Errorf("create settings token: %w", err)
	}

	settingsURL := fmt.Sprintf("%s://%s/settings?t=%s", p.Scheme, p.Host, token)

	subject := "Your MailBlogger settings link"
	var body strings.Builder
	fmt.Fprintf(&body, "Hi %s,\r\n\r\n", displayName)
	fmt.Fprintf(&body, "Click the link below to manage your notification settings:\r\n\r\n")
	fmt.Fprintf(&body, "%s\r\n\r\n", settingsURL)
	fmt.Fprintf(&body, "This link expires in 24 hours.\r\n")

	fromAddr := fmt.Sprintf("%s@%s", p.EmailLocal, p.EmailDomain)
	msg := buildEmailMessage(fromAddr, addr, subject, "", body.String())

	if err := p.Sender.Send(fromAddr, addr, msg); err != nil {
		log.Printf("settings: send reply failed: %v", err)
		return fmt.Errorf("send settings email: %w", err)
	}

	log.Printf("settings: sent link to %s (%s)", addr, name)
	return nil
}

var notifyTagRe = regexp.MustCompile(`(?i)\[(NOTIFY|WATCH|MUTE|NOWATCH)\]`)

type notifyResult struct {
	notify bool
	found  bool
}

func parseNotifyTag(subject string) notifyResult {
	m := notifyTagRe.FindStringSubmatch(subject)
	if len(m) == 0 {
		return notifyResult{}
	}
	tag := strings.ToUpper(m[1])
	if tag == "NOTIFY" || tag == "WATCH" {
		return notifyResult{notify: true, found: true}
	}
	return notifyResult{found: true}
}

func stripNotifyTags(subject string) string {
	s := notifyTagRe.ReplaceAllString(subject, "")
	return strings.TrimSpace(s)
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func buildEmailMessage(from, to, subject, replyTo, body string) string {
	var msg strings.Builder
	msg.WriteString("From: ")
	msg.WriteString(sanitize(from))
	msg.WriteString("\r\n")
	msg.WriteString("To: ")
	msg.WriteString(sanitize(to))
	msg.WriteString("\r\n")
	msg.WriteString("Subject: ")
	msg.WriteString(sanitize(subject))
	msg.WriteString("\r\n")
	if replyTo != "" {
		msg.WriteString("Reply-To: ")
		msg.WriteString(sanitize(replyTo))
		msg.WriteString("\r\n")
	}
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	return msg.String()
}
