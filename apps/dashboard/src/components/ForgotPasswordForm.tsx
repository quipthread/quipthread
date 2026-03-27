import { useState } from 'preact/hooks'
import { API } from '../api'

export default function ForgotPasswordForm() {
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [sent, setSent] = useState(false)

  const handleSubmit = async (e: SubmitEvent) => {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      const res = await fetch(`${API}/auth/email/forgot`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ email }),
      })
      if (!res.ok) {
        const data = (await res.json().catch(() => ({}))) as { error?: string }
        setError(data.error ?? 'Something went wrong. Please try again.')
        return
      }
      setSent(true)
    } catch {
      setError('Network error — please try again.')
    } finally {
      setLoading(false)
    }
  }

  if (sent) {
    return (
      <div class="auth-card">
        <div class="auth-card-header">
          <a href="/" class="auth-brand">
            Quipthread
          </a>
        </div>
        <div class="auth-card-body">
          <h1 class="auth-title">Check your email</h1>
          <p class="auth-subtitle">
            If <strong>{email}</strong> is registered, you'll receive a reset link shortly.
          </p>
          <p class="auth-footer">
            <a href="/login">Back to sign in</a>
          </p>
        </div>
      </div>
    )
  }

  return (
    <div class="auth-card">
      <div class="auth-card-header">
        <a href="/" class="auth-brand">
          Quipthread
        </a>
      </div>
      <div class="auth-card-body">
        <h1 class="auth-title">Reset your password</h1>
        <p class="auth-subtitle">Enter your email and we'll send you a reset link.</p>

        <form onSubmit={handleSubmit}>
          {error && (
            <div class="error-msg" style={{ marginBottom: '1rem' }}>
              {error}
            </div>
          )}

          <div class="field">
            <label for="forgot-email">Email</label>
            <input
              id="forgot-email"
              type="email"
              placeholder="you@example.com"
              value={email}
              onInput={(e) => setEmail(e.currentTarget.value)}
              required
              disabled={loading}
            />
          </div>

          <button
            type="submit"
            class="btn btn-primary"
            disabled={loading || !email}
            style={{
              width: '100%',
              justifyContent: 'center',
              padding: '0.625rem 1rem',
              fontSize: '0.9375rem',
              marginTop: '0.25rem',
            }}
          >
            {loading ? 'Sending…' : 'Send reset link'}
          </button>
        </form>

        <p class="auth-footer">
          <a href="/login">Back to sign in</a>
        </p>
      </div>
    </div>
  )
}
