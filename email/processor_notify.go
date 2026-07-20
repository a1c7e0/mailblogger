package email

import (
	"fmt"
	"log"
	"strings"

	"mailblogger/blog"
)

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
