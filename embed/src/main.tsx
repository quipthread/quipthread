import { createRoot } from 'react-dom/client'
import { initApi } from './api'
import { CommentWidget } from './CommentWidget'
import { injectStyles } from './styles'

// Capture the script's origin synchronously during IIFE evaluation.
// document.currentScript is only set while the script tag is being parsed.
const scriptEl = document.currentScript as HTMLScriptElement | null
const apiBase = scriptEl?.src ? new URL(scriptEl.src).origin : ''

// CSS custom property overrides readable from data-qt-* attributes on the container.
// data-qt-accent → --qt-accent, data-qt-bg → --qt-bg, etc.
const CSS_VAR_ATTRS: [string, string][] = [
  ['qtAccent', '--qt-accent'],
  ['qtAccentHover', '--qt-accent-hover'],
  ['qtBg', '--qt-bg'],
  ['qtBgAlt', '--qt-bg-alt'],
  ['qtText', '--qt-text'],
  ['qtTextMuted', '--qt-text-muted'],
  ['qtBorder', '--qt-border'],
  ['qtRadius', '--qt-radius'],
  ['qtRadiusSm', '--qt-radius-sm'],
  ['qtRadiusLg', '--qt-radius-lg'],
  ['qtFont', '--qt-font'],
]

function mount() {
  injectStyles()
  initApi(apiBase)

  const container = document.getElementById('comments')
  if (!container) return

  const { siteId = '', pageId = '', pageUrl, pageTitle, lang = 'en', theme } = container.dataset

  if (!siteId || !pageId) {
    console.error('[quipthread] data-site-id and data-page-id are required on #comments')
    return
  }

  const customVars: Record<string, string> = {}
  for (const [dataKey, cssVar] of CSS_VAR_ATTRS) {
    const val = container.dataset[dataKey]
    if (val) customVars[cssVar] = val
  }

  createRoot(container).render(
    <CommentWidget
      siteId={siteId}
      pageId={pageId}
      pageUrl={pageUrl}
      pageTitle={pageTitle}
      lang={lang}
      theme={theme}
      customVars={customVars}
    />,
  )
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mount)
} else {
  mount()
}
