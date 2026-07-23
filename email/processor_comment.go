package email

import (
	"fmt"
	"log"
	"strings"

	"mailblogger/blog"
)

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
		Body:        stripEmailQuotes(raw.Body),
		ReplyTo:     replyTo,
	}

	if len(raw.Images) > 0 {
		_, cidMap := saveCommentImages(p.Store, parentID, uniqueID, raw.Images)
		if len(cidMap) > 0 {
			comment.Body = replaceCIDInBody(comment.Body, cidMap)
		}
	}

	// Resolve numeric image references to full filenames
	articleDir, _ := p.Store.GetArticleDir(parentID)
	if articleDir != "" {
		comment.Body = resolveImageNumbers(comment.Body, articleDir)
	}

	if err := p.Store.SaveComment(comment); err != nil {
		return fmt.Errorf("save comment: %w", err)
	}

	p.notifyReply(comment, parentID)

	log.Printf("saved comment %s on %s", uniqueID, parentID)
	return nil
}

func (p *Processor) handleEditCommentCommand(raw *RawMessage, comment *blog.Comment, articleID string) error {
	if err := p.Store.EditComment(articleID, comment.UniqueID, stripEmailQuotes(raw.Body)); err != nil {
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
