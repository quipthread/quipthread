import { useState } from 'preact/hooks'
import { API } from '../api'

export default function LoginForm() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [unverified, setUnverified] = useState(false)
  const [resendSent, setResendSent] = useState(false)
  const [resending, setResending] = useState(false)

  const handleResend = async () => {
    setResending(true)
    try {
      await fetch(`${API}/auth/email/resend-verification`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ email }),
      })
      setResendSent(true)
    } finally {
      setResending(false)
    }
  }

  const handleSubmit = async (e: SubmitEvent) => {
    e.preventDefault()
    setError(null)
    setUnverified(false)
    setLoading(true)

    try {
      const res = await fetch(`${API}/auth/email/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ email, password }),
      })
      const data = await res.json().catch(() => ({})) as { error?: string; code?: string; message?: string }
      if (!res.ok) {
        if (res.status === 403 && data.code === 'email_not_verified') {
          setUnverified(true)
        } else {
          setError(data.error ?? 'Something went wrong. Please try again.')
        }
        return
      }
      window.location.href = '/dashboard/'
    } catch {
      setError('Network error — please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div class="auth-card">
      <div class="auth-card-header">
        <a href="/" class="auth-brand">Quipthread</a>
      </div>
      <div class="auth-card-body">
        <h1 class="auth-title">Sign in</h1>
        <p class="auth-subtitle">Welcome back to your dashboard.</p>

        <div class="oauth-buttons">
          <a href={`${API}/auth/github/login`} class="btn oauth-btn">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.167 6.839 9.49.5.092.682-.217.682-.482 0-.237-.009-.868-.013-1.703-2.782.604-3.369-1.342-3.369-1.342-.454-1.154-1.11-1.462-1.11-1.462-.908-.62.069-.608.069-.608 1.003.071 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836a9.59 9.59 0 012.504.337c1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.163 22 16.418 22 12c0-5.523-4.477-10-10-10z" />
            </svg>
            Continue with GitHub
          </a>
          <a href={`${API}/auth/google/login`} class="btn oauth-btn">
            <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
              <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
              <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
              <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z"/>
              <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
            </svg>
            Continue with Google
          </a>
        </div>

        <div class="auth-divider"><span>or</span></div>

        <form onSubmit={handleSubmit}>
          {unverified && (
            <div class="error-msg" style={{ marginBottom: '1rem' }}>
              {resendSent
                ? 'Verification email sent — check your inbox.'
                : 'Your email isn\'t verified yet.'}
              {!resendSent && (
                <span>
                  {' '}
                  <button
                    type="button"
                    onClick={handleResend}
                    disabled={resending}
                    style={{ background: 'none', border: 'none', padding: 0, color: 'inherit', textDecoration: 'underline', cursor: 'pointer', font: 'inherit' }}
                  >
                    {resending ? 'Sending…' : 'Resend verification email'}
                  </button>
                </span>
              )}
            </div>
          )}
          {error && !unverified && (
            <div class="error-msg" style={{ marginBottom: '1rem' }}>{error}</div>
          )}

          <div class="field">
            <label for="login-email">Email</label>
            <input
              id="login-email"
              type="email"
              placeholder="you@example.com"
              value={email}
              onInput={e => setEmail(e.currentTarget.value)}
              required
              disabled={loading}
            />
          </div>

          <div class="field">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: '0.375rem' }}>
              <label for="login-password" style={{ margin: 0 }}>Password</label>
              <a href="/forgot-password" class="forgot-link">Forgot password?</a>
            </div>
            <input
              id="login-password"
              type="password"
              placeholder="Your password"
              value={password}
              onInput={e => setPassword(e.currentTarget.value)}
              required
              disabled={loading}
            />
          </div>

          <button
            type="submit"
            class="btn btn-primary"
            disabled={loading || !email || !password}
            style={{ width: '100%', justifyContent: 'center', padding: '0.625rem 1rem', fontSize: '0.9375rem', marginTop: '0.25rem' }}
          >
            {loading ? 'Signing in…' : 'Sign in'}
          </button>
        </form>

        <p class="auth-footer">
          Don't have an account? <a href="/signup">Sign up</a>
        </p>
      </div>
    </div>
  )
}
