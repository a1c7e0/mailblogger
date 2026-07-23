package email

import (
	"fmt"
	"log"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"mailblogger/blog"
	"mailblogger/config"
)

var plusRe = regexp.MustCompile(`^([^+]+)\+([^@]+)@.+$`)
var validSlugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
var settingsRe = regexp.MustCompile(`(?i)^(?:re|fwd?)?:?\s*settings$`)

const (
	maxQuoteLineLen  = 200
	maxNotifyBodyLen = 6000
	maxAncestorDepth = 4
	settingsTokenTTL = 24 * time.Hour
)

type Processor struct {
	Store       *blog.Store
	EmailLocal  string
	EmailDomain string
	Host        string
	Scheme      string
	Whitelist   []string
	Sender      *SMTPSender
	DKIMPolicy  config.DKIMPolicy
}

func NewProcessor(store *blog.Store, emailLocal, emailDomain, host, scheme string, whitelist []string, sender *SMTPSender, dkimPolicy config.DKIMPolicy) *Processor {
	return &Processor{
		Store:       store,
		EmailLocal:  emailLocal,
		EmailDomain: emailDomain,
		Host:        host,
		Scheme:      scheme,
		Whitelist:   whitelist,
		Sender:      sender,
		DKIMPolicy:  dkimPolicy,
	}
}

func (p *Processor) ProcessMessage(raw *RawMessage) error {
	// Parse target ID early so DKIM policy can differ for articles vs comments
	targetUID := p.parseTargetID(raw.To)

	if raw.RawBody != nil && p.DKIMPolicy != config.DKIMNone {
		ok, domain, err := VerifyDKIM(raw.RawBody)
		if err != nil {
			log.Printf("DKIM: %s = error (%v)", domain, err)
			p.sendErrorReply(raw, fmt.Sprintf("Email rejected: DKIM verification failed for %s.", domain))
			return fmt.Errorf("DKIM verification failed for %s: %w", domain, err)
		}
		if ok {
			log.Printf("DKIM: %s = pass", domain)
		} else if domain != "" {
			// Invalid signature — always reject (normal + strict)
			log.Printf("DKIM: %s = fail, rejected", domain)
			p.sendErrorReply(raw, fmt.Sprintf("Email rejected: DKIM signature invalid for %s.", domain))
			return fmt.Errorf("DKIM signature invalid for %s", domain)
		} else if p.DKIMPolicy == config.DKIMStrict && targetUID != "" {
			// Strict mode: reject unsigned comments (articles protected by whitelist)
			log.Printf("DKIM: unsigned comment rejected (strict mode)")
			p.sendErrorReply(raw, "Email rejected: DKIM signature required for comments.")
			return fmt.Errorf("DKIM signature required (strict mode)")
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
		if strings.Contains(local, "+") {
			continue
		}
		_, found := p.Store.FindEmailByHash(local)
		if !found {
			continue
		}
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

var configBlockRe = regexp.MustCompile(`^-{3,}config\s*$`)

func isConfigBlockStart(s string) bool {
	return configBlockRe.MatchString(strings.TrimSpace(s))
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
	if len(lines) < 3 || !isConfigBlockStart(lines[0]) {
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

// replaceCIDInBody replaces all CID image references in the body with image numbers.
// Handles both markdown image syntax ![...](cid:xxx) and bare cid:xxx references.
func replaceCIDInBody(body string, cidMap map[string]string) string {
	for cid, num := range cidMap {
		body = strings.ReplaceAll(body, "![image](cid:"+cid+")", num)
		body = strings.ReplaceAll(body, "cid:"+cid, num)
	}
	return body
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
