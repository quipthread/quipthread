import { useEffect, useState } from 'preact/hooks'
import { api } from '../api'
import { IS_SELF_HOSTED } from '../lib/env'
import type { AccountInfo, BillingStatus, SecuritySettings } from '../types'
import UpgradeGate from './UpgradeGate'

const PLAN_ORDER = ['hobby', 'starter', 'pro', 'business']

// ---- SVG icons --------------------------------------------------------------

function GitHubIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 0C5.374 0 0 5.373 0 12c0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0 1 12 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576C20.566 21.797 24 17.3 24 12c0-6.627-5.373-12-12-12z" />
    </svg>
  )
}

function GoogleIcon() {
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: 18,
        height: 18,
        borderRadius: '50%',
        background: 'var(--surface)',
        border: '1px solid var(--border)',
        fontSize: '0.6875rem',
        fontWeight: 700,
        color: 'var(--text)',
        flexShrink: 0,
      }}
    >
      G
    </span>
  )
}

function EmailIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
    </svg>
  )
}

// ---- Section card -----------------------------------------------------------

function SectionCard({ children }: { children: any }) {
  return (
    <div
      style={{
        background: 'var(--card-bg)',
        border: '1px solid var(--border)',
        borderRadius: 10,
        padding: '1.375rem 1.5rem',
        marginBottom: '1.5rem',
      }}
    >
      {children}
    </div>
  )
}

function SectionHeading({ children }: { children: any }) {
  return (
    <h2
      style={{
        margin: '0 0 1.125rem',
        fontFamily: 'var(--f-display)',
        fontSize: '1rem',
        fontWeight: 600,
        color: 'var(--text)',
        letterSpacing: '-0.01em',
      }}
    >
      {children}
    </h2>
  )
}

function InlineMsg({ type, children }: { type: 'success' | 'error'; children: any }) {
  const styles: Record<string, object> = {
    success: {
      color: 'var(--green-text)',
      background: 'var(--green-bg)',
      border: '1px solid var(--green-border)',
    },
    error: {
      color: 'var(--red-text)',
      background: 'var(--red-bg)',
      border: '1px solid var(--red-border)',
    },
  }
  return (
    <div
      style={{
        borderRadius: 6,
        padding: '0.5rem 0.875rem',
        fontSize: '0.8125rem',
        marginTop: '0.75rem',
        ...styles[type],
      }}
    >
      {children}
    </div>
  )
}

// ---- Profile section --------------------------------------------------------

