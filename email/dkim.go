package email

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"regexp"
	"strings"
)

var dkimHeaderRe = regexp.MustCompile(`(?i)^DKIM-Signature\s*:\s*(.*)`)

func VerifyDKIM(rawBody []byte) (bool, string, error) {
	text := string(rawBody)
	m := dkimHeaderRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return false, "", nil
	}

	params := parseDKIMParams(m[1])
	domain := params["d"]
	selector := params["s"]
	if domain == "" || selector == "" {
		return false, "", nil
	}

	pubKey, err := lookupDKIMKey(selector, domain)
	if err != nil {
		return false, domain, fmt.Errorf("dns lookup: %w", err)
	}

	sig := params["b"]
	if sig == "" {
		return false, domain, nil
	}
	algo := params["a"]
	if algo == "" {
		algo = "rsa-sha256"
	}
	_ = algo

	headers := strings.Split(params["h"], ":")

	var headerData strings.Builder
	lines := strings.Split(text, "\r\n")
	for _, line := range lines {
		for _, h := range headers {
			if strings.HasPrefix(strings.ToLower(line), strings.ToLower(strings.TrimSpace(h))+":") {
				headerData.WriteString(line)
				headerData.WriteString("\r\n")
				break
			}
		}
	}

	sigBytes, _ := base64.StdEncoding.DecodeString(sig)
	hash := sha256.Sum256([]byte(headerData.String()))
	err = rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sigBytes)
	if err != nil {
		return false, domain, fmt.Errorf("signature invalid: %w", err)
	}
	return true, domain, nil
}

func parseDKIMParams(s string) map[string]string {
	params := map[string]string{}
	parts := strings.Split(s, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(strings.ToLower(kv[0]))
			v := strings.TrimSpace(kv[1])
			v = strings.Trim(v, "\"")
			v = strings.TrimSpace(v)
			params[k] = v
		}
	}
	return params
}

func lookupDKIMKey(selector, domain string) (*rsa.PublicKey, error) {
	txts, err := net.LookupTXT(selector + "._domainkey." + domain)
	if err != nil {
		return nil, fmt.Errorf("dns lookup: %w", err)
	}
	var record string
	for _, t := range txts {
		if strings.Contains(t, "k=rsa") || strings.Contains(t, "p=") {
			record = t
			break
		}
	}
	if record == "" {
		return nil, fmt.Errorf("no DKIM record found")
	}

	params := parseDKIMParams(strings.ReplaceAll(record, "; ", ";"))
	p := params["p"]
	if p == "" {
		return nil, fmt.Errorf("no public key in record")
	}

	block, _ := pem.Decode([]byte("-----BEGIN PUBLIC KEY-----\n" + p + "\n-----END PUBLIC KEY-----"))
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	return rsaKey, nil
}
