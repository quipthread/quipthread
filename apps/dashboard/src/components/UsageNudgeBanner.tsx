import { useQuery } from '@tanstack/preact-query'
import { useState } from 'preact/hooks'
import { api } from '../api'
import { queryKeys } from '../lib/queryKeys'
import { computeNudge } from '../lib/usageNudge'
import QueryProvider from './QueryProvider'

function UsageNudgeBannerInner() {
  const [dismissedKey, setDismissedKey] = useState<string | null>(null)

  const { data } = useQuery({
    queryKey: queryKeys.billingStatus(),
    queryFn: () => api.billing.status(),
    staleTime: 60_000,
    refetchInterval: 60_000,
  })

  const nudge = data ? computeNudge(data) : null

  const period = new Date().toISOString().slice(0, 7)
  const key = nudge
    ? `${data?.plan}:${period}:${nudge.metric}:${nudge.level}:${nudge.pct === 100}`
    : null
  if (!nudge || dismissedKey === key) return null

  function dismiss() {
    setDismissedKey(key)
  }

  const isCritical = nudge.level === 'critical'
  const bg = isCritical ? 'var(--red-bg)' : 'var(--amber-bg)'
  const border = isCritical ? 'var(--red-border)' : 'var(--amber-border)'
  const textColor = isCritical ? 'var(--red-text)' : 'var(--amber)'
  const barFill = isCritical ? 'var(--red-text)' : 'var(--amber)'

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
                width: '100%',
                transform: `scaleX(${nudge.pct / 100})`,
                transformOrigin: 'left',
                background: barFill,
                borderRadius: 9999,
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

export default function UsageNudgeBanner() {
  return (
    <QueryProvider>
      <UsageNudgeBannerInner />
    </QueryProvider>
  )
}
