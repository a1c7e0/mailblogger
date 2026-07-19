package blog

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Token struct {
	AuthorHash  string
	AuthorEmail string
	AuthorName  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type UserPrefs struct {
	ArticleNotify bool
	CommentNotify bool
	HideEmail     bool
	UpdatedAt     time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS settings_tokens (
    token       TEXT PRIMARY KEY,
    author_hash TEXT NOT NULL,
    author_email TEXT NOT NULL,
    author_name TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_prefs (
    author_hash    TEXT PRIMARY KEY,
    article_notify INTEGER NOT NULL DEFAULT 1,
    comment_notify INTEGER NOT NULL DEFAULT 0,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS article_watchers (
    article_id  TEXT NOT NULL,
    author_hash TEXT NOT NULL,
    PRIMARY KEY (article_id, author_hash)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS article_muters (
    article_id  TEXT NOT NULL,
    author_hash TEXT NOT NULL,
    PRIMARY KEY (article_id, author_hash)
) WITHOUT ROWID;
`

const migrateV2 = `
ALTER TABLE user_prefs ADD COLUMN hide_email INTEGER NOT NULL DEFAULT 1;
`

func (s *Store) initDB() error {
	dbPath := filepath.Join(s.ContentDir, "mailblogger.db")

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", dbPath))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("ping db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return fmt.Errorf("set WAL: %w", err)
	}

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return fmt.Errorf("set busy_timeout: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return fmt.Errorf("create schema: %w", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return fmt.Errorf("query user_version: %w", err)
	}
	if version < 1 {
		if _, err := db.Exec("PRAGMA user_version=1"); err != nil {
			db.Close()
			return fmt.Errorf("set user_version=1: %w", err)
		}
	}
	if version < 2 {
		if _, err := db.Exec(migrateV2); err != nil {
			db.Close()
			return fmt.Errorf("migrate v2: %w", err)
		}
		if _, err := db.Exec("PRAGMA user_version=2"); err != nil {
			db.Close()
			return fmt.Errorf("set user_version=2: %w", err)
		}
	}

	s.db = db
	return nil
}

func (s *Store) generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) CreateToken(authorHash, authorEmail, authorName string, ttl time.Duration) (string, error) {
	token, err := s.generateToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	_, err = s.db.Exec(
		"INSERT INTO settings_tokens (token, author_hash, author_email, author_name, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		token, authorHash, authorEmail, authorName, now.Format(time.RFC3339), expires.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	return token, nil
}

func (s *Store) GetToken(token string) (*Token, error) {
	row := s.db.QueryRow(
		"SELECT author_hash, author_email, author_name, created_at, expires_at FROM settings_tokens WHERE token = ?",
		token,
	)
	var t Token
	var createdAt, expiresAt string
	err := row.Scan(&t.AuthorHash, &t.AuthorEmail, &t.AuthorName, &createdAt, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query token: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	if time.Now().UTC().After(t.ExpiresAt) {
		s.db.Exec("DELETE FROM settings_tokens WHERE token = ?", token)
		return nil, nil
	}
	return &t, nil
}

func (s *Store) CleanExpiredTokens() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec("DELETE FROM settings_tokens WHERE expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) GetPrefs(authorHash string) (*UserPrefs, error) {
	row := s.db.QueryRow(
		"SELECT article_notify, comment_notify, hide_email, updated_at FROM user_prefs WHERE author_hash = ?",
		authorHash,
	)
	var p UserPrefs
	var updatedAt string
	err := row.Scan(&p.ArticleNotify, &p.CommentNotify, &p.HideEmail, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query prefs: %w", err)
	}
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}

func (s *Store) SavePrefs(authorHash string, prefs *UserPrefs) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO user_prefs (author_hash, article_notify, comment_notify, hide_email, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(author_hash) DO UPDATE SET
		   article_notify = excluded.article_notify,
		   comment_notify = excluded.comment_notify,
		   hide_email = excluded.hide_email,
		   updated_at = excluded.updated_at`,
		authorHash, prefs.ArticleNotify, prefs.CommentNotify, prefs.HideEmail, now,
	)
	return err
}

func (s *Store) AddWatcher(articleID, authorHash string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO article_watchers (article_id, author_hash) VALUES (?, ?)",
		articleID, authorHash,
	)
	return err
}

func (s *Store) RemoveWatcher(articleID, authorHash string) error {
	_, err := s.db.Exec(
		"DELETE FROM article_watchers WHERE article_id = ? AND author_hash = ?",
		articleID, authorHash,
	)
	return err
}

func (s *Store) AddMuter(articleID, authorHash string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO article_muters (article_id, author_hash) VALUES (?, ?)",
		articleID, authorHash,
	)
	return err
}

func (s *Store) RemoveMuter(articleID, authorHash string) error {
	_, err := s.db.Exec(
		"DELETE FROM article_muters WHERE article_id = ? AND author_hash = ?",
		articleID, authorHash,
	)
	return err
}

func (s *Store) IsWatcher(articleID, authorHash string) bool {
	var count int
	s.db.QueryRow(
		"SELECT COUNT(*) FROM article_watchers WHERE article_id = ? AND author_hash = ?",
		articleID, authorHash,
	).Scan(&count)
	return count > 0
}

func (s *Store) IsMuter(articleID, authorHash string) bool {
	var count int
	s.db.QueryRow(
		"SELECT COUNT(*) FROM article_muters WHERE article_id = ? AND author_hash = ?",
		articleID, authorHash,
	).Scan(&count)
	return count > 0
}

func (s *Store) ShouldNotify(authorHash, articleID string, isArticle bool) bool {
	if s.IsMuter(articleID, authorHash) {
		return false
	}
	if s.IsWatcher(articleID, authorHash) {
		return true
	}
	prefs, err := s.GetPrefs(authorHash)
	if err == nil && prefs != nil {
		if isArticle {
			return prefs.ArticleNotify
		}
		return prefs.CommentNotify
	}
	return s.defaultNotify(isArticle)
}

func (s *Store) defaultNotify(isArticle bool) bool {
	if isArticle {
		return s.defaultArticleNotify
	}
	return s.defaultCommentNotify
}

func (s *Store) FindEmailByHash(hash string) (string, bool) {
	var email string
	err := s.db.QueryRow(
		"SELECT author_email FROM settings_tokens WHERE author_hash = ? LIMIT 1", hash,
	).Scan(&email)
	if err == nil && email != "" {
		return email, true
	}
	articles := s.ListArticlesByAuthor(hash)
	if len(articles) > 0 && articles[0].AuthorEmail != "" {
		return articles[0].AuthorEmail, true
	}
	return "", false
}

func (s *Store) ShouldHideEmail(authorHash string, globalDefault bool) bool {
	prefs, err := s.GetPrefs(authorHash)
	if err == nil && prefs != nil {
		return prefs.HideEmail
	}
	return globalDefault
}
