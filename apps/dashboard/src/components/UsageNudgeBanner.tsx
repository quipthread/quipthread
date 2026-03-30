import { useEffect, useState } from 'preact/hooks'
import { api } from '../api'
import type { BillingStatus } from '../types'

type Nudge = {
  level: 'warning' | 'critical'
  message: string
  detail: string
  pct?: number // 0–100, for comment quota progress bar
}

function computeNudge(s: BillingStatus): Nudge | null {
  if (s.plan !== 'hobby') return null

  const commentPct =
    s.comments_limit > 0 ? Math.round((s.comments_this_month / s.comments_limit) * 100) : 0

  // Comment quota takes priority over site limit.
  if (s.comments_limit > 0) {
    if (commentPct >= 100) {
      return {
        level: 'critical',
        message: 'Monthly comment limit reached',
        detail: `${s.comments_this_month.toLocaleString()} / ${s.comments_limit.toLocaleString()} comments used. New comments are paused until next month.`,
        pct: 100,
      }
    }
    if (commentPct >= 95) {
      return {
        level: 'critical',
        message: 'Almost out of comments',
        detail: `${s.comments_this_month.toLocaleString()} of ${s.comments_limit.toLocaleString()} used this month.`,
        pct: commentPct,
      }
    }
    if (commentPct >= 80) {
      return {
        level: 'warning',
        message: 'Approaching comment limit',
        detail: `${s.comments_this_month.toLocaleString()} of ${s.comments_limit.toLocaleString()} used this month.`,
        pct: commentPct,
      }
    }
  }

  // Site limit nudge (only if not already showing comment nudge).
  if (s.sites_limit !== null && s.sites_limit > 0 && s.sites_count >= s.sites_limit) {
    return {
      level: 'warning',
      message: 'Site limit reached',
      detail: `You're on the Hobby plan, which includes 1 site.`,
    }
  }

  return null
}

const DISMISS_KEY = 'qt-nudge-dismissed'

export default function UsageNudgeBanner() {
  const [nudge, setNudge] = useState<Nudge | null>(null)
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    // Check if already dismissed this session.
    if (sessionStorage.getItem(DISMISS_KEY)) {
      setDismissed(true)
      return
    }

    api.billing
      .status()
      .then((s) => {
        const n = computeNudge(s)
        setNudge(n)
      })
      .catch(() => {})
  }, [])

  if (!nudge || dismissed) return null

  function dismiss() {
    sessionStorage.setItem(DISMISS_KEY, '1')
    setDismissed(true)
  }

  const isCritical = nudge.level === 'critical'
  const bg = isCritical ? 'var(--red-bg)' : 'var(--amber-bg)'
  const border = isCritical ? 'var(--red-border)' : 'var(--amber-border)'
  const textColor = isCritical ? 'var(--red-text)' : 'var(--amber)'
  const barFill = isCritical ? '#ef4444' : '#e07f32'

  return (
    <div
      style={{
        background: bg,
        border: `1px solid ${border}`,
        borderRadius: 8,
        padding: '0.75rem 1rem',
        marginBottom: '1.5rem',
        display: 'flex',
        alignItems: 'flex-start',
        gap: '0.75rem',
      }}
      role="alert"
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <div
          style={{
            display: 'flex',
            alignItems: 'baseline',
            gap: '0.5rem',
            flexWrap: 'wrap' as const,
          }}
        >
          <span
            style={{
              fontSize: '0.875rem',
              fontWeight: 600,
              color: textColor,
            }}
          >
            {nudge.message}
          </span>
          <span style={{ fontSize: '0.8125rem', color: 'var(--muted)' }}>{nudge.detail}</span>
          <a
            href="/dashboard/billing"
            style={{
              fontSize: '0.8125rem',
              fontWeight: 600,
              color: textColor,
              textDecoration: 'none',
              whiteSpace: 'nowrap' as const,
            }}
          >
            Upgrade plan &rarr;
          </a>
        </div>

        {nudge.pct !== undefined && (
          <div
            style={{
              marginTop: '0.5rem',
              height: 4,
              borderRadius: 9999,
              background: border,
              overflow: 'hidden',
              maxWidth: 320,
            }}
          >
            <div
              style={{
                height: '100%',
                width: `${nudge.pct}%`,
                background: barFill,
                borderRadius: 9999,
                transition: 'width 0.3s ease',
              }}
            />
          </div>
        )}
      </div>

      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        style={{
          background: 'transparent',
          border: 'none',
          cursor: 'pointer',
          padding: '0.125rem',
          color: textColor,
          opacity: 0.6,
          flexShrink: 0,
          lineHeight: 1,
        }}
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor" aria-hidden="true">
          <path d="M1.4 1.4a1 1 0 0 1 1.42 0L7 5.6l4.18-4.2a1 1 0 1 1 1.42 1.42L8.4 7l4.2 4.18a1 1 0 1 1-1.42 1.42L7 8.4l-4.18 4.2a1 1 0 1 1-1.42-1.42L5.6 7 1.4 2.82a1 1 0 0 1 0-1.42z" />
        </svg>
      </button>
    </div>
  )
}
