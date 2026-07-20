package web

import (
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	"time"

	"mailblogger/blog"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		extension.DefinitionList,
	),
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"]+|www\.[^\s<>"]+`)

func renderPlaintext(input string) template.HTML {
	escaped := template.HTMLEscapeString(input)
	escaped = urlRe.ReplaceAllStringFunc(escaped, func(rawURL string) string {
		href := rawURL
		if !strings.HasPrefix(href, "http") {
			href = "https://" + href
		}
		safeHref := template.HTMLEscapeString(href)
		safeText := template.HTMLEscapeString(rawURL)
		return fmt.Sprintf(`<a href="%s">%s</a>`, safeHref, safeText)
	})
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return template.HTML(escaped)
}

func renderMarkdown(input string) template.HTML {
	input = ensureImageBreaks(input)
	var buf strings.Builder
	if err := md.Convert([]byte(input), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(input))
	}
	return template.HTML(wrapImages(buf.String()))
}

var imgTagReFull = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"[^>]*alt="([^"]*)"[^>]*>`)
var imgTagReAltFirst = regexp.MustCompile(`<img\s+[^>]*alt="([^"]*)"[^>]*src="([^"]+)"[^>]*>`)
var imgTagReNoAlt = regexp.MustCompile(`<img\s+src="([^"]+)"[^>]*>`)

func wrapImages(html string) string {
	html = imgTagReAltFirst.ReplaceAllStringFunc(html, func(match string) string {
		m := imgTagReAltFirst.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		alt, src := m[1], m[2]
		fig := fmt.Sprintf(`<figure><a href="%s" target="_blank" rel="noopener">%s</a>`, src, match)
		if alt != "" {
			fig += fmt.Sprintf(`<figcaption>%s</figcaption>`, alt)
		}
		fig += `</figure>`
		return fig
	})
	html = imgTagReFull.ReplaceAllStringFunc(html, func(match string) string {
		if strings.Contains(match, "<figure") {
			return match
		}
		m := imgTagReFull.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		src, alt := m[1], m[2]
		fig := fmt.Sprintf(`<figure><a href="%s" target="_blank" rel="noopener">%s</a>`, src, match)
		if alt != "" {
			fig += fmt.Sprintf(`<figcaption>%s</figcaption>`, alt)
		}
		fig += `</figure>`
		return fig
	})
	html = imgTagReNoAlt.ReplaceAllStringFunc(html, func(match string) string {
		if strings.Contains(match, "<figure") || strings.Contains(match, "alt=") {
			return match
		}
		m := imgTagReNoAlt.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		src := m[1]
		return fmt.Sprintf(`<figure><a href="%s" target="_blank" rel="noopener">%s</a></figure>`, src, match)
	})
	return html
}

func ensureImageBreaks(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isImage := strings.HasPrefix(trimmed, "![") && strings.Contains(trimmed, "](") && strings.HasSuffix(trimmed, ")")
		if isImage {
			if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
				result = append(result, "")
			}
			result = append(result, line)
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				result = append(result, "")
			}
		} else {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func findImageRefs(body string) map[string]bool {
	refs := make(map[string]bool)
	for i := 0; i < len(body); {
		start := strings.Index(body[i:], "![")
		if start < 0 {
			break
		}
		start += i
		paren := strings.Index(body[start:], "](")
		if paren < 0 {
			i = start + 2
			continue
		}
		paren += start + 2
		end := strings.Index(body[paren:], ")")
		if end < 0 {
			break
		}
		name := body[paren : paren+end]
		refs[name] = true
		i = paren + end + 1
	}
	return refs
}

func makeMailto(uniqueID, subject, emailLocal, emailDomain, body, parentAuthor, parentDate, parentUID, parentHash string, isComment bool) string {
	addr := fmt.Sprintf("%s+%s@%s", emailLocal, uniqueID, emailDomain)
	subjPrefix := "Re: " + subject
	if isComment {
		subjPrefix += " - Comment"
	}
	subjEnc := strings.ReplaceAll(url.QueryEscape(subjPrefix+" #"+parentUID), "+", "%20")
	link := fmt.Sprintf("mailto:%s?subject=%s", addr, subjEnc)
	if parentAuthor != "" {
		rawBody := strings.ReplaceAll(body, "\r\n", "\n")
		quoted := "> " + strings.ReplaceAll(rawBody, "\n", "\n> ")
		if len(quoted) > 1200 {
			quoted = quoted[:1200] + "\n> ..."
		}
		var ref strings.Builder
		ref.WriteString("\n\n\n> ---\n> Write your reply above this line. Only text above will be saved.\n>\n")
		ref.WriteString(fmt.Sprintf("> On %s, %s (%s) wrote:\n>\n", parentDate, parentAuthor, parentHash))
		ref.WriteString(quoted)
		ref.WriteString("\n")
		enc := strings.ReplaceAll(url.QueryEscape(ref.String()), "+", "%20")
		link += "&body=" + enc
	}
	return link
}

func toTime(v interface{}) time.Time {
	switch val := v.(type) {
	case *blog.Article:
		return val.Date
	case blog.Article:
		return val.Date
	case *blog.Comment:
		return val.Date
	case blog.CommentEdit:
		return val.Date
	case *blog.CommentEdit:
		return val.Date
	case time.Time:
		return val
	}
	return time.Time{}
}

func fmtDate(d interface{}) string {
	t := toTime(d)
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func fmtDateTitle(d interface{}) string {
	t := toTime(d)
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (Unix: %d)", t.Format("2006-01-02 15:04:05 -0700"), t.Unix())
}

func datetimeISO(d interface{}) string {
	t := toTime(d)
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var mdStripRe = regexp.MustCompile(`[#*\[\]!()>|_~` + "`" + `]`)

func excerpt(input string, maxLen int) string {
	s := mdStripRe.ReplaceAllString(input, "")
	s = strings.Join(strings.Fields(s), " ")
	return truncate(s, maxLen)
}

func authorTooltipFn(store *blog.Store, globalHideEmail bool, authorHash, authorEmail string) string {
	hide := store.ShouldHideEmail(authorHash, globalHideEmail)
	if hide {
		return "hash: " + authorHash
	}
	if authorEmail != "" {
		return authorEmail + "\nhash: " + authorHash
	}
	return "hash: " + authorHash
}

func parseAcceptLanguage(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if len(lang) >= 2 {
			code := strings.ToLower(lang[:2])
			if code >= "aa" && code <= "zz" {
				return code
			}
		}
	}
	return ""
}
