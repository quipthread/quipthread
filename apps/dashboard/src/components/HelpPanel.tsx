import { useEffect, useState } from 'preact/hooks'
import { api } from '../api'
import { IS_SELF_HOSTED } from '../lib/env'
import type { BillingStatus } from '../types'

const FAQ_ITEMS = [
  {
    q: 'How does comment moderation work?',
    a: 'By default, comments from first-time commenters are held for approval. Once a commenter is approved once, their future comments are auto-approved. You can change this behavior per-site in the Sites tab.',
    gate: null,
  },
  {
    q: 'How do I embed Quipthread on my site?',
    a: "Go to the Preview page — select your site, copy the snippet for your framework, and paste it into your page template. The Preview page also shows installation detection once you've deployed.",
    gate: null,
  },
  {
    q: 'What counts toward my monthly comment limit?',
    a: 'Every new comment submitted counts, including ones that are pending moderation or later rejected. Deleted comments are not subtracted from your count.',
    gate: 'billing' as const,
  },
  {
    q: 'How do I upgrade my plan?',
    a: 'Go to the Billing tab to view available plans and start an upgrade. Changes take effect immediately after checkout completes.',
    gate: 'billing' as const,
  },
  {
    q: 'Can I import comments from another platform?',
    a: 'Yes — the Import tab supports Disqus, WordPress, Remark42, and native Quipthread exports. Imports do not count toward your monthly comment limit.',
    gate: null,
  },
  {
    q: 'How do I reset my password?',
    a: "On the login page, click 'Forgot password?' to receive a reset link by email.",
    gate: null,
  },
  {
    q: 'How do I get support?',
    a: 'Open an issue on GitHub or reach out via the contact on our website.',
    gate: null,
  },
]

export default function HelpPanel() {
  const [billing, setBilling] = useState<BillingStatus | null>(null)
  const [openFaq, setOpenFaq] = useState<boolean[]>([])

  useEffect(() => {
    if (!IS_SELF_HOSTED) {
      api.billing
        .status()
        .then((status) => {
          setBilling(status)
        })
        .catch(() => {})
    }
    setOpenFaq(new Array(FAQ_ITEMS.length).fill(false))
  }, [])

  function toggleFaq(i: number) {
    setOpenFaq((prev) => prev.map((v, idx) => (idx === i ? !v : v)))
  }

  const visibleFaq = FAQ_ITEMS.filter((item) => {
    if (item.gate === 'billing') return !IS_SELF_HOSTED && billing !== null
    return true
  })

  return (
    <div>
      <div class="page-header">
        <h1>Help</h1>
      </div>

      {/* ── Quick Start ──────────────────────────────────── */}
      <section style={{ marginBottom: '2.5rem' }}>
        <h2 style={sectionHeadingStyle}>Add Quipthread to your site</h2>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.875rem',
            padding: '1rem 1.25rem',
            background: 'var(--amber-bg)',
            border: '1px solid var(--amber-border)',
            borderRadius: 8,
          }}
        >
          <span style={{ fontSize: '1.125rem', lineHeight: 1 }}>→</span>
          <span style={{ fontSize: '0.9375rem', color: 'var(--text)' }}>
            Get your embed snippet, pick a theme, and verify installation on the{' '}
            <a
              href="/dashboard/preview"
              style={{ color: 'var(--amber)', fontWeight: 600, textDecoration: 'none' }}
            >
              Preview page
            </a>
            .
          </span>
        </div>
      </section>

      {/* ── FAQ ──────────────────────────────────────────── */}
      <section style={{ marginBottom: '2.5rem' }}>
        <h2 style={sectionHeadingStyle}>Frequently asked questions</h2>

        <div style={{ border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
          {visibleFaq.map((item, i) => {
            const isOpen = openFaq[FAQ_ITEMS.indexOf(item)] ?? false
            return (
              <div
                key={i}
                style={{
                  borderBottom: i < visibleFaq.length - 1 ? '1px solid var(--border)' : 'none',
                }}
              >
                <button
                  type="button"
                  onClick={() => toggleFaq(FAQ_ITEMS.indexOf(item))}
                  style={faqButtonStyle}
                  aria-expanded={isOpen}
                >
                  <span
                    style={{ fontWeight: 500, color: 'var(--text)', textAlign: 'left' as const }}
                  >
                    {item.q}
                  </span>
                  <span
                    aria-hidden="true"
                    style={{
                      flexShrink: 0,
                      marginLeft: '0.75rem',
                      color: 'var(--muted)',
                      fontSize: '1rem',
                      lineHeight: 1,
                      transition: 'transform 230ms cubic-bezier(0.4, 0, 0.2, 1)',
                      transform: isOpen ? 'rotate(45deg)' : 'rotate(0deg)',
                      display: 'inline-block',
                    }}
                  >
                    +
                  </span>
                </button>

                <div
                  style={{
                    display: 'grid',
                    gridTemplateRows: isOpen ? '1fr' : '0fr',
                    transition: 'grid-template-rows 230ms cubic-bezier(0.4, 0, 0.2, 1)',
                  }}
                >
                  <div style={{ overflow: 'hidden' }}>
                    <div style={faqAnswerStyle}>{item.a}</div>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      </section>

      {/* ── Resources ────────────────────────────────────── */}
      <section style={{ marginBottom: '2.5rem' }}>
        <h2 style={sectionHeadingStyle}>Resources</h2>
        <div style={{ display: 'flex', flexDirection: 'column' as const, gap: '0.625rem' }}>
          <ResourceLink href="/docs/" label="Documentation" />
          <ResourceLink href="https://github.com/quipthread/quipthread" label="GitHub" />
          <ResourceLink
            href="https://github.com/quipthread/quipthread/issues"
            label="Report an issue"
          />
        </div>
      </section>
    </div>
  )
}

function ResourceLink({ href, label }: { href: string; label: string }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '0.375rem',
        color: 'var(--amber)',
        textDecoration: 'none',
        fontSize: '0.9375rem',
        fontWeight: 500,
      }}
    >
      {label}
      <svg
        aria-hidden="true"
        width="12"
        height="12"
        viewBox="0 0 12 12"
        fill="none"
        style={{ flexShrink: 0, opacity: 0.7 }}
      >
        <path
          d="M2 2h8M10 2v8M10 2L2 10"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </a>
  )
}

// ── Style objects ────────────────────────────────────────

const sectionHeadingStyle = {
  margin: '0 0 1rem',
  fontFamily: 'var(--f-display)',
  fontSize: '1.0625rem',
  fontWeight: 600,
  letterSpacing: '-0.01em',
  color: 'var(--text)',
}

const faqButtonStyle = {
  width: '100%',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  padding: '0.875rem 1.125rem',
  background: 'var(--card-bg)',
  border: 'none',
  cursor: 'pointer',
  fontFamily: 'var(--f-ui)',
  fontSize: '0.9375rem',
  textAlign: 'left' as const,
  transition: 'background 0.1s',
}

const faqAnswerStyle = {
  padding: '0 1.125rem 0.875rem',
  fontSize: '0.9375rem',
  lineHeight: 1.7,
  color: 'var(--muted)',
  background: 'var(--card-bg)',
}
