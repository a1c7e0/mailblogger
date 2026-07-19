package email

import (
	"testing"
)

func TestParseDKIMParams(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   map[string]string
	}{
		{
			name:  "standard params",
			input: "v=1; a=rsa-sha256; d=example.com; s=selector; h=from:to:subject",
			want: map[string]string{
				"v": "1",
				"a": "rsa-sha256",
				"d": "example.com",
				"s": "selector",
				"h": "from:to:subject",
			},
		},
		{
			name:  "quoted values",
			input: `v=1; a=rsa-sha256; b="base64data"; bh="hash"`,
			want: map[string]string{
				"v":  "1",
				"a":  "rsa-sha256",
				"b":  "base64data",
				"bh": "hash",
			},
		},
		{
			name:  "with whitespace",
			input: "v = 1 ; a = rsa-sha256 ; d = example.com",
			want: map[string]string{
				"v": "1",
				"a": "rsa-sha256",
				"d": "example.com",
			},
		},
		{
			name:  "empty string",
			input: "",
			want:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDKIMParams(tt.input)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseDKIMParams(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestVerifyDKIMNoSignature(t *testing.T) {
	input := []byte("From: test@example.com\r\nTo: user@example.com\r\nSubject: Test\r\n\r\nBody")
	ok, domain, err := VerifyDKIM(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("should return false when no DKIM signature")
	}
	if domain != "" {
		t.Errorf("domain should be empty, got %q", domain)
	}
}

func TestVerifyDKIMMalformedSignature(t *testing.T) {
	input := []byte("DKIM-Signature: v=1; a=rsa-sha256; d=; s=;\r\n\r\nBody")
	ok, domain, err := VerifyDKIM(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("should return false for malformed signature")
	}
	if domain != "" {
		t.Errorf("domain should be empty, got %q", domain)
	}
}

func TestVerifyDKIMEmptyBase64(t *testing.T) {
	input := []byte("DKIM-Signature: v=1; a=rsa-sha256; d=example.com; s=selector; h=from; b=\r\n\r\nBody")
	ok, domain, err := VerifyDKIM(input)
	if err == nil {
		t.Error("expected error for DNS lookup failure")
	}
	if ok {
		t.Error("should return false for empty base64 signature")
	}
	if domain != "example.com" {
		t.Errorf("domain = %q, want example.com", domain)
	}
}
