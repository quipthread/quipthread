package sanitize_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/quipthread/quipthread/sanitize"
)

func TestCommentHTMLPreservesEditorFormatting(t *testing.T) {
	t.Parallel()
	// Given
	inputs := []string{
		`<p><strong>bold</strong><em>italic</em><s>strike</s><u>underline</u><code>inline</code><br/>next</p>`,
		`<p><mark>highlight</mark><sub>subscript</sub><sup>superscript</sup></p>`,
		`<h1>one</h1><h2>two</h2><h3>three</h3><h4>four</h4><h5>five</h5><h6>six</h6>`,
		`<blockquote><p>quote</p></blockquote><pre><code>block</code></pre><hr/>`,
		`<ul><li><p>bullet</p></li></ul><ol start="3"><li><p>numbered</p></li></ol>`,
		`<ol start="0"><li>zero</li></ol>`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			// When
			got := sanitize.CommentHTML(input)
			// Then
			if got != input {
				t.Errorf("CommentHTML() = %q, want %q", got, input)
			}
		})
	}
}

func TestCommentHTMLPreservesSupportedAlignment(t *testing.T) {
	t.Parallel()
	// Given
	for _, tag := range []string{"p", "h1", "h2", "h3", "h4", "h5", "h6"} {
		for _, alignment := range []string{"left", "center", "right", "justify"} {
			t.Run(tag+"/"+alignment, func(t *testing.T) {
				input := fmt.Sprintf(`<%s style="text-align: %s">text</%s>`, tag, alignment, tag)
				// When
				got := sanitize.CommentHTML(input)
				// Then
				if got != input {
					t.Errorf("CommentHTML() = %q, want %q", got, input)
				}
			})
		}
	}
}

func TestCommentHTMLStripsUnsafeMarkup(t *testing.T) {
	t.Parallel()
	// Given
	tests := []struct{ name, input, want string }{
		{"unapproved CSS", `<p style="color:red; position:fixed; text-align:center; background-image:url(https://example.com/pixel)">text</p>`, `<p style="text-align: center">text</p>`},
		{"CSS expression", `<h6 style="text-align:expression(alert(1))">text</h6>`, `<h6>text</h6>`},
		{"CSS variable", `<p style="text-align:var(--alignment)">text</p>`, `<p>text</p>`},
		{"CSS global value", `<p style="text-align:inherit">text</p>`, `<p>text</p>`},
		{"CSS URL", `<p style="text-align:url(javascript:alert(1))">text</p>`, `<p>text</p>`},
		{"styles on other blocks", `<blockquote style="text-align:center"><pre style="text-align:right">text</pre></blockquote>`, `<blockquote><pre>text</pre></blockquote>`},
		{"highlight attributes", `<mark style="background-color:red; text-align:center" data-color="red" class="custom" onclick="alert(1)">text</mark>`, `<mark>text</mark>`},
		{"inline styles", `<sub style="text-align:center">sub</sub><sup style="color:red">sup</sup>`, `<sub>sub</sub><sup>sup</sup>`},
		{"event handlers", `<h4 onclick="alert(1)">heading</h4><hr onmouseover="alert(1)">`, `<h4>heading</h4><hr>`},
		{"script and iframe", `<p>safe<script>alert(1)</script><iframe src="https://example.com">frame</iframe></p>`, `<p>safe</p>`},
		{"media and table", `<img src="https://example.com/a.png"><video src="https://example.com/a.mp4"></video><table><tr><td>cell</td></tr></table>`, `cell`},
		{"list start injection", `<ol start="1; color:red"><li>item</li></ol>`, `<ol><li>item</li></ol>`},
		{"list start decimal", `<ol start="1.5"><li>item</li></ol>`, `<ol><li>item</li></ol>`},
		{"start on other tags", `<ul start="3"><li start="3">item</li></ul>`, `<ul><li>item</li></ul>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := sanitize.CommentHTML(tt.input)
			// Then
			if got != tt.want {
				t.Errorf("CommentHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommentHTMLSecuresLinks(t *testing.T) {
	t.Parallel()
	// Given
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			input := fmt.Sprintf(`<a href="%s://example.com" rel="opener" target="_self" onclick="alert(1)">link</a>`, scheme)
			want := fmt.Sprintf(`<a href="%s://example.com" rel="nofollow noreferrer noopener" target="_blank">link</a>`, scheme)
			// When
			got := sanitize.CommentHTML(input)
			// Then
			if got != want {
				t.Errorf("CommentHTML() = %q, want %q", got, want)
			}
		})
	}
}

func TestCommentHTMLStripsUnsupportedLinkSchemes(t *testing.T) {
	t.Parallel()
	// Given
	for _, href := range []string{"javascript:alert(1)", "JaVaScRiPt:alert(1)", "java&#x73;cript:alert(1)", "java&#10;script:alert(1)", "data:text/html,evil", "vbscript:alert(1)", "mailto:person@example.com", "/relative", "//example.com"} {
		t.Run(href, func(t *testing.T) {
			// When
			got := sanitize.CommentHTML(`<a href="` + href + `">link</a>`)
			// Then
			if strings.Contains(got, "href=") {
				t.Errorf("CommentHTML() retained unsupported href: %s", got)
			}
		})
	}
}

func TestCommentHTMLIsIdempotent(t *testing.T) {
	t.Parallel()
	// Given
	for _, input := range []string{
		`<h4 style="text-align: right">heading</h4><p><mark>highlight</mark><sub>2</sub><sup>3</sup></p><hr><ol start="4"><li>item</li></ol>`,
		`<p style="text-align:center;color:red" onclick="alert(1)"><a href="https://example.com" target="_self">link</a><img src=x onerror=alert(1)></p>`,
		`<p><a href="javascript:alert(1)">unsafe</a><mark data-color="red">text &amp; more</mark></p>`,
	} {
		t.Run(input, func(t *testing.T) {
			clean := sanitize.CommentHTML(input)
			// When
			got := sanitize.CommentHTML(clean)
			// Then
			if got != clean {
				t.Errorf("sanitizing twice = %q, want %q", got, clean)
			}
		})
	}
}
