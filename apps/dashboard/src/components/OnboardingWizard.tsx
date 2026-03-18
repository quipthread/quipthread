import { useState, useEffect } from 'preact/hooks'
import { api, API } from '../api'
import type { Site } from '../types'
import EmbedCodeGenerator from './EmbedCodeGenerator'

type Step = 'auth' | 'site' | 'embed' | 'done'

const STEPS = ['site', 'embed'] as const

function StepIndicator({ current }: { current: typeof STEPS[number] }) {
  const LABELS = { site: 'Your site', embed: 'Add to your site' }
  const idx = STEPS.indexOf(current)
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 0,
        marginBottom: '2.5rem',
        justifyContent: 'center',
      }}
    >
      {STEPS.map((s, i) => {
        const done = i < idx
        const active = i === idx
        return (
          <>
            <div
              key={s}
              style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '0.375rem' }}
            >
              <div
                style={{
                  width: 28,
                  height: 28,
                  borderRadius: '50%',
                  background: active
                    ? 'var(--ink)'
                    : done
                    ? 'var(--amber)'
                    : 'var(--surface)',
                  border: active
                    ? '2px solid var(--ink)'
                    : done
                    ? '2px solid var(--amber)'
                    : '2px solid var(--border)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: active ? 'var(--paper)' : done ? 'white' : 'var(--muted)',
                  fontSize: '0.75rem',
                  fontWeight: 700,
                  transition: 'all 0.2s',
                }}
              >
                {done ? (
                  <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="1.5,6 5,9.5 10.5,2.5" />
                  </svg>
                ) : (
                  i + 1
                )}
              </div>
              <span
                style={{
                  fontSize: '0.6875rem',
                  fontWeight: 500,
                  color: active ? 'var(--text)' : done ? 'var(--amber)' : 'var(--muted)',
                  whiteSpace: 'nowrap',
                }}
              >
                {LABELS[s]}
              </span>
            </div>
            {i < STEPS.length - 1 && (
              <div
                style={{
                  height: 2,
                  width: 64,
                  background: done ? 'var(--amber)' : 'var(--border)',
                  marginBottom: '1.25rem',
                  flexShrink: 0,
                  transition: 'background 0.2s',
                }}
              />
            )}
          </>
        )
      })}
    </div>
  )
}

export default function OnboardingWizard() {
  const [step, setStep] = useState<Step>('auth')
  const [authError, setAuthError] = useState(false)

  const [domain, setDomain] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [site, setSite] = useState<Site | null>(null)

  // Auth check on mount
  useEffect(() => {
    api.me()
      .then(() => setStep('site'))
      .catch(() => {
        setAuthError(true)
        const returnTo = encodeURIComponent(window.location.href)
        window.location.href = `${API}/auth/github/login?returnTo=${returnTo}`
      })
  }, [])

  const handleCreateSite = async (e: SubmitEvent) => {
    e.preventDefault()
    const trimmed = domain.trim()
    if (!trimmed) return
    setCreating(true)
    setCreateError(null)
    try {
      const created = await api.sites.create(trimmed) as Site
      setSite(created)
      setStep('embed')
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create site.')
    } finally {
      setCreating(false)
    }
  }

  if (step === 'auth' || authError) {
    return (
      <div style={{ textAlign: 'center', padding: '4rem', color: 'var(--muted)' }}>
        Redirecting…
      </div>
    )
  }

  if (step === 'done') {
    return (
      <div style={{ textAlign: 'center', padding: '4rem' }}>
        <p style={{ color: 'var(--muted)' }}>Redirecting to your dashboard…</p>
      </div>
    )
  }

  return (
    <div
      style={{
        maxWidth: 560,
        margin: '0 auto',
        padding: '2rem 1.5rem 3rem',
        width: '100%',
      }}
    >
      <StepIndicator current={step as typeof STEPS[number]} />

      {/* ── Step 1: Site ──────────────────────────────────── */}
      {step === 'site' && (
        <div>
          <h1
            style={{
              margin: '0 0 0.375rem',
              fontFamily: 'var(--f-display)',
              fontSize: '1.5rem',
              fontWeight: 600,
              color: 'var(--text)',
            }}
          >
            Set up your first site
          </h1>
          <p style={{ margin: '0 0 2rem', color: 'var(--muted)', fontSize: '0.9375rem' }}>
            Enter the domain where you want to add comments.
          </p>

          <form onSubmit={handleCreateSite}>
            <div style={{ marginBottom: '1rem' }}>
              <label
                style={{
                  display: 'block',
                  marginBottom: '0.375rem',
                  fontSize: '0.875rem',
                  fontWeight: 500,
                  color: 'var(--text)',
                }}
              >
                Site domain
              </label>
              <input
                type="text"
                placeholder="example.com"
                value={domain}
                onChange={e => setDomain(e.currentTarget.value)}
                disabled={creating}
                style={{ width: '100%' }}
                autoFocus
              />
              <p style={{ margin: '0.375rem 0 0', fontSize: '0.8125rem', color: 'var(--muted)' }}>
                Enter the root domain without https:// — e.g. myblog.com
              </p>
            </div>

            {createError && (
              <div className="error-msg" style={{ marginBottom: '1rem' }}>
                {createError}
              </div>
            )}

            <button
              type="submit"
              className="btn btn-primary"
              disabled={creating || !domain.trim()}
              style={{ width: '100%', justifyContent: 'center', padding: '0.625rem 1rem', fontSize: '0.9375rem' }}
            >
              {creating ? 'Creating…' : 'Continue'}
            </button>
          </form>
        </div>
      )}

      {/* ── Step 2: Embed ─────────────────────────────────── */}
      {step === 'embed' && site && (
        <div>
          <h1
            style={{
              margin: '0 0 0.375rem',
              fontFamily: 'var(--f-display)',
              fontSize: '1.5rem',
              fontWeight: 600,
              color: 'var(--text)',
            }}
          >
            Add comments to your site
          </h1>
          <p style={{ margin: '0 0 1.75rem', color: 'var(--muted)', fontSize: '0.9375rem' }}>
            Copy the snippet for your framework and drop it wherever you want comments to appear.
          </p>

          {/* Site ID reference */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.5rem',
              padding: '0.625rem 0.875rem',
              background: 'white',
              border: '1px solid var(--border)',
              borderRadius: 7,
              marginBottom: '1.25rem',
              fontSize: '0.875rem',
            }}
          >
            <span style={{ color: 'var(--muted)', flexShrink: 0 }}>Site ID:</span>
            <code
              style={{
                fontFamily: 'var(--f-mono)',
                fontSize: '0.8125rem',
                color: 'var(--text)',
                flex: 1,
                wordBreak: 'break-all',
              }}
            >
              {site.id}
            </code>
          </div>

          <EmbedCodeGenerator siteId={site.id} apiBase={API || window.location.origin} />

          <button
            className="btn btn-primary"
            onClick={() => {
              setStep('done')
              window.location.href = '/dashboard/comments'
            }}
            style={{
              width: '100%',
              justifyContent: 'center',
              padding: '0.625rem 1rem',
              fontSize: '0.9375rem',
              marginTop: '1.75rem',
            }}
          >
            Go to dashboard
          </button>
        </div>
      )}
    </div>
  )
}
