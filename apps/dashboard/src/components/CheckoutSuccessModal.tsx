import { useState, useEffect, useRef } from 'preact/hooks'
import { api } from '../api'
import { PLAN_LABELS, PLAN_FEATURES } from '../lib/plan'
import type { Plan } from '../lib/plan'

const POLL_INTERVAL = 1500
const MAX_ATTEMPTS = 12 // ~18 seconds

export default function CheckoutSuccessModal() {
  const [visible, setVisible] = useState(false)
  const [plan, setPlan] = useState<Plan | null>(null)
  const [confirming, setConfirming] = useState(true)
  const attempts = useRef(0)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('checkout') !== 'success') return

    // Remove the param from URL immediately
    const clean = window.location.pathname
    window.history.replaceState({}, '', clean)

    setVisible(true)

    function poll() {
      api.billing.status()
        .then(status => {
          if (status.plan !== 'hobby') {
            setPlan(status.plan as Plan)
            setConfirming(false)
            // Sync nav + localStorage
            document.documentElement.dataset.plan = status.plan
            localStorage.setItem('qt-plan', status.plan)
          } else if (attempts.current < MAX_ATTEMPTS) {
            attempts.current++
            setTimeout(poll, POLL_INTERVAL)
          } else {
            // Timed out waiting — show whatever plan we have
            setPlan(status.plan as Plan)
            setConfirming(false)
          }
        })
        .catch(() => {
          if (attempts.current < MAX_ATTEMPTS) {
            attempts.current++
            setTimeout(poll, POLL_INTERVAL)
          } else {
            setConfirming(false)
          }
        })
    }

    poll()
  }, [])

  if (!visible) return null

  const features = plan ? PLAN_FEATURES[plan] : null

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '1.5rem',
        background: 'rgba(0,0,0,0.55)',
        backdropFilter: 'blur(4px)',
      }}
      onClick={e => { if (e.target === e.currentTarget) setVisible(false) }}
    >
      <div
        style={{
          background: 'var(--card-bg)',
          border: '1px solid var(--border)',
          borderRadius: 14,
          padding: '2rem 2.25rem',
          maxWidth: 460,
          width: '100%',
          boxShadow: '0 16px 48px rgba(0,0,0,0.2)',
        }}
      >
        {confirming ? (
          <div style={{ textAlign: 'center', padding: '1.5rem 0' }}>
            <div style={{
              width: 40, height: 40, borderRadius: '50%',
              border: '3px solid var(--border)',
              borderTopColor: 'var(--amber)',
              margin: '0 auto 1.25rem',
              animation: 'spin 0.8s linear infinite',
            }} />
            <p style={{ margin: 0, color: 'var(--muted)', fontSize: '0.9375rem' }}>
              Confirming your subscription...
            </p>
          </div>
        ) : (
          <>
            {/* Header */}
            <div style={{ textAlign: 'center', marginBottom: '1.5rem' }}>
              <div style={{
                width: 48, height: 48, borderRadius: '50%',
                background: 'var(--amber-bg)',
                border: '2px solid var(--amber)',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                margin: '0 auto 1rem',
              }}>
                <svg width="22" height="22" viewBox="0 0 22 22" fill="none">
                  <path d="M4 11.5l5 5 9-9.5" stroke="var(--amber)" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
              </div>
              <h2 style={{
                margin: '0 0 0.375rem',
                fontFamily: 'var(--f-display)',
                fontSize: '1.375rem',
                fontWeight: 600,
              }}>
                {plan && plan !== 'hobby'
                  ? `Welcome to ${PLAN_LABELS[plan]}!`
                  : 'You\'re all set!'}
              </h2>
              <p style={{ margin: 0, color: 'var(--muted)', fontSize: '0.9375rem' }}>
                {plan && plan !== 'hobby'
                  ? `Your ${PLAN_LABELS[plan]} plan is now active.`
                  : 'Your subscription is active.'}
              </p>
            </div>

            {/* Feature list */}
            {features && plan !== 'hobby' && (
              <div style={{
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                borderRadius: 8,
                padding: '1rem 1.125rem',
                marginBottom: '1.5rem',
              }}>
                <div style={{
                  fontSize: '0.6875rem',
                  fontWeight: 700,
                  textTransform: 'uppercase',
                  letterSpacing: '0.08em',
                  color: 'var(--muted)',
                  marginBottom: '0.625rem',
                }}>
                  What you now have access to
                </div>
                <div style={{ fontSize: '0.8125rem', color: 'var(--muted)', marginBottom: '0.375rem' }}>
                  {features.sites} &middot; {features.comments}
                </div>
                <ul style={{ margin: '0.5rem 0 0', paddingLeft: '1.125rem', lineHeight: 1.8, fontSize: '0.875rem' }}>
                  {features.highlights.map(h => (
                    <li key={h} style={{ color: 'var(--text)' }}>{h}</li>
                  ))}
                </ul>
              </div>
            )}

            {/* CTA */}
            <div style={{ display: 'flex', gap: '0.625rem' }}>
              <button
                className="btn btn-primary"
                style={{ flex: 1, justifyContent: 'center' }}
                onClick={() => setVisible(false)}
              >
                Get started
              </button>
              <a
                href="/dashboard/billing"
                className="btn"
                style={{ textDecoration: 'none' }}
                onClick={() => setVisible(false)}
              >
                View billing
              </a>
            </div>
          </>
        )}
      </div>

      <style>{`
        @keyframes spin {
          to { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  )
}
