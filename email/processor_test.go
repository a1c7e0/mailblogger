package email

import (
	"testing"
)

func TestCleanSubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "Hello"},
		{"Re: Hello", "Hello"},
		{"RE: Hello", "Hello"},
		{"re: Hello", "Hello"},
		{"Fwd: Hello", "Hello"},
		{"FWD: Hello", "Hello"},
		{"fwd: Hello", "Hello"},
		{"Re: Re: Hello", "Hello"},
		{"Re: Fwd: Hello", "Hello"},
		{"Re: RE: re: Fwd: FWD: fwd: Deep", "Deep"},
		{"Re: Spaced", "Spaced"},
		{"Re:NoSpace", "NoSpace"},
		{"RE:NoSpace", "NoSpace"},
		{"Fwd:NoSpace", "NoSpace"},
		{"Re: Re:NoSpace", "NoSpace"},
		{"No prefix", "No prefix"},
		{"Re:", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanSubject(tt.input)
			if got != tt.want {
				t.Errorf("cleanSubject(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		addr    string
		want    bool
	}{
		{"*", "any@any.com", true},
		{"alice@example.com", "alice@example.com", true},
		{"alice@example.com", "bob@example.com", false},
		{"*@example.com", "alice@example.com", true},
		{"*@example.com", "alice@other.com", false},
		{"*@example.com", "alice", false},
	}
	for _, tt := range tests {
		got := matchPattern(tt.pattern, tt.addr)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.addr, got, tt.want)
		}
	}
}

func TestParseNotifyTag(t *testing.T) {
	tests := []struct {
		subject    string
		wantNotify bool
		wantFound  bool
	}{
		{"Hello [NOTIFY]", true, true},
		{"Hello [WATCH]", true, true},
		{"Hello [MUTE]", false, true},
		{"Hello [NOWATCH]", false, true},
		{"Hello [notify]", true, true},
		{"Hello World", false, false},
		{"[NOTIFY] Important", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			result := parseNotifyTag(tt.subject)
			if result.notify != tt.wantNotify || result.found != tt.wantFound {
				t.Errorf("parseNotifyTag(%q) = {notify:%v, found:%v}, want {notify:%v, found:%v}",
					tt.subject, result.notify, result.found, tt.wantNotify, tt.wantFound)
			}
		})
	}
}

func TestIsWhitelisted(t *testing.T) {
	p := &Processor{Whitelist: nil}
	if !p.isWhitelisted("anyone@test.com") {
		t.Error("empty whitelist should allow everyone")
	}

	p.Whitelist = []string{"*@example.com", "admin@test.com"}
	if !p.isWhitelisted("user@example.com") {
		t.Error("should match domain pattern")
	}
	if !p.isWhitelisted("admin@test.com") {
		t.Error("should match exact address")
	}
	if p.isWhitelisted("user@other.com") {
		t.Error("should not match unknown domain")
	}

	p.Whitelist = []string{"*"}
	if !p.isWhitelisted("anyone@anywhere.com") {
		t.Error("* should match everyone")
	}
}

func TestStripEmailQuotes(t *testing.T) {
	input := "Hello world\n\nOn Mon, someone wrote:\n> quoted text"
	got := stripEmailQuotes(input)
	if got != "Hello world" {
		t.Errorf("stripEmailQuotes = %q, want 'Hello world'", got)
	}

	input2 := "No quotes here"
	got2 := stripEmailQuotes(input2)
	if got2 != input2 {
		t.Errorf("stripEmailQuotes without quotes = %q, want %q", got2, input2)
	}
}

func TestStripEmailQuotesPreservesBlockquotes(t *testing.T) {
	// Markdown blockquotes should NOT be stripped
	input := "### Blockquotes\n\nMarkdown uses email-style `>` characters.\n\n> This is a blockquote.\n> Second line.\n\nMore text after."
	got := stripEmailQuotes(input)
	if got != input {
		t.Errorf("stripEmailQuotes should preserve markdown blockquotes\ngot:  %q\nwant: %q", got, input)
	}
}

func TestStripEmailQuotesStripsOnWroteChain(t *testing.T) {
	// "On ... wrote:" with > quoted lines should be stripped
	input := "My message\n\nOn Mon, Jan 1, Alice wrote:\n> their reply\n> more reply"
	got := stripEmailQuotes(input)
	if got != "My message" {
		t.Errorf("stripEmailQuotes = %q, want 'My message'", got)
	}
}

func TestStripEmailQuotesStripsDashSeparator(t *testing.T) {
	// Standalone "---" separator should be stripped
	input := "My message\n\n---\nSignature here"
	got := stripEmailQuotes(input)
	if got != "My message" {
		t.Errorf("stripEmailQuotes = %q, want 'My message'", got)
	}
}

