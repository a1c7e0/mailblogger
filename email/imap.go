package email

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type Config struct {
	Server   string
	Port     int
	Username string
	Password string
}

type RawMessage struct {
	SeqNum    uint32
	From      *mail.Address
	To        []*mail.Address
	Subject   string
	Date      string
	MessageID string
	Body      string
	HTMLBody  string
	RawBody   []byte
	Images    []ImageData
}

func ConnectIMAP(cfg Config) (*client.Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Server, cfg.Port)
	tlsCfg := &tls.Config{ServerName: cfg.Server}
	c, err := client.DialTLS(addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("dial imap: %w", err)
	}
	if err := c.Login(cfg.Username, cfg.Password); err != nil {
		return nil, fmt.Errorf("login imap: %w", err)
	}
	return c, nil
}

func FetchUnseen(c *client.Client) ([]*RawMessage, error) {
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		return nil, fmt.Errorf("select inbox: %w", err)
	}

	if mbox.Messages == 0 {
		return nil, nil
	}

	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	if len(uids) == 0 {
		return nil, nil
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uids...)

	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, section.FetchItem()}

	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqSet, items, messages)
	}()

	var results []*RawMessage
	for msg := range messages {
		raw, err := parseMessage(msg)
		if err != nil {
			log.Printf("warning: parse message UID=%d: %v", msg.Uid, err)
			continue
		}
		raw.SeqNum = msg.SeqNum
		results = append(results, raw)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	return results, nil
}

func DeleteEmails(c *client.Client, seqNums []uint32) error {
	if len(seqNums) == 0 {
		return nil
	}
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqNums...)
	if err := c.Store(seqSet, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.DeletedFlag}, nil); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}
	if err := c.Expunge(nil); err != nil {
		return fmt.Errorf("expunge: %w", err)
	}
	return nil
}

func parseMessage(msg *imap.Message) (*RawMessage, error) {
	raw := &RawMessage{}

	if msg.Envelope != nil {
		if len(msg.Envelope.From) > 0 {
			raw.From = &mail.Address{
				Name:    msg.Envelope.From[0].PersonalName,
				Address: msg.Envelope.From[0].MailboxName + "@" + msg.Envelope.From[0].HostName,
			}
		}
		for _, to := range msg.Envelope.To {
			raw.To = append(raw.To, &mail.Address{
				Name:    to.PersonalName,
				Address: to.MailboxName + "@" + to.HostName,
			})
		}
		raw.Subject = decodeMIMEHeader(msg.Envelope.Subject)
		if !msg.Envelope.Date.IsZero() {
			raw.Date = msg.Envelope.Date.Format("Mon, 02 Jan 2006 15:04:05 -0700")
		}
		raw.MessageID = msg.Envelope.MessageId
	}

	for _, literal := range msg.Body {
		if literal != nil {
			data, err := io.ReadAll(literal)
			if err != nil {
				continue
			}
			raw.RawBody = data
			parsed, err := mail.ReadMessage(strings.NewReader(string(data)))
			if err == nil {
				raw.Body, raw.HTMLBody, raw.Images = parseBodyParts(parsed)
			}
			break
		}
	}

	return raw, nil
}

// parseBodyParts extracts body, HTML body, and images from a parsed mail.Message.
// This is the shared body extraction logic used by both ParseRawEmail and parseMessage.
func parseBodyParts(parsed *mail.Message) (body, html string, images []ImageData) {
	mediaType, params, _ := getContentType(parsed)

	if mediaType == "text/plain" {
		data, _ := io.ReadAll(parsed.Body)
		enc := strings.ToLower(parsed.Header.Get("Content-Transfer-Encoding"))
		if b := decodeBody(data, enc); b != "" {
			body = cleanBody(b)
		}
	} else if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			body, html, images = extractMultipartAll(parsed.Body, boundary, 0)
			if body != "" {
				body = cleanBody(body)
			}
		}
	} else if mediaType == "text/html" {
		data, _ := io.ReadAll(parsed.Body)
		enc := strings.ToLower(parsed.Header.Get("Content-Transfer-Encoding"))
		html = decodeBody(data, enc)
		body = cleanBody(htmlToMarkdown(html))
	}

	return
}

