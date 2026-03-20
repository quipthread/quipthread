package handlers

import (
	"fmt"
	"html"
	"net/http"
)

// HandleEmbedPreview serves a minimal HTML page that loads the embed widget in
// preview mode. No auth required — intended to be loaded in an iframe by the
// dashboard Preview page. The widget's comment-create endpoint rejects requests
// where pageId == "__preview__", so no test data is ever persisted.
func HandleEmbedPreview(w http.ResponseWriter, r *http.Request) {
	siteID := html.EscapeString(r.URL.Query().Get("siteId"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	fmt.Fprintf(w, embedPreviewHTML, siteID) //nolint:errcheck // ResponseWriter.Write errors are not actionable
}

const embedPreviewHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Quipthread — Embed Preview</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
      background: #ffffff;
      min-height: 100vh;
    }
    @media (prefers-color-scheme: dark) {
      body { background: #0F0F0F; }
    }
    .preview-banner {
      position: sticky;
      top: 0;
      z-index: 100;
      background: #FEF3C7;
      color: #92400E;
      border-bottom: 1px solid #FCD34D;
      padding: 0.5rem 1.25rem;
      font-size: 0.8125rem;
      font-weight: 500;
      text-align: center;
      letter-spacing: 0.01em;
    }
    .preview-content {
      padding: 2rem 1.5rem;
      max-width: 720px;
      margin: 0 auto;
    }
  </style>
</head>
<body>
  <div class="preview-banner">Preview mode — commenting is disabled.</div>
  <div class="preview-content">
    <div id="comments" data-site-id="%s" data-page-id="__preview__"></div>
  </div>
  <script src="/embed.js"></script>
  <script>
    window.addEventListener('message', function(e) {
      if (e.origin !== window.location.origin) return;
      if (!e.data || e.data.type !== 'qt:theme') return;
      var c = document.getElementById('comments');
      if (c) c.dataset.theme = e.data.theme;
    });
  </script>
</body>
</html>`
