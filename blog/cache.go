package blog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FilterOptions controls comment filtering behavior.
type FilterOptions struct {
	ShowDeleted bool
	ShowReplies bool
}

func (s *Store) buildCache() {
	s.once.Do(func() {
		s.hashMap = make(map[string]string)
		s.slugMap = make(map[string]string)
		s.cmtMap = make(map[string]string)
		entries, err := os.ReadDir(s.ContentDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			h := parseDirHash(e.Name())
			if h != "" {
				s.hashMap[h] = e.Name()
				s.indexComments(h, e.Name())
			}
			if slug := ParseDirSlug(e.Name()); slug != "" {
				s.slugMap[slug] = e.Name()
			}
		}
		s.rebuildArticleList()
	})
}

func (s *Store) indexComments(articleID, dirName string) {
	dir := filepath.Join(s.ContentDir, dirName)
	comments, err := readCommentsJSON(filepath.Join(dir, "comments.json"))
	if err != nil {
		return
	}
	for _, c := range comments {
		if c.UniqueID != "" {
			s.cmtMap[c.UniqueID] = articleID
		}
	}
}

func (s *Store) invalidateCache() {
	s.mu.Lock()
	s.once = sync.Once{}
	s.hashMap = nil
	s.slugMap = nil
	s.cmtMap = nil
	s.articleList = nil
	s.mu.Unlock()
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *Store) rebuildArticleList() {
	entries, err := os.ReadDir(s.ContentDir)
	if err != nil {
		return
	}
	var articles []*Article
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		a, err := s.readArticleDir(filepath.Join(s.ContentDir, entry.Name()))
		if err != nil {
			continue
		}
		articles = append(articles, a)
	}
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].Date.After(articles[j].Date)
	})
	s.articleList = articles
}

func (s *Store) GetArticle(hash string) (*Article, error) {
	dir, err := s.findByHash(hash)
	if err != nil {
		return nil, err
	}
	return s.readArticleDir(dir)
}

func (s *Store) GetArticleBySlug(slug string) (*Article, error) {
	dir, err := s.findBySlug(slug)
	if err != nil {
		return nil, err
	}
	return s.readArticleDir(dir)
}

func (s *Store) GetComments(articleID string) ([]*Comment, error) {
	articleDir, err := s.findArticleDir(articleID)
	if err != nil {
		return nil, err
	}

	commentsPath := filepath.Join(articleDir, "comments.json")
	if _, err := os.Stat(commentsPath); os.IsNotExist(err) {
		return []*Comment{}, nil
	}
	comments, err := readCommentsJSON(commentsPath)
	if err != nil {
		return nil, fmt.Errorf("parse comments.json: %w", err)
	}

	for _, c := range comments {
		if c.ReplyTo != "" && c.ReplyTo != articleID {
			c.Depth = 1
		}
	}

	return comments, nil
}

// GetFilteredComments returns comments for an article, filtered by the given options.
func (s *Store) GetFilteredComments(articleID string, opts FilterOptions) ([]*Comment, error) {
	comments, err := s.GetComments(articleID)
	if err != nil {
		return nil, err
	}

	var filtered []*Comment
	deletedIDs := make(map[string]bool)
	for _, c := range comments {
		if c.Deleted {
			deletedIDs[c.UniqueID] = true
			if opts.ShowDeleted {
				filtered = append(filtered, c)
			}
		} else {
			filtered = append(filtered, c)
		}
	}

	if !opts.ShowReplies {
		var withReplies []*Comment
		for _, c := range filtered {
			if c.ReplyTo != "" && c.ReplyTo != articleID && deletedIDs[c.ReplyTo] {
				continue
			}
			withReplies = append(withReplies, c)
		}
		filtered = withReplies
	}

	return filtered, nil
}

func (s *Store) ListArticlesPaged(page, perPage int) ([]*Article, int, error) {
	all, err := s.ListArticles()
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	start := (page - 1) * perPage
	if start >= total {
		return []*Article{}, total, nil
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

func (s *Store) ListArticles() ([]*Article, error) {
	s.buildCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Article, len(s.articleList))
	copy(result, s.articleList)
	return result, nil
}

func (s *Store) ListArticlesByAuthor(authorHash string) []*Article {
	s.buildCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Article
	for _, a := range s.articleList {
		if a.AuthorHash == authorHash {
			result = append(result, a)
		}
	}
	return result
}

func (s *Store) FindByUniqueID(uniqueID string) (*Article, error) {
	return s.GetArticle(uniqueID)
}

func (s *Store) FindBySlug(slug string) (*Article, error) {
	dir, err := s.findBySlug(slug)
	if err != nil {
		return nil, err
	}
	return s.readArticleDir(dir)
}

func (s *Store) FindComment(articleID, commentID string) (*Comment, error) {
	articleDir, err := s.findArticleDir(articleID)
	if err != nil {
		return nil, err
	}
	comments, err := readCommentsJSON(filepath.Join(articleDir, "comments.json"))
	if err != nil {
		return nil, fmt.Errorf("parse comments.json: %w", err)
	}
	for _, c := range comments {
		if c.UniqueID == commentID {
			return c, nil
		}
	}
	return nil, fmt.Errorf("comment %s not found", commentID)
}

func (s *Store) ArticleExists(hash string) bool {
	_, err := s.findByHash(hash)
	return err == nil
}

func (s *Store) CommentExists(commentID string) (bool, *Comment, string) {
	s.buildCache()
	s.mu.RLock()
	articleID, ok := s.cmtMap[commentID]
	s.mu.RUnlock()
	if !ok {
		return false, nil, ""
	}
	c, err := s.FindComment(articleID, commentID)
	if err != nil {
		return false, nil, ""
	}
	return true, c, articleID
}

func (s *Store) findArticleDir(needle string) (string, error) {
	dir, err := s.findByHash(needle)
	if err == nil {
		return dir, nil
	}
	// needle might be a comment ID — check the comment cache
	s.buildCache()
	s.mu.RLock()
	articleID, ok := s.cmtMap[needle]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("article for %s not found", needle)
	}
	return s.findByHash(articleID)
}

func (s *Store) findByHash(hash string) (string, error) {
	s.buildCache()
	s.mu.RLock()
	dir, ok := s.hashMap[hash]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("article %s not found", hash)
	}
	return filepath.Join(s.ContentDir, dir), nil
}

func (s *Store) findBySlug(slug string) (string, error) {
	s.buildCache()
	s.mu.RLock()
	dir, ok := s.slugMap[slug]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("slug %s not found", slug)
	}
	return filepath.Join(s.ContentDir, dir), nil
}
