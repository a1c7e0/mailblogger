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

func MarkAsSeen(c *client.Client, seqNums []uint32) error {
	if len(seqNums) == 0 {
		return nil
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqNums...)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}
	if err := c.Store(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}
	return nil
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
		raw.Subject = msg.Envelope.Subject
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
			parseRawBody(raw)
			break
		}
	}

	return raw, nil
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
	raw.Subject = parsed.Header.Get("Subject")
	raw.MessageID = parsed.Header.Get("Message-Id")
	if dateStr := parsed.Header.Get("Date"); dateStr != "" {
		if t, err := mail.ParseDate(dateStr); err == nil {
			raw.Date = t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
		}
	}

	mediaType, params, _ := getContentType(parsed)
	if mediaType == "text/plain" {
		body, _ := io.ReadAll(parsed.Body)
		enc := strings.ToLower(parsed.Header.Get("Content-Transfer-Encoding"))
		if b := decodeBody(body, enc); b != "" {
			raw.Body = cleanBody(b)
		}
	} else if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			raw.Body, raw.HTMLBody, raw.Images = extractMultipartAll(parsed.Body, boundary, 0)
			if raw.Body != "" {
				raw.Body = cleanBody(raw.Body)
			}
		}
	} else if mediaType == "text/html" {
		body, _ := io.ReadAll(parsed.Body)
		enc := strings.ToLower(parsed.Header.Get("Content-Transfer-Encoding"))
		raw.HTMLBody = decodeBody(body, enc)
		raw.Body = cleanBody(htmlToMarkdown(raw.HTMLBody))
	}

	return raw, nil
}

// parseRawBody extracts body, HTML, and images from raw.RawBody.
func parseRawBody(raw *RawMessage) {
	parsed, err := mail.ReadMessage(strings.NewReader(string(raw.RawBody)))
	if err != nil {
		return
	}

	mediaType, params, _ := getContentType(parsed)

	if mediaType == "text/plain" {
		body, _ := io.ReadAll(parsed.Body)
		enc := strings.ToLower(parsed.Header.Get("Content-Transfer-Encoding"))
		if b := decodeBody(body, enc); b != "" {
			raw.Body = cleanBody(b)
		}
	} else if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			raw.Body, raw.HTMLBody, raw.Images = extractMultipartAll(parsed.Body, boundary, 0)
			if raw.Body != "" {
				raw.Body = cleanBody(raw.Body)
			}
		}
	} else if mediaType == "text/html" {
		body, _ := io.ReadAll(parsed.Body)
		enc := strings.ToLower(parsed.Header.Get("Content-Transfer-Encoding"))
		raw.HTMLBody = decodeBody(body, enc)
		raw.Body = cleanBody(htmlToMarkdown(raw.HTMLBody))
	}
}

func extractTextBody(msg *mail.Message) string {
	mediaType, params, err := getContentType(msg)
	if err != nil {
		return ""
	}

	if mediaType == "text/plain" {
		body, _ := io.ReadAll(msg.Body)
		enc := strings.ToLower(msg.Header.Get("Content-Transfer-Encoding"))
		return decodeBody(body, enc)
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		result, err := extractMultipart(msg.Body, boundary, 0)
		if err == nil {
			return result
		}
	}

	if mediaType == "text/html" {
		body, _ := io.ReadAll(msg.Body)
		enc := strings.ToLower(msg.Header.Get("Content-Transfer-Encoding"))
		return htmlToMarkdown(decodeBody(body, enc))
	}

	return ""
}

const maxMultipartDepth = 5
const maxParts = 100

func extractMultipart(body io.Reader, boundary string, depth int) (string, error) {
	if boundary == "" {
		return "", fmt.Errorf("empty boundary")
	}
	if depth >= maxMultipartDepth {
		return "", fmt.Errorf("multipart depth exceeded")
	}
	mr := multipart.NewReader(body, boundary)
	firstHTML := ""
	partCount := 0
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
				if sub, err := extractMultipart(part, subBoundary, depth+1); err == nil && sub != "" {
					if firstHTML == "" && isHTMLContentType(ct) {
						firstHTML = sub
					}
					if !isHTMLContentType(ct) {
						return sub, nil
					}
				}
			}
			continue
		}

		enc := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))
		bodyBytes, _ := io.ReadAll(part)
		decoded := decodeBody(bodyBytes, enc)
		if decoded == "" {
			continue
		}
		if ct == "text/plain" {
			return decoded, nil
		}
		if ct == "text/html" && firstHTML == "" {
			firstHTML = decoded
		}
	}
	if firstHTML != "" {
		return htmlToMarkdown(firstHTML), nil
	}
	return "", fmt.Errorf("no text/plain or text/html part found")
}

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

func isHTMLContentType(ct string) bool {
	return ct == "text/html" || ct == "multipart/alternative"
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

func scanMIMEPartsForHTML(body io.Reader, header mail.Header) string {
	ct := header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(ct)
	if mediaType == "text/html" {
		data, _ := io.ReadAll(body)
		enc := strings.ToLower(header.Get("Content-Transfer-Encoding"))
		return decodeBody(data, enc)
	}
	if !strings.HasPrefix(mediaType, "multipart/") || params["boundary"] == "" {
		return ""
	}
	mr := multipart.NewReader(body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		hdr := mail.Header(part.Header)
		if result := scanMIMEPartsForHTML(part, hdr); result != "" {
			return result
		}
	}
	return ""
}

func cleanBody(body string) string {
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = stripEmailQuotes(body)
	return body
}

var quoteRe = regexp.MustCompile(`(?im)^(On\s+.+\s+wrote:|[>\|]\s|^---$)`)
var imgTagRe = regexp.MustCompile(`<img[^>]*src="([^"]+)"[^>]*alt="([^"]*)"[^>]*>`)

func stripEmailQuotes(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if quoteRe.MatchString(line) {
			result := strings.TrimSpace(strings.Join(lines[:i], "\n"))
			if result == "" {
				return body
			}
			return result
		}
	}
	return body
}

var htmlStripRe = regexp.MustCompile(`(?i)<(html|body|head|meta[^>]*|!DOCTYPE[^>]*|\?xml[^>]*)\b[^>]*>`)
var htmlImgNoAltRe = regexp.MustCompile(`(?i)<img\b[^>]*src="([^"]+)"[^>]*>`)

func htmlToMarkdown(html string) string {
	text := html
	text = htmlStripRe.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "</html>", "")
	text = strings.ReplaceAll(text, "</body>", "")
	text = strings.ReplaceAll(text, "</head>", "")
	text = imgTagRe.ReplaceAllString(text, "![$2]($1)")
	text = htmlImgNoAltRe.ReplaceAllString(text, "![image]($1)")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "<p>", "\n")
	text = strings.ReplaceAll(text, "</p>", "\n")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	for {
		newText := strings.ReplaceAll(text, "\n\n\n", "\n\n")
		if newText == text {
			break
		}
		text = newText
	}
	return strings.TrimSpace(text)
}
