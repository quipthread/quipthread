import { useState, useEffect, useRef } from 'preact/hooks'
import { api } from '../api'
import type { BlockedTerm } from '../types'
import UpgradeGate from './UpgradeGate'
import { IS_SELF_HOSTED } from '../lib/env'

const PLAN_ORDER = ['hobby', 'starter', 'pro', 'business']

const BORDER  = 'var(--border)'
const MUTED   = 'var(--muted)'
const TEXT    = 'var(--text)'
const SURFACE = 'var(--surface)'

const POPULAR_LISTS = [
  {
    label: 'LDNOOBW (English profanity)',
    url: 'https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/en',
  },
  {
    label: 'Generic spam keywords',
    url: 'https://raw.githubusercontent.com/splorp/wordpress-comment-blacklist/master/blacklist.txt',
  },
]

export default function ModRulesPanel() {
  const [hasAccess, setHasAccess] = useState<boolean | null>(null)
  const [terms, setTerms] = useState<BlockedTerm[]>([])
  const [loadingList, setLoadingList] = useState(false)
  const [newTerm, setNewTerm] = useState('')
  const [addError, setAddError] = useState<string | null>(null)
  const [addLoading, setAddLoading] = useState(false)
  const [importUrl, setImportUrl] = useState('')
  const [importLoading, setImportLoading] = useState(false)
  const [importResult, setImportResult] = useState<{ added: number; skipped: number } | null>(null)
  const [importError, setImportError] = useState<string | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (IS_SELF_HOSTED) {
      setHasAccess(true)
      fetchTerms()
      return
    }
    api.billing.status()
      .then(status => {
        const hasIt = PLAN_ORDER.indexOf(status.plan) >= PLAN_ORDER.indexOf('pro')
        setHasAccess(hasIt)
        if (hasIt) fetchTerms()
      })
      .catch(() => setHasAccess(false))
  }, [])

  function fetchTerms() {
    setLoadingList(true)
    api.modrules.list()
      .then(({ terms }) => { setTerms(terms); setLoadingList(false) })
      .catch(() => setLoadingList(false))
  }

  async function handleAdd(e: Event) {
    e.preventDefault()
    const t = newTerm.trim().toLowerCase()
    if (!t) return
    setAddLoading(true)
    setAddError(null)
    try {
      const created = await api.modrules.add(t)
      setTerms(prev => [created, ...prev.filter(x => x.id !== created.id)])
      setNewTerm('')
      inputRef.current?.focus()
    } catch {
      setAddError('Failed to add term.')
    } finally {
      setAddLoading(false)
    }
  }

  async function handleDelete(id: string) {
    setDeletingId(id)
    try {
      await api.modrules.delete(id)
      setTerms(prev => prev.filter(t => t.id !== id))
    } catch {
      // silently ignore; keep the term in the list
    } finally {
      setDeletingId(null)
    }
  }

  async function handleImport(e: Event) {
    e.preventDefault()
    const url = importUrl.trim()
    if (!url) return
    setImportLoading(true)
    setImportError(null)
    setImportResult(null)
    try {
      const result = await api.modrules.import(url)
      setImportResult(result)
      setImportUrl('')
      fetchTerms()
    } catch {
      setImportError('Failed to import. Check the URL and try again.')
    } finally {
      setImportLoading(false)
    }
  }

  if (hasAccess === null) {
    return <div className="loading">Loading…</div>
  }

  if (!hasAccess) {
    return (
      <UpgradeGate
        feature="Moderation Rules"
        description="Define keyword blocklists to automatically reject comments containing unwanted words or phrases. Import curated community lists or add your own."
        minPlan="pro"
      />
    )
  }

  return (
    <div>
      <div className="page-header">
        <h1>Moderation Rules</h1>
      </div>

      <p style={{ color: MUTED, fontSize: '0.875rem', marginBottom: '1.5rem', marginTop: '-0.25rem' }}>
        Comments containing any blocked term are automatically rejected. Rules apply globally across all sites.
      </p>

      {/* Add term */}
      <div style={{
        background: 'var(--card-bg)',
        border: `1px solid ${BORDER}`,
        borderRadius: 10,
        padding: '1.25rem 1.5rem',
        marginBottom: '1rem',
      }}>
        <div style={{
          fontSize: '0.75rem', fontWeight: 600, textTransform: 'uppercase' as const,
          letterSpacing: '0.07em', color: MUTED, marginBottom: '0.875rem',
        }}>
          Add blocked term
        </div>
        <form onSubmit={handleAdd} style={{ display: 'flex', gap: '0.625rem' }}>
          <input
            ref={inputRef}
            className="input"
            style={{ margin: 0, flex: 1 }}
            type="text"
            placeholder="e.g. spam, buy now, click here…"
            value={newTerm}
            onInput={e => setNewTerm((e.target as HTMLInputElement).value)}
            disabled={addLoading}
            maxLength={200}
          />
          <button className="btn btn-primary" type="submit" disabled={addLoading || !newTerm.trim()}>
            {addLoading ? 'Adding…' : 'Add'}
          </button>
        </form>
        {addError && (
          <p style={{ color: 'var(--red-text)', fontSize: '0.8125rem', marginTop: '0.5rem' }}>{addError}</p>
        )}
      </div>

      {/* Import from URL */}
      <div style={{
        background: 'var(--card-bg)',
        border: `1px solid ${BORDER}`,
        borderRadius: 10,
        marginBottom: '1.5rem',
        overflow: 'hidden',
      }}>
        <button
          onClick={() => { setImportOpen(o => !o); setImportResult(null); setImportError(null) }}
          style={{
            width: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '1rem 1.5rem',
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            fontFamily: 'var(--f-ui)',
            fontSize: '0.875rem',
            fontWeight: 500,
            color: TEXT,
            textAlign: 'left' as const,
          }}
        >
          <span>Import from URL</span>
          <span style={{
            color: MUTED,
            display: 'inline-flex',
            flexShrink: 0,
            transition: 'transform 230ms cubic-bezier(0.4, 0, 0.2, 1)',
            transform: importOpen ? 'rotate(180deg)' : 'rotate(0deg)',
          }}>
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M2.5 5l4.5 4.5L11.5 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </span>
        </button>

        {importOpen && (
          <div style={{ padding: '0 1.5rem 1.25rem', borderTop: `1px solid ${BORDER}` }}>
            <p style={{ fontSize: '0.8125rem', color: MUTED, margin: '0.875rem 0 0.75rem' }}>
              Provide a URL to a plain-text file with one term per line. Lines starting with <code>#</code> are ignored.
            </p>

            <div style={{ marginBottom: '0.75rem' }}>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: MUTED, marginBottom: '0.375rem' }}>
                Popular lists
              </div>
              <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' as const }}>
                {POPULAR_LISTS.map(l => (
                  <button
                    key={l.url}
                    className="btn"
                    style={{ fontSize: '0.8125rem' }}
                    onClick={() => setImportUrl(l.url)}
                    type="button"
                  >
                    {l.label}
                  </button>
                ))}
              </div>
            </div>

            <form onSubmit={handleImport} style={{ display: 'flex', gap: '0.625rem' }}>
              <input
                className="input"
                style={{ margin: 0, flex: 1, fontFamily: 'var(--f-mono, monospace)', fontSize: '0.8125rem' }}
                type="url"
                placeholder="https://example.com/wordlist.txt"
                value={importUrl}
                onInput={e => setImportUrl((e.target as HTMLInputElement).value)}
                disabled={importLoading}
              />
              <button className="btn btn-primary" type="submit" disabled={importLoading || !importUrl.trim()}>
                {importLoading ? 'Importing…' : 'Import'}
              </button>
            </form>

            {importResult && (
              <p style={{ fontSize: '0.8125rem', color: 'var(--green-text, #2d6a2d)', marginTop: '0.625rem' }}>
                Added {importResult.added} term{importResult.added !== 1 ? 's' : ''}, skipped {importResult.skipped} duplicate{importResult.skipped !== 1 ? 's' : ''}.
              </p>
            )}
            {importError && (
              <p style={{ fontSize: '0.8125rem', color: 'var(--red-text)', marginTop: '0.625rem' }}>{importError}</p>
            )}
          </div>
        )}
      </div>

      {/* Term list */}
      <div style={{
        background: 'var(--card-bg)',
        border: `1px solid ${BORDER}`,
        borderRadius: 10,
        padding: '1.25rem 1.5rem',
      }}>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: '1rem',
        }}>
          <div style={{
            fontSize: '0.75rem', fontWeight: 600, textTransform: 'uppercase' as const,
            letterSpacing: '0.07em', color: MUTED,
          }}>
            Blocked terms
            {terms.length > 0 && (
              <span style={{ marginLeft: '0.5rem', fontWeight: 400 }}>({terms.length})</span>
            )}
          </div>
        </div>

        {loadingList && <div style={{ color: MUTED, fontSize: '0.875rem' }}>Loading…</div>}

        {!loadingList && terms.length === 0 && (
          <div style={{ color: MUTED, fontSize: '0.875rem' }}>
            No blocked terms yet. Add a term above or import a list.
          </div>
        )}

        {!loadingList && terms.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap' as const, gap: '0.5rem' }}>
            {terms.map(t => (
              <span
                key={t.id}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: '0.375rem',
                  background: SURFACE,
                  border: `1px solid ${BORDER}`,
                  borderRadius: 4,
                  padding: '0.25rem 0.5rem 0.25rem 0.625rem',
                  fontSize: '0.8125rem',
                  color: TEXT,
                  fontFamily: 'var(--f-mono, monospace)',
                }}
              >
                {t.term}
                <button
                  onClick={() => handleDelete(t.id)}
                  disabled={deletingId === t.id}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    padding: '0 0.125rem',
                    color: MUTED,
                    fontSize: '0.875rem',
                    lineHeight: 1,
                    opacity: deletingId === t.id ? 0.4 : 1,
                  }}
                  title={`Remove "${t.term}"`}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
