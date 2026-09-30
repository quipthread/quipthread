package mailer

import (
	"strings"

	"golang.org/x/net/html"
)

// plainTextFromHTML derives the text/plain alternative that travels beside the
// rendered HTML. It tokenizes the markup instead of stripping tags with a
// regex, so <head>, <style>, and <script> sources never leak CSS or script text
// into the plain part, and a link label keeps its destination.
func plainTextFromHTML(htmlBody string) string {
	var out strings.Builder
	skipped := 0
	var anchors []plainTextAnchor
	tokenizer := html.NewTokenizer(strings.NewReader(htmlBody))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(out.String()), " ")
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if skipPlainTextElement(tag) {
				skipped++
				continue
			}
			if skipped > 0 {
				continue
			}
			if tag == "a" {
				anchors = append(anchors, plainTextAnchor{labelStart: out.Len(), href: anchorHref(tokenizer, hasAttr)})
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if skipPlainTextElement(tag) {
				if skipped > 0 {
					skipped--
				}
				continue
			}
			if skipped > 0 {
				continue
			}
			if tag == "a" && len(anchors) > 0 {
				anchor := anchors[len(anchors)-1]
				anchors = anchors[:len(anchors)-1]
				label := strings.TrimSpace(out.String()[anchor.labelStart:])
				if anchor.href != "" && anchor.href != label {
					out.WriteByte(' ')
					out.WriteString("(" + anchor.href + ")")
					out.WriteByte(' ')
				}
			}
		case html.TextToken:
			if skipped > 0 {
				continue
			}
			out.Write(tokenizer.Text())
			out.WriteByte(' ')
		}
	}
}

// plainTextAnchor remembers where a link label started so the href can be
// appended after the label when the link closes.
type plainTextAnchor struct {
	labelStart int
	href       string
}

// skipPlainTextElement reports whether an element carries source text that is
// meaningless in a plain-text alternative.
func skipPlainTextElement(tag string) bool {
	switch tag {
	case "head", "style", "script", "title":
		return true
	default:
		return false
	}
}

func anchorHref(tokenizer *html.Tokenizer, hasAttr bool) string {
	if !hasAttr {
		return ""
	}
	href := ""
	for {
		key, value, more := tokenizer.TagAttr()
		if strings.EqualFold(string(key), "href") {
			href = strings.TrimSpace(string(value))
		}
		if !more {
			return href
		}
	}
}
