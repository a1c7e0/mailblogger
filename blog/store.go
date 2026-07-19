package blog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Store struct {
	ContentDir           string
	History              HistoryConfig
	mu                   sync.RWMutex
	once                 sync.Once
	hashMap              map[string]string
	slugMap              map[string]string
	cmtMap               map[string]string
	articleList          []*Article
	onChange             func()
	db                   *sql.DB
	defaultArticleNotify bool
	defaultCommentNotify bool
}

type HistoryConfig struct {
	ArticleKeep    bool
	ArticleVisible bool
	CommentKeep    bool
	CommentVisible bool
	ShowDeleted    bool
	ShowReplies    bool
}

func NewStore(contentDir string) (*Store, error) {
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return nil, err
	}
	s := &Store{ContentDir: contentDir}
	if err := s.initDB(); err != nil {
		return nil, fmt.Errorf("init db: %w", err)
	}
	return s, nil
}

func (s *Store) SetOnChange(fn func()) {
	s.onChange = fn
}

func (s *Store) SetDefaultNotify(article, comment bool) {
	s.defaultArticleNotify = article
	s.defaultCommentNotify = comment
}

func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
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
			if slug := parseDirSlug(e.Name()); slug != "" {
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

func (s *Store) SaveArticle(a *Article) error {
	dir := filepath.Join(s.ContentDir, articleDirName(a))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	fm := map[string]string{
		"uniqueid":     a.UniqueID,
		"subject":      a.Subject,
		"author":       a.Author,
		"author_hash":  a.AuthorHash,
		"author_email": a.AuthorEmail,
		"date":         a.Date.Format(time.RFC3339),
	}
	if a.Banner != "" {
		fm["banner"] = a.Banner
	}

	if err := writeFrontmatterFile(filepath.Join(dir, "index.md"), fm, a.Body); err != nil {
		return err
	}

	s.invalidateCache()
	return nil
}

func (s *Store) SaveComment(c *Comment) error {
	articleDir, err := s.findArticleDir(c.ParentID)
	if err != nil {
		return err
	}

	commentsPath := filepath.Join(articleDir, "comments.json")
	comments, _ := readCommentsJSON(commentsPath)
	comments = append(comments, c)
	if err := writeCommentsJSON(commentsPath, comments); err != nil {
		return err
	}

	s.mu.Lock()
	if s.cmtMap != nil {
		s.cmtMap[c.UniqueID] = c.ParentID
	}
	s.mu.Unlock()
	return nil
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

func (s *Store) readArticleDir(dir string) (*Article, error) {
	path := filepath.Join(dir, "index.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	meta, body, err := parseFrontmatter(data)
	if err != nil {
		return nil, err
	}
	a := &Article{
		UniqueID:    getStr(meta, "uniqueid"),
		Subject:     getStr(meta, "subject"),
		Author:      getStr(meta, "author"),
		AuthorHash:  getStr(meta, "author_hash"),
		AuthorEmail: getStr(meta, "author_email"),
		Banner:      getStr(meta, "banner"),
		Body:        body,
	}
	if ds := getStr(meta, "date"); ds != "" {
		a.Date, _ = time.Parse(time.RFC3339, ds)
	}
	a.Slug = parseDirSlug(filepath.Base(dir))
	return a, nil
}

func (s *Store) GetComments(articleID string) ([]*Comment, error) {
	articleDir, err := s.findArticleDir(articleID)
	if err != nil {
		return nil, err
	}

	commentsPath := filepath.Join(articleDir, "comments.json")
	comments, _ := readCommentsJSON(commentsPath)

	for _, c := range comments {
		if c.ReplyTo != "" && c.ReplyTo != articleID {
			c.Depth = 1
		}
	}

	return comments, nil
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
	comments, _ := readCommentsJSON(filepath.Join(articleDir, "comments.json"))
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

func (s *Store) DeleteArticle(hash string) bool {
	dir, err := s.findByHash(hash)
	if err != nil {
		return false
	}
	if err := os.RemoveAll(dir); err != nil {
		return false
	}
	s.invalidateCache()
	return true
}

// ArchiveArticle moves an article to _deleted/ directory
func (s *Store) ArchiveArticle(hash string) error {
	dir, err := s.findByHash(hash)
	if err != nil {
		return err
	}
	deletedDir := filepath.Join(s.ContentDir, "_deleted")
	if err := os.MkdirAll(deletedDir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(deletedDir, filepath.Base(dir))
	if err := os.Rename(dir, dest); err != nil {
		return err
	}
	s.invalidateCache()
	return nil
}

// ArchiveArticleVersion copies current article to edit_N/ before editing
func (s *Store) ArchiveArticleVersion(hash string) error {
	dir, err := s.findByHash(hash)
	if err != nil {
		return err
	}

	// Find next edit_N number
	n := 0
	for {
		editDir := filepath.Join(dir, fmt.Sprintf("edit_%d", n))
		if _, err := os.Stat(editDir); os.IsNotExist(err) {
			break
		}
		n++
	}

	editDir := filepath.Join(dir, fmt.Sprintf("edit_%d", n))
	if err := os.MkdirAll(editDir, 0755); err != nil {
		return err
	}

	// Copy index.md
	srcIndex := filepath.Join(dir, "index.md")
	if data, err := os.ReadFile(srcIndex); err == nil {
		os.WriteFile(filepath.Join(editDir, "index.md"), data, 0644)
	}

	// Copy comments.json
	srcComments := filepath.Join(dir, "comments.json")
	if data, err := os.ReadFile(srcComments); err == nil {
		os.WriteFile(filepath.Join(editDir, "comments.json"), data, 0644)
	}

	// Copy images
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if validImageExt(name) {
			data, _ := os.ReadFile(filepath.Join(dir, name))
			os.WriteFile(filepath.Join(editDir, name), data, 0644)
		}
	}

	return nil
}

func (s *Store) SaveDraft(a *Article) error {
	dir := filepath.Join(s.ContentDir, "_drafts", a.UniqueID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	fm := map[string]string{
		"uniqueid":     a.UniqueID,
		"subject":      a.Subject,
		"author":       a.Author,
		"author_hash":  a.AuthorHash,
		"author_email": a.AuthorEmail,
		"date":         a.Date.Format(time.RFC3339),
	}

	return writeFrontmatterFile(filepath.Join(dir, "index.md"), fm, a.Body)
}

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

func validImageExt(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

func (s *Store) GetArticleDir(articleID string) (string, error) {
	return s.findArticleDir(articleID)
}

func (s *Store) GetArticleDirName(a *Article) string {
	return filepath.Join(s.ContentDir, articleDirName(a))
}

func (s *Store) SaveImage(articleID, filename string, data []byte) error {
	dir, err := s.findArticleDir(articleID)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

func (s *Store) ListImages(articleID string) ([]string, error) {
	dir, err := s.findArticleDir(articleID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var images []string
	for _, e := range entries {
		if !e.IsDir() && validImageExt(e.Name()) {
			images = append(images, e.Name())
		}
	}
	sort.Strings(images)
	return images, nil
}

func (s *Store) ListCommentImages(articleID, commentUID string) ([]string, error) {
	dir, err := s.findArticleDir(articleID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	prefix := "c_" + commentUID + "_"
	var images []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && validImageExt(e.Name()) {
			images = append(images, e.Name())
		}
	}
	sort.Strings(images)
	return images, nil
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

func articleDirName(a *Article) string {
	name := a.Date.UTC().Format("20060102") + "_" + a.UniqueID
	if a.Slug != "" {
		name += "_" + a.Slug
	}
	return name
}

func parseDirHash(dirName string) string {
	parts := strings.SplitN(dirName, "_", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func parseDirSlug(dirName string) string {
	parts := strings.SplitN(dirName, "_", 3)
	if len(parts) == 3 {
		return parts[2]
	}
	return ""
}

func writeFrontmatterFile(path string, fm map[string]string, body string) error {
	content, err := formatFrontmatterBlock(fm, body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func formatFrontmatterBlock(fm map[string]string, body string) (string, error) {
	yamlBlock, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	return "---\n" + string(yamlBlock) + "---\n\n" + body, nil
}

// ParseFrontmatterExport is the exported version of parseFrontmatter
func ParseFrontmatterExport(data []byte) (map[string]string, string, error) {
	return parseFrontmatter(data)
}

// ParseDirSlugExport is the exported version of parseDirSlug
func ParseDirSlugExport(dirName string) string {
	return parseDirSlug(dirName)
}

func parseFrontmatter(data []byte) (map[string]string, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return map[string]string{}, text, nil
	}
	rest := text[4:]
	endIdx := strings.Index(rest, "\n---\n")
	closingLen := 5
	if endIdx == -1 {
		if strings.HasSuffix(rest, "\n---") {
			endIdx = len(rest) - 4
			closingLen = 4
		} else {
			return map[string]string{}, text, nil
		}
	}
	yamlBlock := rest[:endIdx]
	body := rest[endIdx+closingLen:]
	body = strings.TrimLeft(body, "\n")

	var meta map[string]string
	if err := yaml.Unmarshal([]byte(yamlBlock), &meta); err != nil {
		return map[string]string{}, body, nil
	}
	if meta == nil {
		meta = map[string]string{}
	}
	return meta, body, nil
}

func splitCommentBlocks(data []byte) [][]byte {
	var blocks [][]byte
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var current strings.Builder
	state := "none"

	isYAMLLike := func(idx int) bool {
		if idx >= len(lines) {
			return false
		}
		line := strings.TrimSpace(lines[idx])
		if line == "---" || line == "" {
			return false
		}
		return strings.Contains(line, ":") &&
			len(strings.SplitN(line, ":", 2)) == 2 &&
			!strings.HasPrefix(line, "- ") &&
			!strings.HasPrefix(line, "* ") &&
			!strings.HasPrefix(line, "# ")
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		switch state {
		case "none":
			if line == "---" {
				state = "frontmatter"
				current.Reset()
				current.WriteString(line)
			} else if line != "" {
				current.Reset()
				current.WriteString(line)
				state = "body"
			}
		case "frontmatter":
			current.WriteString("\n")
			current.WriteString(line)
			if line == "---" && current.String() != "---" {
				state = "body"
			}
		case "body":
			if line == "---" && isYAMLLike(i + 1) {
				blocks = append(blocks, []byte(strings.TrimSpace(current.String())))
				current.Reset()
				current.WriteString(line)
				state = "frontmatter"
			} else {
				current.WriteString("\n")
				current.WriteString(line)
			}
		}
	}
	if current.Len() > 0 {
		blocks = append(blocks, []byte(strings.TrimSpace(current.String())))
	}
	return blocks
}

func getStr(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	return m[key]
}

var avatarExts = []string{"png", "jpg", "webp", "svg"}

func (s *Store) EditComment(articleID, commentID, newBody string) error {
	articleDir, err := s.findArticleDir(articleID)
	if err != nil {
		return err
	}

	commentsPath := filepath.Join(articleDir, "comments.json")
	comments, err := readCommentsJSON(commentsPath)
	if err != nil {
		return err
	}

	for _, c := range comments {
		if c.UniqueID == commentID {
			if s.History.CommentKeep {
				c.Edits = append(c.Edits, CommentEdit{
					Date: c.Date,
					Body: c.Body,
				})
			}
			c.Body = newBody
			break
		}
	}

	return writeCommentsJSON(commentsPath, comments)
}

func (s *Store) DeleteComment(articleID, commentID string) error {
	articleDir, err := s.findArticleDir(articleID)
	if err != nil {
		return err
	}

	commentsPath := filepath.Join(articleDir, "comments.json")
	comments, err := readCommentsJSON(commentsPath)
	if err != nil {
		return err
	}

	for _, c := range comments {
		if c.UniqueID == commentID {
			c.Deleted = true
			c.Body = ""
			if !s.History.CommentKeep {
				c.Edits = nil
			}
			break
		}
	}

	return writeCommentsJSON(commentsPath, comments)
}

func readCommentsJSON(path string) ([]*Comment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var comments []*Comment
	if err := json.Unmarshal(data, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

func writeCommentsJSON(path string, comments []*Comment) error {
	data, err := json.MarshalIndent(comments, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *Store) DetectAvatar() string {
	for _, ext := range avatarExts {
		path := filepath.Join(s.ContentDir, "avatar."+ext)
		if _, err := os.Stat(path); err == nil {
			return "avatar." + ext
		}
	}
	return ""
}
