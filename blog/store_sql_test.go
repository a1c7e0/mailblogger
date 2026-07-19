package blog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mailblogger-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	store, err := NewStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("create store: %v", err)
	}
	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}
	return store, cleanup
}

func TestCreateAndGetToken(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	token, err := store.CreateToken("abc123", "test@example.com", "Test User", time.Hour)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	got, err := store.GetToken(token)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got == nil {
		t.Fatal("token not found")
	}
	if got.AuthorHash != "abc123" {
		t.Errorf("AuthorHash = %q, want abc123", got.AuthorHash)
	}
	if got.AuthorEmail != "test@example.com" {
		t.Errorf("AuthorEmail = %q, want test@example.com", got.AuthorEmail)
	}
	if got.AuthorName != "Test User" {
		t.Errorf("AuthorName = %q, want Test User", got.AuthorName)
	}
}

func TestGetExpiredToken(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	token, err := store.CreateToken("abc123", "test@example.com", "Test User", -time.Hour)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	got, err := store.GetToken(token)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got != nil {
		t.Error("expired token should return nil")
	}
}

func TestGetNonexistentToken(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	got, err := store.GetToken("nonexistent")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got != nil {
		t.Error("nonexistent token should return nil")
	}
}

func TestSaveAndGetPrefs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	prefs := &UserPrefs{
		ArticleNotify: true,
		CommentNotify: false,
		HideEmail:     true,
	}
	if err := store.SavePrefs("abc123", prefs); err != nil {
		t.Fatalf("SavePrefs: %v", err)
	}

	got, err := store.GetPrefs("abc123")
	if err != nil {
		t.Fatalf("GetPrefs: %v", err)
	}
	if got == nil {
		t.Fatal("prefs not found")
	}
	if !got.ArticleNotify {
		t.Error("ArticleNotify should be true")
	}
	if got.CommentNotify {
		t.Error("CommentNotify should be false")
	}
	if !got.HideEmail {
		t.Error("HideEmail should be true")
	}
}

func TestGetNonexistentPrefs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	got, err := store.GetPrefs("nonexistent")
	if err != nil {
		t.Fatalf("GetPrefs: %v", err)
	}
	if got != nil {
		t.Error("nonexistent prefs should return nil")
	}
}

func TestUpdatePrefs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	prefs1 := &UserPrefs{ArticleNotify: true, CommentNotify: false, HideEmail: true}
	store.SavePrefs("abc123", prefs1)

	prefs2 := &UserPrefs{ArticleNotify: false, CommentNotify: true, HideEmail: false}
	store.SavePrefs("abc123", prefs2)

	got, _ := store.GetPrefs("abc123")
	if got.ArticleNotify {
		t.Error("ArticleNotify should be false after update")
	}
	if !got.CommentNotify {
		t.Error("CommentNotify should be true after update")
	}
	if got.HideEmail {
		t.Error("HideEmail should be false after update")
	}
}

func TestWatcherAndMuter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	if err := store.AddWatcher("article1", "user1"); err != nil {
		t.Fatalf("AddWatcher: %v", err)
	}

	if !store.IsWatcher("article1", "user1") {
		t.Error("user1 should be a watcher of article1")
	}
	if store.IsWatcher("article1", "user2") {
		t.Error("user2 should not be a watcher of article1")
	}

	if err := store.RemoveWatcher("article1", "user1"); err != nil {
		t.Fatalf("RemoveWatcher: %v", err)
	}
	if store.IsWatcher("article1", "user1") {
		t.Error("user1 should not be a watcher after removal")
	}

	if err := store.AddMuter("article1", "user2"); err != nil {
		t.Fatalf("AddMuter: %v", err)
	}
	if !store.IsMuter("article1", "user2") {
		t.Error("user2 should be a muter of article1")
	}

	if err := store.RemoveMuter("article1", "user2"); err != nil {
		t.Fatalf("RemoveMuter: %v", err)
	}
	if store.IsMuter("article1", "user2") {
		t.Error("user2 should not be a muter after removal")
	}
}

func TestShouldNotify(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SetDefaultNotify(true, false)

	store.AddWatcher("article1", "user1")
	if !store.ShouldNotify("user1", "article1", false) {
		t.Error("watcher should be notified")
	}

	store.AddMuter("article1", "user2")
	if store.ShouldNotify("user2", "article1", false) {
		t.Error("muter should not be notified")
	}

	store.SavePrefs("user3", &UserPrefs{ArticleNotify: true, CommentNotify: false})
	if !store.ShouldNotify("user3", "article1", true) {
		t.Error("user with article notify should be notified")
	}
	if store.ShouldNotify("user3", "article1", false) {
		t.Error("user without comment notify should not be notified")
	}

	if !store.ShouldNotify("unknown", "article1", true) {
		t.Error("unknown user should use default (article=true)")
	}
	if store.ShouldNotify("unknown", "article1", false) {
		t.Error("unknown user should use default (comment=false)")
	}
}

func TestFindEmailByHash(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.CreateToken("abc123", "test@example.com", "Test User", time.Hour)

	email, found := store.FindEmailByHash("abc123")
	if !found {
		t.Error("should find email for existing hash")
	}
	if email != "test@example.com" {
		t.Errorf("email = %q, want test@example.com", email)
	}

	_, found = store.FindEmailByHash("nonexistent")
	if found {
		t.Error("should not find email for nonexistent hash")
	}
}

func TestShouldHideEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	if !store.ShouldHideEmail("unknown", true) {
		t.Error("unknown user should use global default (true)")
	}
	if store.ShouldHideEmail("unknown", false) {
		t.Error("unknown user should use global default (false)")
	}

	store.SavePrefs("user1", &UserPrefs{HideEmail: true})
	if !store.ShouldHideEmail("user1", false) {
		t.Error("user with HideEmail=true should hide email")
	}

	store.SavePrefs("user2", &UserPrefs{HideEmail: false})
	if store.ShouldHideEmail("user2", true) {
		t.Error("user with HideEmail=false should not hide email")
	}
}

func TestCleanExpiredTokens(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.CreateToken("user1", "a@b.com", "A", -2*time.Hour)
	store.CreateToken("user2", "c@d.com", "C", time.Hour)

	count, err := store.CleanExpiredTokens()
	if err != nil {
		t.Fatalf("CleanExpiredTokens: %v", err)
	}
	if count != 1 {
		t.Errorf("deleted %d tokens, want 1", count)
	}

	got, _ := store.GetToken("user1")
	if got != nil {
		t.Error("expired token should be deleted")
	}
}

func TestClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mailblogger-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "mailblogger.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file should exist")
	}
}
