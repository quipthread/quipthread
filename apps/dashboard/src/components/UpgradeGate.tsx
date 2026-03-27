import type { Plan } from '../lib/plan'
import { PLAN_LABELS, PLAN_PRICES } from '../lib/plan'

interface Props {
  feature: string
  description: string
  minPlan: Plan
}

export default function UpgradeGate({ feature, description, minPlan }: Props) {
  return (
    <div style={{ maxWidth: 520, margin: '4rem auto', textAlign: 'center', padding: '0 1rem' }}>
      <div
        style={{
          width: 52,
          height: 52,
          borderRadius: '50%',
          background: 'var(--surface)',
          border: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          margin: '0 auto 1.25rem',
        }}
      >
        <svg
          width="22"
          height="22"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          style={{ color: 'var(--muted)' }}
        >
          <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
          <path d="M7 11V7a5 5 0 0 1 10 0v4" />
        </svg>
      </div>

      <h2
        style={{
          margin: '0 0 0.5rem',
          fontFamily: 'var(--f-display)',
          fontSize: '1.25rem',
          fontWeight: 600,
          color: 'var(--text)',
        }}
      >
        {feature}
      </h2>

      <p
        style={{
          margin: '0 0 1.25rem',
          fontSize: '0.9375rem',
          color: 'var(--muted)',
          lineHeight: 1.6,
        }}
      >
        {description}
      </p>

      <div
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.375rem',
          padding: '0.25rem 0.75rem',
          borderRadius: 9999,
          background: 'var(--amber-bg)',
          border: '1px solid var(--amber-border)',
          fontSize: '0.8125rem',
          fontWeight: 600,
          color: 'var(--amber)',
          marginBottom: '1.5rem',
        }}
      >
        {PLAN_LABELS[minPlan]} plan — {PLAN_PRICES[minPlan]}
      </div>

      <div style={{ display: 'flex', gap: '0.625rem', justifyContent: 'center' }}>
        <a
          href="/dashboard/billing"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            padding: '0.5rem 1.125rem',
            borderRadius: 6,
            background: 'var(--ink)',
            color: 'var(--paper)',
            textDecoration: 'none',
            fontSize: '0.9375rem',
            fontWeight: 500,
            transition: 'background 0.1s',
          }}
        >
          View plans
        </a>
      </div>
    </div>
  )
}
