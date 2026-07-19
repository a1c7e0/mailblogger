package web

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"mailblogger/blog"
	"mailblogger/config"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := blog.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site := config.SiteConfig{
		Title:      "Test Blog",
		ShowAuthor: true,
		Width:      600,
	}
	srv, err := NewServer(store, "localhost", "http", "blog", "localhost", true, site, "0.0.0.0", 8080)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetConfigGetter(func() *config.Config {
		return &config.Config{}
	})
	return srv
}

func setupTestServerWithWebhook(t *testing.T, secret string) *Server {
	t.Helper()
	store, err := blog.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site := config.SiteConfig{
		Title:      "Test Blog",
		ShowAuthor: true,
		Width:      600,
	}
	srv, err := NewServer(store, "localhost", "http", "blog", "localhost", true, site, "0.0.0.0", 8080)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetConfigGetter(func() *config.Config {
		return &config.Config{
			Webhook: config.WebhookConfig{Secret: secret},
		}
	})
	return srv
}

func TestAPIStatus(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handleAPIStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

func TestAPIArticle(t *testing.T) {
	srv := setupTestServer(t)
	body := APIArticleRequest{
		From:    "Alice <alice@example.com>",
		Subject: "Test Article",
		Body:    "Hello world",
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/article", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAPIArticle(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.OK {
		t.Errorf("ok = false, want true")
	}
	if resp.ID == "" {
		t.Errorf("id is empty")
	}
	if resp.Type != "article" {
		t.Errorf("type = %q, want article", resp.Type)
	}
}

func TestAPIComment(t *testing.T) {
	srv := setupTestServer(t)

	// First create an article
	artBody := APIArticleRequest{
		From:    "Alice <alice@example.com>",
		Subject: "Parent Article",
		Body:    "Article body",
	}
	data, _ := json.Marshal(artBody)
	req := httptest.NewRequest("POST", "/api/article", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAPIArticle(w, req)

	var artResp APIResponse
	json.NewDecoder(w.Body).Decode(&artResp)

	// Now create a comment
	cmtBody := APICommentRequest{
		From:    "Bob <bob@example.com>",
		Subject: "Re: Parent Article",
		Body:    "Nice article!",
		ReplyTo: artResp.ID,
	}
	data, _ = json.Marshal(cmtBody)
	req = httptest.NewRequest("POST", "/api/comment", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.handleAPIComment(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var cmtResp APIResponse
	json.NewDecoder(w.Body).Decode(&cmtResp)
	if !cmtResp.OK {
		t.Errorf("ok = false, want true")
	}
	if cmtResp.Type != "comment" {
		t.Errorf("type = %q, want comment", cmtResp.Type)
	}
}

func TestAPIArticleInvalidJSON(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest("POST", "/api/article", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAPIArticle(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAPIArticleMethodNotAllowed(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/article", nil)
	w := httptest.NewRecorder()
	srv.handleAPIArticle(w, req)

	if w.Code != 405 {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestAPIArticleWithImages(t *testing.T) {
	srv := setupTestServer(t)
	body := APIArticleRequest{
		From:    "Alice <alice@example.com>",
		Subject: "Article with image",
		Body:    "Check this out",
		Images: []APIImage{
			{
				Data:        "iVBORw0KGgoAAAANSUhEUg==",
				ContentType: "image/png",
				Filename:    "test.png",
			},
		},
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/article", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAPIArticle(w, req)

	// Should succeed even with potentially invalid base64
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

func TestBuildRawMessage(t *testing.T) {
	srv := setupTestServer(t)
	raw, err := srv.buildRawMessage(
		"Alice <alice@example.com>",
		"blog@localhost",
		"Test Subject",
		"Body text",
		"<p>Body HTML</p>",
		nil,
		"2026-01-15T10:30:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	if raw.From.Address != "alice@example.com" {
		t.Errorf("from = %q, want alice@example.com", raw.From.Address)
	}
	if raw.Subject != "Test Subject" {
		t.Errorf("subject = %q, want Test Subject", raw.Subject)
	}
	if raw.Body != "Body text" {
		t.Errorf("body = %q, want Body text", raw.Body)
	}
	if raw.HTMLBody != "<p>Body HTML</p>" {
		t.Errorf("htmlBody = %q", raw.HTMLBody)
	}
	if len(raw.To) != 1 || raw.To[0].Address != "blog@localhost" {
		t.Errorf("to = %v", raw.To)
	}
}

func TestBuildRawMessageInvalidFrom(t *testing.T) {
	srv := setupTestServer(t)
	_, err := srv.buildRawMessage("invalid", "", "Subj", "body", "", nil, "")
	if err == nil {
		t.Error("expected error for invalid from address")
	}
}

func TestAPIRawEmail(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "test-secret")

	rawEmail := "From: alice@example.com\r\n" +
		"To: blog@localhost\r\n" +
		"Subject: Hello via Worker\r\n" +
		"Message-Id: <worker-test@example.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"\r\n" +
		"Body from worker.\r\n"

	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawEmail))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()
	srv.handleAPIRawEmail(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var resp APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.OK {
		t.Errorf("ok = false, want true")
	}
	if resp.ID == "" {
		t.Errorf("id is empty")
	}
	if resp.Type != "email" {
		t.Errorf("type = %q, want email", resp.Type)
	}
}

func TestAPIRawEmailInvalidSecret(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "correct-secret")

	rawEmail := "From: alice@example.com\r\nTo: blog@localhost\r\nSubject: Test\r\n\r\nBody\r\n"
	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawEmail))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "wrong-secret")
	w := httptest.NewRecorder()
	srv.handleAPIRawEmail(w, req)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAPIRawEmailNoSecret(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "test-secret")

	rawEmail := "From: alice@example.com\r\nTo: blog@localhost\r\nSubject: Test\r\n\r\nBody\r\n"
	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawEmail))
	req.Header.Set("Content-Type", "message/rfc822")
	w := httptest.NewRecorder()
	srv.handleAPIRawEmail(w, req)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAPIRawEmailWebhookNotConfigured(t *testing.T) {
	srv := setupTestServer(t) // no webhook secret configured

	rawEmail := "From: alice@example.com\r\nTo: blog@localhost\r\nSubject: Test\r\n\r\nBody\r\n"
	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawEmail))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "anything")
	w := httptest.NewRecorder()
	srv.handleAPIRawEmail(w, req)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAPIRawEmailMethodNotAllowed(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "test-secret")

	req := httptest.NewRequest("GET", "/api/raw-email", nil)
	w := httptest.NewRecorder()
	srv.handleAPIRawEmail(w, req)

	if w.Code != 405 {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