function ProfileSection({
  account,
  onUpdated,
}: {
  account: AccountInfo
  onUpdated: (name: string) => void
}) {
  const [displayName, setDisplayName] = useState(account.display_name)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  async function save() {
    if (!displayName.trim()) return
    setSaving(true)
    setMsg(null)
    try {
      await api.account.updateProfile(displayName.trim())
      setMsg({ type: 'success', text: 'Display name updated.' })
      onUpdated(displayName.trim())
    } catch (e: any) {
      setMsg({ type: 'error', text: e.message ?? 'Failed to save.' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <SectionCard>
      <SectionHeading>Profile</SectionHeading>
      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.25rem' }}>
        {account.avatar_url && (
          <img
            src={account.avatar_url}
            alt={account.display_name}
            style={{
              width: 48,
              height: 48,
              borderRadius: '50%',
              objectFit: 'cover',
              border: '1px solid var(--border)',
              flexShrink: 0,
            }}
          />
        )}
        <div>
          <div style={{ fontSize: '0.8125rem', color: 'var(--muted)', marginBottom: '0.125rem' }}>
            Signed in as
          </div>
          <div style={{ fontSize: '0.875rem', fontWeight: 500, color: 'var(--text)' }}>
            {account.email ||
              Object.entries(account.provider_usernames ?? {}).map(([, u]) => `@${u}`)[0] ||
              '—'}
          </div>
        </div>
      </div>

      <div style={{ marginBottom: '0.875rem' }}>
        <label
          style={{
            display: 'block',
            fontSize: '0.8125rem',
            fontWeight: 500,
            color: 'var(--muted)',
            marginBottom: '0.375rem',
          }}
        >
          Display name
        </label>
        <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
          <input
            type="text"
            value={displayName}
            onInput={(e) => setDisplayName((e.target as HTMLInputElement).value)}
            style={{ flex: 1, maxWidth: 320 }}
            disabled={saving}
          />
          <button
            type="button"
            class="btn btn-primary"
            onClick={save}
            disabled={saving || !displayName.trim() || displayName.trim() === account.display_name}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
        {msg && <InlineMsg type={msg.type}>{msg.text}</InlineMsg>}
      </div>
    </SectionCard>
  )
}

// ---- Connected accounts section ---------------------------------------------

const PROVIDER_META: Record<string, { label: string; icon: () => any }> = {
  github: { label: 'GitHub', icon: GitHubIcon },
  google: { label: 'Google', icon: GoogleIcon },
  email: { label: 'Email / Password', icon: EmailIcon },
}

function ConnectedAccountsSection({
  account,
  onRefresh,
}: {
  account: AccountInfo
  onRefresh: () => void
}) {
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState<Record<string, boolean>>({})
  const [linkError, setLinkError] = useState<string | null>(null)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const err = params.get('link_error')
    if (err) {
      setLinkError(
        err === 'already_linked'
          ? 'That account is already connected to a different Quipthread account.'
          : 'Failed to connect the account. Please try again.',
      )
      window.history.replaceState({}, '', window.location.pathname)
    }
  }, [])

  async function disconnect(provider: string) {
    setLoading((l) => ({ ...l, [provider]: true }))
    setErrors((e) => ({ ...e, [provider]: '' }))
    try {
      await api.account.disconnectIdentity(provider)
      onRefresh()
    } catch (e: any) {
      setErrors((err) => ({ ...err, [provider]: e.message ?? 'Failed to disconnect.' }))
    } finally {
      setLoading((l) => ({ ...l, [provider]: false }))
    }
  }

  function connect(provider: string) {
    window.location.href = `/auth/${provider}/link`
  }

  // Show OAuth providers that are either connected or available to connect.
  // Email/password only shown if already connected (no UI to add it post-signup).
  const visibleProviders = ['github', 'google']
    .filter((p) => account.providers.includes(p) || account.configured_providers.includes(p))
    .concat(account.providers.includes('email') ? ['email'] : [])

  return (
    <SectionCard>
      <SectionHeading>Connected Accounts</SectionHeading>
      {linkError && <InlineMsg type="error">{linkError}</InlineMsg>}
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: '0.25rem',
          marginTop: linkError ? '0.75rem' : 0,
        }}
      >
        {visibleProviders.map((provider) => {
          const meta = PROVIDER_META[provider]
          const connected = account.providers.includes(provider)
          const canDisconnect = connected && account.providers.length > 1
          const canConnect = !connected && provider !== 'email'
          const Icon = meta.icon

          return (
            <div
              key={provider}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '0.875rem',
                padding: '0.75rem 0',
                borderBottom: '1px solid var(--surface)',
              }}
            >
              <span style={{ color: 'var(--muted)', display: 'flex', alignItems: 'center' }}>
                <Icon />
              </span>
              <div style={{ flex: 1, fontSize: '0.875rem', fontWeight: 500, color: 'var(--text)' }}>
                {meta.label}
                {connected &&
                  provider !== 'email' &&
                  (() => {
                    const identifier = account.provider_usernames[provider]
                      ? `@${account.provider_usernames[provider]}`
                      : account.email || ''
                    return identifier ? (
                      <span style={{ fontWeight: 400, color: 'var(--muted)' }}>
                        {' '}
                        ({identifier})
                      </span>
                    ) : null
                  })()}
              </div>
              {connected ? (
                <span
                  style={{
                    fontSize: '0.6875rem',
                    fontWeight: 600,
                    textTransform: 'uppercase' as const,
                    letterSpacing: '0.06em',
                    color: 'var(--green-text)',
                    background: 'var(--green-bg)',
                    border: '1px solid var(--green-border)',
                    borderRadius: 9999,
                    padding: '0.15em 0.55em',
                  }}
                >
                  Connected
                </span>
              ) : (
                <span
                  style={{
                    fontSize: '0.6875rem',
                    fontWeight: 600,
                    textTransform: 'uppercase' as const,
                    letterSpacing: '0.06em',
                    color: 'var(--muted)',
                    borderRadius: 9999,
                    padding: '0.15em 0.55em',
                  }}
                >
                  Not connected
                </span>
              )}
              {canConnect && (
                <button
                  type="button"
                  class="btn"
                  style={{ fontSize: '0.8125rem' }}
                  onClick={() => connect(provider)}
                >
                  Connect
                </button>
              )}
              {canDisconnect && (
                <button
                  type="button"
                  class="btn btn-ghost"
                  style={{ fontSize: '0.8125rem' }}
                  onClick={() => disconnect(provider)}
                  disabled={loading[provider]}
                >
                  {loading[provider] ? 'Disconnecting…' : 'Disconnect'}
                </button>
              )}
              {errors[provider] && (
                <span style={{ fontSize: '0.8125rem', color: 'var(--red-text)' }}>
                  {errors[provider]}
                </span>
              )}
            </div>
          )
        })}
      </div>
    </SectionCard>
  )
}