// ParseRawEmail parses a raw RFC 2822 email (e.g. from a Cloudflare Worker)
// into a RawMessage. This is the non-IMAP counterpart of parseMessage.
func ParseRawEmail(rawBytes []byte) (*RawMessage, error) {
	raw := &RawMessage{RawBody: rawBytes}

	parsed, err := mail.ReadMessage(strings.NewReader(string(rawBytes)))
	if err != nil {
		return nil, fmt.Errorf("parse email: %w", err)
	}

	if from, err := mail.ParseAddress(parsed.Header.Get("From")); err == nil {
		raw.From = from
	}
	if toList, err := mail.ParseAddressList(parsed.Header.Get("To")); err == nil {
		raw.To = toList
	}
	raw.Subject = decodeMIMEHeader(parsed.Header.Get("Subject"))
	raw.MessageID = parsed.Header.Get("Message-Id")
	if dateStr := parsed.Header.Get("Date"); dateStr != "" {
		if t, err := mail.ParseDate(dateStr); err == nil {
			raw.Date = t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
		}
	}

	raw.Body, raw.HTMLBody, raw.Images = parseBodyParts(parsed)

	return raw, nil
}

const maxMultipartDepth = 5
const maxParts = 100

func extractMultipartAll(body io.Reader, boundary string, depth int) (text, html string, images []ImageData) {
	if boundary == "" || depth >= maxMultipartDepth {
		return
	}
	mr := multipart.NewReader(body, boundary)
	firstHTML := ""
	partCount := 0
	imgOrder := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		partCount++
		if partCount > maxParts {
			break
		}
		ct, params, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if ct == "" {
			continue
		}

		if strings.HasPrefix(ct, "multipart/") {
			if subBoundary := params["boundary"]; subBoundary != "" {
				subText, subHTML, subImages := extractMultipartAll(part, subBoundary, depth+1)
				if subText != "" && text == "" {
					text = subText
				}
				if subHTML != "" && firstHTML == "" {
					firstHTML = subHTML
				}
				images = append(images, subImages...)
			}
			continue
		}

		enc := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))
		bodyBytes, _ := io.ReadAll(part)

		if strings.HasPrefix(ct, "image/") {
			imgOrder++
			var decoded []byte
			switch enc {
			case "base64":
				raw := strings.ReplaceAll(string(bodyBytes), "\r\n", "")
				raw = strings.ReplaceAll(raw, "\n", "")
				if d, err := base64.StdEncoding.DecodeString(raw); err == nil {
					decoded = d
				} else {
					decoded = bodyBytes
				}
			default:
				decoded = bodyBytes
			}
			if len(decoded) > 0 {
				cid := part.Header.Get("Content-ID")
				cid = strings.Trim(cid, "<>")
				filename := part.Header.Get("Content-Disposition")
				if idx := strings.Index(filename, "filename=\""); idx >= 0 {
					rest := filename[idx+10:]
					if q := strings.Index(rest, "\""); q >= 0 {
						filename = rest[:q]
					} else {
						filename = rest
					}
				} else {
					filename = fmt.Sprintf("img%d", imgOrder)
					if ext := extByType(ct); ext != "" {
						filename += ext
					}
				}
				images = append(images, ImageData{
					CID:          cid,
					OriginalName: filename,
					Data:         decoded,
					ContentType:  ct,
					PartOrder:    imgOrder,
				})
			}
			continue
		}

		decoded := decodeBody(bodyBytes, enc)
		if decoded == "" {
			continue
		}
		if ct == "text/plain" && text == "" {
			text = decoded
		}
		if ct == "text/html" && firstHTML == "" {
			firstHTML = decoded
		}
	}
	if text == "" && firstHTML != "" {
		text = htmlToMarkdown(firstHTML)
		html = firstHTML
	} else if firstHTML != "" {
		html = firstHTML
	}
	return
}

