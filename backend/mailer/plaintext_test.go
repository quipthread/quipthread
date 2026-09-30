package mailer

import "testing"

func TestPlainTextFromHTMLKeepsVisibleText(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"drops tags", `<p>Hello</p>`, "Hello"},
		{"keeps link label and target", `<a href="https://example.test">Reset password</a>`, "Reset password (https://example.test)"},
		{"does not repeat a link label that is the target", `<a href="https://example.test">https://example.test</a>`, "https://example.test"},
		{"decodes entities", `Hi &amp; bye`, "Hi & bye"},
		{"collapses whitespace", "<p>a</p>\n\n<p>b</p>", "a b"},
		{
			"drops head, style, and script source",
			`<!DOCTYPE html><html><head><title>Subject</title><style>body{color:#E07F32}</style><script>var leak=1;</script></head><body><p>Hello</p></body></html>`,
			"Hello",
		},
		{
			"keeps body text after a style block",
			`<body><style>.x{color:red}</style><p>Read me</p></body>`,
			"Read me",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := plainTextFromHTML(test.html); got != test.want {
				t.Fatalf("plainTextFromHTML(%q) = %q, want %q", test.html, got, test.want)
			}
		})
	}
}