// ---- Password section -------------------------------------------------------

function PasswordSection() {
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  async function save() {
    if (newPw.length < 8) {
      setMsg({ type: 'error', text: 'New password must be at least 8 characters.' })
      return
    }
    setSaving(true)
    setMsg(null)
    try {
      await api.account.updatePassword(currentPw, newPw)
      setMsg({ type: 'success', text: 'Password updated successfully.' })
      setCurrentPw('')
      setNewPw('')
    } catch (e: any) {
      setMsg({ type: 'error', text: e.message ?? 'Failed to update password.' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <SectionCard>
      <SectionHeading>Change Password</SectionHeading>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', maxWidth: 360 }}>
        <div>
          <label
            style={{
              display: 'block',
              fontSize: '0.8125rem',
              fontWeight: 500,
              color: 'var(--muted)',
              marginBottom: '0.375rem',
            }}
          >
            Current password
          </label>
          <input
            type="password"
            value={currentPw}
            onInput={(e) => setCurrentPw((e.target as HTMLInputElement).value)}
            style={{ width: '100%' }}
            disabled={saving}
          />
        </div>
        <div>
          <label
            style={{
              display: 'block',
              fontSize: '0.8125rem',
              fontWeight: 500,
              color: 'var(--muted)',
              marginBottom: '0.375rem',
            }}
          >
            New password
          </label>
          <input
            type="password"
            value={newPw}
            onInput={(e) => setNewPw((e.target as HTMLInputElement).value)}
            style={{ width: '100%' }}
            disabled={saving}
          />
          <div style={{ fontSize: '0.75rem', color: 'var(--muted)', marginTop: '0.25rem' }}>
            {newPw.length} / 8 characters minimum
          </div>
        </div>
        <div>
          <button
            type="button"
            class="btn btn-primary"
            onClick={save}
            disabled={saving || !currentPw || newPw.length < 8}
          >
            {saving ? 'Saving…' : 'Update password'}
          </button>
        </div>
        {msg && <InlineMsg type={msg.type}>{msg.text}</InlineMsg>}
      </div>
    </SectionCard>
  )
}

// ---- Security section -------------------------------------------------------

function SecuritySection({ billing }: { billing: BillingStatus | null }) {
  const [security, setSecurity] = useState<SecuritySettings | null>(null)
  const [siteKey, setSiteKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [secretChanged, setSecretChanged] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const planIndex = billing ? PLAN_ORDER.indexOf(billing.plan) : PLAN_ORDER.indexOf('business')
  const canAccess = IS_SELF_HOSTED || planIndex >= PLAN_ORDER.indexOf('starter')

  useEffect(() => {
    if (!canAccess) return
    api.account
      .getSecurity()
      .then((s) => {
        setSecurity(s)
        setSiteKey(s.turnstile_site_key)
      })
      .catch(() => {})
  }, [canAccess])

  async function save() {
    setSaving(true)
    setMsg(null)
    try {
      await api.account.updateSecurity(siteKey, secretChanged ? secretKey : undefined)
      setMsg({ type: 'success', text: 'Security settings saved.' })
      setSecretChanged(false)
      setSecretKey('')
      // Refresh security state
      const s = await api.account.getSecurity()
      setSecurity(s)
      setSiteKey(s.turnstile_site_key)
    } catch (e: any) {
      setMsg({ type: 'error', text: e.message ?? 'Failed to save.' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <SectionCard>
      <SectionHeading>Cloudflare Turnstile</SectionHeading>
      <p
        style={{
          fontSize: '0.875rem',
          color: 'var(--muted)',
          margin: '0 0 1.25rem',
          lineHeight: 1.6,
        }}
      >
        Add bot protection to your comment forms. Create a widget at{' '}
        <a
          href="https://dash.cloudflare.com"
          target="_blank"
          rel="noopener noreferrer"
          style={{ color: 'var(--amber)' }}
        >
          dash.cloudflare.com
        </a>{' '}
        and paste the keys below.
      </p>

      {!canAccess ? (
        <UpgradeGate
          feature="Turnstile bot protection"
          description="Protect your comment sections from bots with Cloudflare Turnstile."
          minPlan="starter"
        />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', maxWidth: 420 }}>
          <div>
            <label
              style={{
                display: 'block',
                fontSize: '0.8125rem',
                fontWeight: 500,
                color: 'var(--muted)',
                marginBottom: '0.375rem',
              }}
            >
              Site key
            </label>
            <input
              type="text"
              value={siteKey}
              onInput={(e) => setSiteKey((e.target as HTMLInputElement).value)}
              style={{ width: '100%' }}
              disabled={saving}
              placeholder="0x4AAAAAAA..."
            />
          </div>
          <div>
            <label
              style={{
                display: 'block',
                fontSize: '0.8125rem',
                fontWeight: 500,
                color: 'var(--muted)',
                marginBottom: '0.375rem',
              }}
            >
              Secret key
            </label>
            <input
              type="password"
              value={secretKey}
              onInput={(e) => {
                setSecretKey((e.target as HTMLInputElement).value)
                setSecretChanged(true)
              }}
              style={{ width: '100%' }}
              disabled={saving}
              placeholder={security?.has_turnstile_secret ? '••••••••' : ''}
            />
            {security?.has_turnstile_secret && !secretChanged && (
              <div style={{ fontSize: '0.75rem', color: 'var(--muted)', marginTop: '0.25rem' }}>
                Secret key is set — enter a new value to replace it.
              </div>
            )}
          </div>
          <div>
            <button type="button" class="btn btn-primary" onClick={save} disabled={saving}>
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
          {msg && <InlineMsg type={msg.type}>{msg.text}</InlineMsg>}
        </div>
      )}
    </SectionCard>
  )
}

// ---- Root component ---------------------------------------------------------

export default function AccountPanel() {
  const [account, setAccount] = useState<AccountInfo | null>(null)
  const [billing, setBilling] = useState<BillingStatus | null>(null)
  const [loadErr, setLoadErr] = useState<string | null>(null)

  function load() {
    api.account
      .get()
      .then(setAccount)
      .catch((e: any) => {
        setLoadErr(e.message ?? 'Failed to load account.')
      })
  }

  useEffect(() => {
    load()
    if (!IS_SELF_HOSTED) {
      api.billing
        .status()
        .then(setBilling)
        .catch(() => {
          setBilling({ plan: 'business' } as BillingStatus)
        })
    }
  }, [])

  if (loadErr) {
    return (
      <div class="error-msg" style={{ marginTop: '2rem' }}>
        {loadErr}
      </div>
    )
  }

  if (!account) {
    return <div class="loading">Loading…</div>
  }

  return (
    <div>
      <div class="page-header">
        <h1>Account</h1>
      </div>

      <ProfileSection
        account={account}
        onUpdated={(name) => setAccount((a) => (a ? { ...a, display_name: name } : a))}
      />

      <ConnectedAccountsSection account={account} onRefresh={load} />

      {account.providers.includes('email') && <PasswordSection />}

      <SecuritySection billing={billing} />
    </div>
  )
}