func TestCleanBodyNBSP(t *testing.T) {
	// Non-breaking spaces (U+00A0, UTF-8: \xc2\xa0) from email clients
	// should be replaced with regular spaces
	input := "Hello\xc2\xa0\xc2\xa0World\xc2\xa0test"
	got := cleanBody(input)
	want := "Hello  World test"
	if got != want {
		t.Errorf("cleanBody nbsp = %q, want %q", got, want)
	}
}

func TestCleanBodyPreservesLists(t *testing.T) {
	// Lists with regular spaces should be preserved
	input := "* item one\n* item two\n\n1. first\n2. second"
	got := cleanBody(input)
	if got != input {
		t.Errorf("cleanBody should preserve lists\ngot:  %q\nwant: %q", got, input)
	}
}

func TestDecodeBodyBase64(t *testing.T) {
	input := "SGVsbG8gV29ybGQ="
	got := decodeBody([]byte(input), "base64")
	if got != "Hello World" {
		t.Errorf("decodeBody base64 = %q, want 'Hello World'", got)
	}
}

func TestDecodeBodyQuotedPrintable(t *testing.T) {
	input := "Hello=20World"
	got := decodeBody([]byte(input), "quoted-printable")
	if got != "Hello World" {
		t.Errorf("decodeBody qp = %q, want 'Hello World'", got)
	}
}

func TestHtmlToMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "br tags",
			input: "Line 1<br>Line 2<br/>Line 3<br />Line 4",
			want:  "Line 1\nLine 2\nLine 3\nLine 4",
		},
		{
			name:  "p tags",
			input: "<p>Para 1</p><p>Para 2</p>",
			want:  "Para 1\n\nPara 2",
		},
		{
			name:  "img to markdown",
			input: `<img src="pic.jpg" alt="Photo">`,
			want:  "![Photo](pic.jpg)",
		},
		{
			name:  "html entities",
			input: "&lt;div&gt; &amp; &quot;test&quot;",
			want:  "<div> & \"test\"",
		},
		{
			name:  "strip html tags",
			input: "<html><body><p>Hello</p></body></html>",
			want:  "Hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("htmlToMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseEmailDate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Mon, 02 Jan 2006 15:04:05 -0700", true},
		{"2006-01-02T15:04:05Z", true},
		{"not a date", false},
	}
	for _, tt := range tests {
		_, err := parseEmailDate(tt.input)
		got := err == nil
		if got != tt.want {
			t.Errorf("parseEmailDate(%q) success = %v, want %v (err=%v)", tt.input, got, tt.want, err)
		}
	}
}

func TestParseBodyConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCfg  map[string]string
		wantBody string
	}{
		{
			name:     "config with banner",
			input:    "---config\nbanner: 1\n---\n\nHello world",
			wantCfg:  map[string]string{"banner": "1"},
			wantBody: "Hello world",
		},
		{
			name:     "config with slug and banner",
			input:    "---config\nslug: my-post\nbanner: 2\n---\n\nBody here",
			wantCfg:  map[string]string{"slug": "my-post", "banner": "2"},
			wantBody: "Body here",
		},
		{
			name:     "no config - plain body",
			input:    "Just a regular article body",
			wantCfg:  nil,
			wantBody: "Just a regular article body",
		},
		{
			name:     "no config - starts with text not dash",
			input:    "Hello\n---\nworld",
			wantCfg:  nil,
			wantBody: "Hello\n---\nworld",
		},
		{
			name:     "no config - unknown key falls back",
			input:    "---\nNote: this is important\n---\n\nBody",
			wantCfg:  nil,
			wantBody: "---\nNote: this is important\n---\n\nBody",
		},
		{
			name:     "no config - key with space falls back",
			input:    "---\nnot valid: yes\n---\n\nBody",
			wantCfg:  nil,
			wantBody: "---\nnot valid: yes\n---\n\nBody",
		},
		{
			name:     "empty config block",
			input:    "---config\n---\n\nBody",
			wantCfg:  nil,
			wantBody: "---config\n---\n\nBody",
		},
		{
			name:     "config with empty lines",
			input:    "---config\nbanner: 1\n\nslug: test\n---\n\nBody",
			wantCfg:  map[string]string{"banner": "1", "slug": "test"},
			wantBody: "Body",
		},
		{
			name:     "config with value containing colon",
			input:    "---config\nslug: my-post\n---\n\nTime is 10:30",
			wantCfg:  map[string]string{"slug": "my-post"},
			wantBody: "Time is 10:30",
		},
		{
			name:     "markdown heading with dash below",
			input:    "# Title\n\nSome text\n---\n\nMore",
			wantCfg:  nil,
			wantBody: "# Title\n\nSome text\n---\n\nMore",
		},
		{
			name:     "5 dashes",
			input:    "-----config\nbanner: 1\n-----\n\nBody",
			wantCfg:  map[string]string{"banner": "1"},
			wantBody: "Body",
		},
		{
			name:     "notify and banner",
			input:    "---config\nnotify: true\nbanner: 3\n---\n\nBody",
			wantCfg:  map[string]string{"notify": "true", "banner": "3"},
			wantBody: "Body",
		},
		{
			name:     "2 dashes not enough",
			input:    "--\nbanner: 1\n--\n\nBody",
			wantCfg:  nil,
			wantBody: "--\nbanner: 1\n--\n\nBody",
		},
		{
			name:     "dashes with spaces",
			input:    "---config\nbanner: 1\n---\n\nBody",
			wantCfg:  map[string]string{"banner": "1"},
			wantBody: "Body",
		},
		{
			name:     "body starts with horizontal rule",
			input:    "---\n\nSome content.\n\n---\n\nMore.",
			wantCfg:  nil,
			wantBody: "---\n\nSome content.\n\n---\n\nMore.",
		},
		{
			name:     "mixed known and unknown keys",
			input:    "---config\nbanner: 1\nunknown: val\n---\n\nBody",
			wantCfg:  nil,
			wantBody: "---config\nbanner: 1\nunknown: val\n---\n\nBody",
		},
		{
			name:     "notify only",
			input:    "---config\nnotify: on\n---\n\nBody",
			wantCfg:  map[string]string{"notify": "on"},
			wantBody: "Body",
		},
		{
			name:     "unclosed block with known key",
			input:    "---config\nbanner: 1\n\nBody without closing",
			wantCfg:  nil,
			wantBody: "---config\nbanner: 1\n\nBody without closing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, body, err := parseBodyConfig(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantCfg == nil {
				if cfg != nil {
					t.Errorf("cfg = %v, want nil", cfg)
				}
			} else {
				if cfg == nil {
					t.Fatalf("cfg = nil, want %v", tt.wantCfg)
				}
				for k, v := range tt.wantCfg {
					if cfg[k] != v {
						t.Errorf("cfg[%q] = %q, want %q", k, cfg[k], v)
					}
				}
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestParseRawEmailPlainText(t *testing.T) {
	raw := []byte("From: alice@example.com\r\n" +
		"To: blog@example.com\r\n" +
		"Subject: Hello World\r\n" +
		"Message-Id: <test123@example.com>\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"\r\n" +
		"This is the body.\r\n")

	msg, err := ParseRawEmail(raw)
	if err != nil {
		t.Fatalf("ParseRawEmail: %v", err)
	}
	if msg.From == nil || msg.From.Address != "alice@example.com" {
		t.Errorf("from = %v, want alice@example.com", msg.From)
	}
	if len(msg.To) != 1 || msg.To[0].Address != "blog@example.com" {
		t.Errorf("to = %v, want blog@example.com", msg.To)
	}
	if msg.Subject != "Hello World" {
		t.Errorf("subject = %q, want Hello World", msg.Subject)
	}
	if msg.MessageID != "<test123@example.com>" {
		t.Errorf("messageID = %q, want <test123@example.com>", msg.MessageID)
	}
	if msg.Body != "This is the body." {
		t.Errorf("body = %q, want %q", msg.Body, "This is the body.")
	}
	if msg.RawBody == nil {
		t.Error("rawBody is nil")
	}
}

func TestParseRawEmailMultipart(t *testing.T) {
	boundary := "----=_Part_001"
	raw := []byte("From: bob@example.com\r\n" +
		"To: blog@example.com\r\n" +
		"Subject: Multipart Test\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n" +
		"\r\n" +
		"------=_Part_001\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Plain text body.\r\n" +
		"------=_Part_001\r\n" +
		"Content-Type: image/png\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"test.png\"\r\n" +
		"\r\n" +
		"iVBORw0KGgoAAAANSUhEUg==\r\n" +
		"------=_Part_001--\r\n")

	msg, err := ParseRawEmail(raw)
	if err != nil {
		t.Fatalf("ParseRawEmail: %v", err)
	}
	if msg.Body != "Plain text body." {
		t.Errorf("body = %q, want %q", msg.Body, "Plain text body.")
	}
	if len(msg.Images) != 1 {
		t.Fatalf("images = %d, want 1", len(msg.Images))
	}
	if msg.Images[0].OriginalName != "test.png" {
		t.Errorf("image name = %q, want test.png", msg.Images[0].OriginalName)
	}
	if msg.Images[0].ContentType != "image/png" {
		t.Errorf("image type = %q, want image/png", msg.Images[0].ContentType)
	}
}

func TestParseRawEmailInvalid(t *testing.T) {
	_, err := ParseRawEmail([]byte("not an email"))
	if err == nil {
		t.Error("expected error for invalid email")
	}
}
