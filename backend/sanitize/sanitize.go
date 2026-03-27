// Package sanitize provides HTML sanitization for user-generated content.
package sanitize

import "github.com/microcosm-cc/bluemonday"

// commentPolicy is the allowlist for comment HTML. It matches the safe subset
// produced by the Tiptap editor — all other tags and attributes are stripped.
// Constructed once at init time; bluemonday policies are safe for concurrent use
// after construction.
var commentPolicy = buildCommentPolicy()

func buildCommentPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// Inline formatting
	p.AllowElements("p", "br", "strong", "em", "s", "u", "code")

	// Block elements
	p.AllowElements("pre", "blockquote")
	p.AllowElements("ul", "ol", "li")
	p.AllowElements("h1", "h2", "h3")

	// Links — RequireParseableURLs MUST be set before AllowURLSchemes;
	// without it the scheme allowlist is never consulted and javascript: hrefs
	// would pass through unchanged.
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https")
	p.AllowAttrs("href").OnElements("a")

	// Security headers on all links: inject rel="nofollow noreferrer" and
	// target="_blank" on absolute links (bluemonday manages these; user-supplied
	// rel/target attributes are stripped and replaced by the injected values).
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	return p
}

// CommentHTML sanitizes s to the allowlist defined above.
// It is safe to call concurrently.
func CommentHTML(s string) string {
	return commentPolicy.Sanitize(s)
}
