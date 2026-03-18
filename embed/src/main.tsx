import { createRoot } from 'react-dom/client'
import { initApi } from './api'
import { injectStyles } from './styles'
import { CommentWidget } from './CommentWidget'

// Capture the script's origin synchronously during IIFE evaluation.
// document.currentScript is only set while the script tag is being parsed.
const scriptEl = document.currentScript as HTMLScriptElement | null
const apiBase = scriptEl?.src ? new URL(scriptEl.src).origin : ''

function mount() {
  injectStyles()
  initApi(apiBase)

  const container = document.getElementById('comments')
  if (!container) return

  const {
    siteId = '',
    pageId = '',
    pageUrl,
    pageTitle,
    lang = 'en',
    theme,
  } = container.dataset

  if (!siteId || !pageId) {
    console.error('[quipthread] data-site-id and data-page-id are required on #comments')
    return
  }

  createRoot(container).render(
    <CommentWidget
      siteId={siteId}
      pageId={pageId}
      pageUrl={pageUrl}
      pageTitle={pageTitle}
      lang={lang}
      theme={theme}
    />,
  )

}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mount)
} else {
  mount()
}