func getContentType(msg *mail.Message) (string, map[string]string, error) {
	ct := msg.Header.Get("Content-Type")
	if ct == "" {
		return "text/plain", nil, nil
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	return mediaType, params, err
}

func decodeBody(data []byte, enc string) string {
	body := string(data)
	switch enc {
	case "base64":
		raw := strings.ReplaceAll(string(data), "\r\n", "")
		raw = strings.ReplaceAll(raw, "\n", "")
		if d, err := base64.StdEncoding.DecodeString(raw); err == nil {
			body = string(d)
		}
	case "quoted-printable":
		r := quotedprintable.NewReader(strings.NewReader(string(data)))
		if d, err := io.ReadAll(r); err == nil {
			body = string(d)
		}
	}
	return strings.TrimSpace(body)
}

func cleanBody(body string) string {
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return body
}

var wroteRe = regexp.MustCompile(`(?im)^On\s+.+\s+wrote:`)
var dashLineRe = regexp.MustCompile(`^-{3}$`)

// stripEmailQuotes removes quoted reply content from email bodies.
// It preserves user-authored ">" quotes in the body, only stripping
// automatically generated reply templates and standard email quotes.
//
// Strategy:
// 1. Look for the reply-template marker: "> ---" followed by "Write your reply above this line"
// 2. If found, truncate from that marker to end of body
// 3. Otherwise, look for "On ... wrote:" header and strip everything from there
//    (handles email quote chains with ">" prefix and "---" separators)
func stripEmailQuotes(body string) string {
	lines := strings.Split(body, "\n")

	// Look for reply-template marker: "> ---" followed by instruction line
	for i, line := range lines {
		if strings.HasPrefix(line, "> ---") {
			// Check if next non-empty line is the instruction
			for j := i + 1; j < len(lines) && j < i+3; j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					continue
				}
				if strings.Contains(next, "Write your reply above this line") {
					result := strings.TrimSpace(strings.Join(lines[:i], "\n"))
					if result != "" {
						return result
					}
					break
				}
				break
			}
		}
	}

	// Look for "On ... wrote:" header — truncate from there
	// This covers email quote chains (">" prefix) and "---" separators
	for i, line := range lines {
		if wroteRe.MatchString(line) {
			result := strings.TrimSpace(strings.Join(lines[:i], "\n"))
			if result == "" {
				return body
			}
			return result
		}
	}

	// Look for standalone "---" separator (email signature divider)
	for i, line := range lines {
		if dashLineRe.MatchString(strings.TrimSpace(line)) {
			result := strings.TrimSpace(strings.Join(lines[:i], "\n"))
			if result == "" {
				return body
			}
			return result
		}
	}

	return body
}

var imgCIDRe = regexp.MustCompile(`(?i)<img\b[^>]*src="cid:([^"]+)"[^>]*>`)
var imgTagRe = regexp.MustCompile(`<img[^>]*src="([^"]+)"[^>]*alt="([^"]*)"[^>]*>`)
var htmlImgNoAltRe = regexp.MustCompile(`(?i)<img\b[^>]*src="([^"]+)"[^>]*>`)
var htmlTagRe = regexp.MustCompile(`(?i)</?(?:div|p|br|hr|h[1-6]|li|ul|ol|blockquote|pre|table|tr|td|th|thead|tbody|section|article|header|footer|nav|aside|figure|figcaption|dl|dt|dd)[^>]*>`)
var htmlAnyTagRe = regexp.MustCompile(`<[^>]+>`)

func htmlToMarkdown(html string) string {
	text := html
	// Step 1: Convert CID image tags to markdown (preserve for later CID replacement)
	text = imgCIDRe.ReplaceAllString(text, "![image](cid:$1)")
	// Step 2: Convert remaining image tags
	text = imgTagRe.ReplaceAllString(text, "![$2]($1)")
	text = htmlImgNoAltRe.ReplaceAllString(text, "![image]($1)")
	// Step 3: Convert block elements to newlines
	text = htmlTagRe.ReplaceAllString(text, "\n")
	// Step 4: Strip all remaining HTML tags
	text = htmlAnyTagRe.ReplaceAllString(text, "")
	// Step 5: Decode HTML entities
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	// Step 6: Collapse excessive newlines
	for {
		newText := strings.ReplaceAll(text, "\n\n\n", "\n\n")
		if newText == text {
			break
		}
		text = newText
	}
	return strings.TrimSpace(text)
}

// decodeMIMEHeader decodes MIME encoded-words in email headers (e.g. =?UTF-8?B?...?=).
// Returns the original string if decoding fails.
func decodeMIMEHeader(s string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}
