import type { CSSProperties } from 'react'
import { useEffect, useRef, useState } from 'react'
import { AuthModal } from './AuthModal'
import { fetchConfig, getMe } from './api'
import { CommentForm } from './CommentForm'
import { CommentList } from './CommentList'
import { useTranslations } from './i18n'
import type { Comment, User } from './types'

interface CommentWidgetProps {
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  lang: string
  theme?: string
  customVars?: Record<string, string>
}

export function CommentWidget({
  siteId,
  pageId,
  pageUrl,
  pageTitle,
  lang,
  theme,
  customVars,
}: CommentWidgetProps) {
  const t = useTranslations(lang)
  const [user, setUser] = useState<User | null>(null)
  const [authLoading, setAuthLoading] = useState(true)
  const [showAuthModal, setShowAuthModal] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('')
  const [turnstileReady, setTurnstileReady] = useState(false)
  // Priority: explicit data-theme (non-auto) > DB theme > auto
  const [activeTheme, setActiveTheme] = useState<string>(theme && theme !== 'auto' ? theme : 'auto')
  const rootRef = useRef<HTMLDivElement>(null)
  // Cache the DB theme so the MutationObserver can fall back to it without re-fetching.
  const dbTheme = useRef<string>('auto')

  useEffect(() => {
    getMe().then((u) => {
      setUser(u)
      setAuthLoading(false)
    })
    fetchConfig(siteId).then((cfg) => {
      setTurnstileSiteKey(cfg.turnstileSiteKey)
      dbTheme.current = cfg.theme ?? 'auto'
      // Only apply the DB theme if no explicit override is on the container.
      if (!theme || theme === 'auto') {
        setActiveTheme(dbTheme.current)
      }
    })
  }, [siteId, theme])

  // Watch the host page's container for data-theme changes (e.g. page-level theme toggles).
  // Updates activeTheme state so React stays in sync rather than fighting DOM mutations.
  useEffect(() => {
    const container = rootRef.current?.parentElement
    if (!container) return
    const observer = new MutationObserver(() => {
      const containerTheme = container.dataset.theme
      if (containerTheme && containerTheme !== 'auto') {
        setActiveTheme(containerTheme)
      } else {
        setActiveTheme(dbTheme.current)
      }
    })
    observer.observe(container, { attributes: true, attributeFilter: ['data-theme'] })
    return () => observer.disconnect()
  }, [])

  // Load the Turnstile script once we know the site key.
  useEffect(() => {
    if (!turnstileSiteKey) return

    const existing = document.querySelector('script[src*="challenges.cloudflare.com/turnstile"]')
    if (existing) {
      setTurnstileReady(true)
      return
    }

    const script = document.createElement('script')
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    script.async = true
    script.onload = () => setTurnstileReady(true)
    document.head.appendChild(script)
  }, [turnstileSiteKey])

  const handleCommentSuccess = (comment: Comment) => {
    if (comment.status === 'approved') {
      setRefreshKey((k) => k + 1)
    }
  }

  const handleLogout = async () => {
    try {
      await fetch('/auth/logout', { method: 'POST', credentials: 'include' })
    } finally {
      setUser(null)
    }
  }

  const _commentCount = undefined // Could be fetched separately; left for a later pass

  return (
    <div
      ref={rootRef}
      className="qt-root"
      data-theme={activeTheme}
      style={
        customVars && Object.keys(customVars).length > 0 ? (customVars as CSSProperties) : undefined
      }
    >
      <div className="qt-header">
        <h2 className="qt-title">{t.comments}</h2>
        {!authLoading && user && (
          <div className="qt-user-bar">
            {user.display_name && <span className="qt-user-bar-name">{user.display_name}</span>}
            <button type="button" className="qt-btn qt-btn-ghost" onClick={handleLogout}>
              {t.signOut}
            </button>
          </div>
        )}
      </div>

      <CommentList
        siteId={siteId}
        pageId={pageId}
        pageUrl={pageUrl}
        pageTitle={pageTitle}
        user={user}
        t={t}
        refreshKey={refreshKey}
      />

      <div className="qt-new-comment">
        {authLoading ? null : user ? (
          <CommentForm
            siteId={siteId}
            pageId={pageId}
            pageUrl={pageUrl}
            pageTitle={pageTitle}
            t={t}
            onSuccess={handleCommentSuccess}
            turnstileSiteKey={turnstileReady ? turnstileSiteKey : ''}
          />
        ) : (
          <div className="qt-login-prompt">
            <p>{t.loginToComment}</p>
            <button
              type="button"
              className="qt-btn qt-btn-primary"
              onClick={() => setShowAuthModal(true)}
            >
              {t.signIn}
            </button>
          </div>
        )}
      </div>

      {showAuthModal && <AuthModal t={t} onClose={() => setShowAuthModal(false)} />}
    </div>
  )
}
