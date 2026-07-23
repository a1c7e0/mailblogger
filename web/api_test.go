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
		ShowAuthor: true,
		Width:      600,
	}
	srv, err := NewServer(ServerConfig{Store: store, Host: "localhost", Scheme: "http", EmailLocal: "blog", EmailDomain: "localhost", Site: site})
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
		ShowAuthor: true,
		Width:      600,
	}
	srv, err := NewServer(ServerConfig{Store: store, Host: "localhost", Scheme: "http", EmailLocal: "blog", EmailDomain: "localhost", Site: site})
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

// --- Full pipeline integration tests via /api/raw-email ---

func TestRawEmailFullPipeline(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "test-secret")
	handler := srv.Handler()

	// Step 1: Create article via raw email
	rawArticle := "From: Alice <alice@example.com>\r\n" +
		"To: blog@localhost\r\n" +
		"Subject: Integration Test Article\r\n" +
		"Message-Id: <article-integration@test.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"\r\n" +
		"This is the **article body**.\r\n"

	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawArticle))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("create article: status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var artResp APIResponse
	json.NewDecoder(w.Body).Decode(&artResp)
	if !artResp.OK || artResp.Type != "email" {
		t.Fatalf("create article: unexpected response: %+v", artResp)
	}
	articleID := artResp.ID

	// Step 2: Verify article appears in list
	req = httptest.NewRequest("GET", "/api/articles", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("list articles: status = %d", w.Code)
	}
	var listResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&listResp)
	articles, ok := listResp["articles"].([]interface{})
	if !ok || len(articles) != 1 {
		t.Fatalf("list articles: got %v, want 1", listResp["articles"])
	}
	art := articles[0].(map[string]interface{})
	if art["uniqueid"] != articleID {
		t.Errorf("article id = %v, want %s", art["uniqueid"], articleID)
	}
	if art["subject"] != "Integration Test Article" {
		t.Errorf("subject = %v, want Integration Test Article", art["subject"])
	}

	// Step 3: Verify article detail
	req = httptest.NewRequest("GET", "/api/article/"+articleID, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("get article: status = %d", w.Code)
	}
	var detail map[string]interface{}
	json.NewDecoder(w.Body).Decode(&detail)
	if detail["body"] != "This is the **article body**." {
		t.Errorf("body = %q", detail["body"])
	}

	// Step 4: Create comment via raw email
	rawComment := "From: Bob <bob@example.com>\r\n" +
		"To: blog+" + articleID + "@localhost\r\n" +
		"Subject: Re: Integration Test Article\r\n" +
		"Message-Id: <comment-integration@test.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:05:05 -0700\r\n" +
		"\r\n" +
		"Great article!\r\n"

	req = httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawComment))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("create comment: status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var cmtResp APIResponse
	json.NewDecoder(w.Body).Decode(&cmtResp)
	if !cmtResp.OK || cmtResp.Type != "email" {
		t.Fatalf("create comment: unexpected response: %+v", cmtResp)
	}

	// Step 5: Verify comment appears
	req = httptest.NewRequest("GET", "/api/article/"+articleID+"/comments", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("get comments: status = %d", w.Code)
	}
	var comments []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&comments)
	if len(comments) != 1 {
		t.Fatalf("get comments: got %d, want 1", len(comments))
	}
	if comments[0]["body"] != "Great article!" {
		t.Errorf("comment body = %q", comments[0]["body"])
	}
	if comments[0]["author"] != "Bob" {
		t.Errorf("comment author = %v, want Bob", comments[0]["author"])
	}
}

func TestRawEmailEditArticle(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "test-secret")
	handler := srv.Handler()

	// Create article
	rawArticle := "From: Alice <alice@example.com>\r\n" +
		"To: blog@localhost\r\n" +
		"Subject: Editable Article\r\n" +
		"Message-Id: <edit-test@test.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"\r\n" +
		"Original body.\r\n"

	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawArticle))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var artResp APIResponse
	json.NewDecoder(w.Body).Decode(&artResp)
	articleID := artResp.ID

	// Edit via raw email
	rawEdit := "From: Alice <alice@example.com>\r\n" +
		"To: blog+" + articleID + "@localhost\r\n" +
		"Subject: edit\r\n" +
		"Message-Id: <edit-cmd@test.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:05:05 -0700\r\n" +
		"\r\n" +
		"Updated body.\r\n"

	req = httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawEdit))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("edit article: status = %d, body: %s", w.Code, w.Body.String())
	}

	// Verify body updated
	req = httptest.NewRequest("GET", "/api/article/"+articleID, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var detail map[string]interface{}
	json.NewDecoder(w.Body).Decode(&detail)
	if detail["body"] != "Updated body." {
		t.Errorf("body after edit = %q, want %q", detail["body"], "Updated body.\r\n")
	}
}

func TestRawEmailDeleteArticle(t *testing.T) {
	srv := setupTestServerWithWebhook(t, "test-secret")
	handler := srv.Handler()

	// Create article
	rawArticle := "From: Alice <alice@example.com>\r\n" +
		"To: blog@localhost\r\n" +
		"Subject: Deletable Article\r\n" +
		"Message-Id: <delete-test@test.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"\r\n" +
		"Will be deleted.\r\n"

	req := httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawArticle))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var artResp APIResponse
	json.NewDecoder(w.Body).Decode(&artResp)
	articleID := artResp.ID

	// Delete via raw email
	rawDelete := "From: Alice <alice@example.com>\r\n" +
		"To: blog+" + articleID + "@localhost\r\n" +
		"Subject: delete\r\n" +
		"Message-Id: <delete-cmd@test.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:05:05 -0700\r\n" +
		"\r\n" +
		"\r\n"

	req = httptest.NewRequest("POST", "/api/raw-email", strings.NewReader(rawDelete))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Webhook-Secret", "test-secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("delete article: status = %d, body: %s", w.Code, w.Body.String())
	}

	// Verify article no longer accessible
	req = httptest.NewRequest("GET", "/api/article/"+articleID, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("get deleted article: status = %d, want 404", w.Code)
	}
}
