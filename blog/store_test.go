package blog

import (
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMeta map[string]string
		wantBody string
	}{
		{
			name: "normal frontmatter",
			input: "---\nsubject: Hello\nauthor: Test\n---\n\nBody content here",
			wantMeta: map[string]string{
				"subject": "Hello",
				"author":  "Test",
			},
			wantBody: "Body content here",
		},
		{
			name:     "no frontmatter",
			input:    "Just plain text",
			wantMeta: map[string]string{},
			wantBody: "Just plain text",
		},
		{
			name: "empty body",
			input: "---\nsubject: Hello\n---",
			wantMeta: map[string]string{
				"subject": "Hello",
			},
			wantBody: "",
		},
		{
			name: "crlf line endings",
			input: "---\r\nsubject: Hello\r\n---\r\n\r\nBody",
			wantMeta: map[string]string{
				"subject": "Hello",
			},
			wantBody: "Body",
		},
		{
			name: "has suffix --- without newline",
			input: "---\nsubject: Hello\n---",
			wantMeta: map[string]string{
				"subject": "Hello",
			},
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body, err := ParseFrontmatter([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.wantMeta {
				if meta[k] != v {
					t.Errorf("meta[%q] = %q, want %q", k, meta[k], v)
				}
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestSplitCommentBlocks(t *testing.T) {
	input := `---
uniqueid: abc123
author: Alice
date: 2026-01-01T00:00:00Z
---

First comment body

---
uniqueid: def456
author: Bob
date: 2026-01-02T00:00:00Z
---

Second comment body`

	blocks := splitCommentBlocks([]byte(input))
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	meta1, body1, err := ParseFrontmatter(blocks[0])
	if err != nil {
		t.Fatalf("block 1 parse error: %v", err)
	}
	if meta1["uniqueid"] != "abc123" {
		t.Errorf("block 1 uniqueid = %q, want abc123", meta1["uniqueid"])
	}
	if body1 != "First comment body" {
		t.Errorf("block 1 body = %q, want 'First comment body'", body1)
	}

	meta2, body2, err := ParseFrontmatter(blocks[1])
	if err != nil {
		t.Fatalf("block 2 parse error: %v", err)
	}
	if meta2["uniqueid"] != "def456" {
		t.Errorf("block 2 uniqueid = %q, want def456", meta2["uniqueid"])
	}
	if body2 != "Second comment body" {
		t.Errorf("block 2 body = %q, want 'Second comment body'", body2)
	}
}

func TestSplitCommentBlocksEmpty(t *testing.T) {
	blocks := splitCommentBlocks([]byte(""))
	if len(blocks) != 0 {
		t.Errorf("empty input should return 0 blocks, got %d", len(blocks))
	}
}

func TestSplitCommentBlocksSingle(t *testing.T) {
	input := `---
uniqueid: abc123
author: Alice
---

Single comment`

	blocks := splitCommentBlocks([]byte(input))
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	meta, body, _ := ParseFrontmatter(blocks[0])
	if meta["uniqueid"] != "abc123" {
		t.Errorf("uniqueid = %q, want abc123", meta["uniqueid"])
	}
	if body != "Single comment" {
		t.Errorf("body = %q, want 'Single comment'", body)
	}
}

func TestSplitCommentBlocksNoBody(t *testing.T) {
	input := `---
uniqueid: abc123
author: Alice
---`

	blocks := splitCommentBlocks([]byte(input))
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	meta, body, _ := ParseFrontmatter(blocks[0])
	if meta["uniqueid"] != "abc123" {
		t.Errorf("uniqueid = %q, want abc123", meta["uniqueid"])
	}
	if body != "" {
		t.Errorf("body should be empty, got %q", body)
	}
}

func TestSplitCommentBlocksNoFrontmatter(t *testing.T) {
	input := `Just some text without frontmatter`

	blocks := splitCommentBlocks([]byte(input))
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	meta, body, _ := ParseFrontmatter(blocks[0])
	if len(meta) != 0 {
		t.Errorf("meta should be empty, got %v", meta)
	}
	if body != input {
		t.Errorf("body = %q, want %q", body, input)
	}
}

func TestGenUniqueID(t *testing.T) {
	id1 := GenUniqueID("test-input")
	id2 := GenUniqueID("test-input")
	id3 := GenUniqueID("different-input")

	if id1 != id2 {
		t.Errorf("same input should produce same ID: %q != %q", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("different input should produce different ID: %q == %q", id1, id3)
	}
	if len(id1) != 8 {
		t.Errorf("ID length = %d, want 8", len(id1))
	}
}

func TestGenDisplayName(t *testing.T) {
	if got := GenDisplayName("Alice", "alice@example.com"); got != "Alice" {
		t.Errorf("with name, got %q, want Alice", got)
	}
	if got := GenDisplayName("", "alice@example.com"); got == "" {
		t.Error("without name, should return hash")
	}
}

func TestArticleDirName(t *testing.T) {
	a := &Article{
		UniqueID: "abc123",
		Slug:     "my-post",
	}
	name := articleDirName(a)
	if name != "00010101_abc123_my-post" {
		t.Errorf("articleDirName = %q", name)
	}

	a2 := &Article{UniqueID: "abc123"}
	name2 := articleDirName(a2)
	if name2 != "00010101_abc123" {
		t.Errorf("articleDirName without slug = %q", name2)
	}
}

func TestParseDirHash(t *testing.T) {
	if h := parseDirHash("20260713_abc123_my-slug"); h != "abc123" {
		t.Errorf("parseDirHash = %q, want abc123", h)
	}
	if h := parseDirHash("20260713_abc123"); h != "abc123" {
		t.Errorf("parseDirHash (no slug) = %q, want abc123", h)
	}
	if h := parseDirHash("invalid"); h != "" {
		t.Errorf("parseDirHash (invalid) = %q, want empty", h)
	}
}

func TestParseDirSlug(t *testing.T) {
	if s := ParseDirSlug("20260713_abc123_my-slug"); s != "my-slug" {
		t.Errorf("ParseDirSlug = %q, want my-slug", s)
	}
	if s := ParseDirSlug("20260713_abc123"); s != "" {
		t.Errorf("ParseDirSlug (no slug) = %q, want empty", s)
	}
}
