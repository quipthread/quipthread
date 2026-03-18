import { useState, useEffect, useCallback } from 'preact/hooks'
import { api, buildExportURL } from '../api'
import { relativeTime } from '../utils'
import type { Site } from '../types'

const THEME_OPTIONS = [
  {
    group: 'Default',
    options: [
      { value: 'auto', label: 'Auto' },
      { value: 'light', label: 'Light' },
      { value: 'dark', label: 'Dark' },
    ],
  },
  {
    group: 'Catppuccin',
    options: [
      { value: 'catppuccin-latte', label: 'Latte' },
      { value: 'catppuccin-frappe', label: 'Frappé' },
      { value: 'catppuccin-macchiato', label: 'Macchiato' },
      { value: 'catppuccin-mocha', label: 'Mocha' },
    ],
  },
  {
    group: 'Other',
    options: [
      { value: 'dracula', label: 'Dracula' },
      { value: 'nord', label: 'Nord' },
      { value: 'gruvbox-light', label: 'Gruvbox Light' },
      { value: 'gruvbox-dark', label: 'Gruvbox Dark' },
      { value: 'tokyo-night', label: 'Tokyo Night' },
      { value: 'rose-pine', label: 'Rosé Pine' },
      { value: 'rose-pine-dawn', label: 'Rosé Pine Dawn' },
      { value: 'solarized-light', label: 'Solarized Light' },
      { value: 'solarized-dark', label: 'Solarized Dark' },
      { value: 'one-dark', label: 'One Dark' },
    ],
  },
]

const THEME_BG: Record<string, string> = {
  'auto':                '#888888',
  'light':               '#f7f4ef',
  'dark':                '#0f0f0f',
  'catppuccin-latte':    '#eff1f5',
  'catppuccin-frappe':   '#303446',
  'catppuccin-macchiato':'#24273a',
  'catppuccin-mocha':    '#1e1e2e',
  'dracula':             '#282a36',
  'nord':                '#2e3440',
  'gruvbox-light':       '#fbf1c7',
  'gruvbox-dark':        '#282828',
  'tokyo-night':         '#1a1b26',
  'rose-pine':           '#191724',
  'rose-pine-dawn':      '#faf4ed',
  'solarized-light':     '#fdf6e3',
  'solarized-dark':      '#002b36',
  'one-dark':            '#282c34',
}

const THEME_ACCENT: Record<string, string> = {
  'auto':                '#e07f32',
  'light':               '#c06020',
  'dark':                '#e07f32',
  'catppuccin-latte':    '#1e66f5',
  'catppuccin-frappe':   '#8caaee',
  'catppuccin-macchiato':'#8aadf4',
  'catppuccin-mocha':    '#89b4fa',
  'dracula':             '#bd93f9',
  'nord':                '#88c0d0',
  'gruvbox-light':       '#d65d0e',
  'gruvbox-dark':        '#d79921',
  'tokyo-night':         '#7aa2f7',
  'rose-pine':           '#c4a7e7',
  'rose-pine-dawn':      '#907aa9',
  'solarized-light':     '#268bd2',
  'solarized-dark':      '#268bd2',
  'one-dark':            '#61afef',
}

type ExportState = {
  format: string
  status: string
  from: string
  to: string
}

const defaultExport = (): ExportState => ({ format: 'native', status: 'approved', from: '', to: '' })

export default function SitesPanel() {
  const [sites, setSites] = useState<Site[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [domain, setDomain] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [savingId, setSavingId] = useState<string | null>(null)
  const [savedId, setSavedId] = useState<string | null>(null)
  const [exportOpenId, setExportOpenId] = useState<string | null>(null)
  const [exportState, setExportState] = useState<ExportState>(defaultExport())

  const fetchSites = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await api.sites.list()
      setSites(res.sites ?? [])
    } catch {
      setError('Failed to load sites.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchSites() }, [fetchSites])

  const createSite = async (e: SubmitEvent) => {
    e.preventDefault()
    const trimmed = domain.trim()
    if (!trimmed) return
    setCreating(true)
    setCreateError(null)
    try {
      const site = await api.sites.create(trimmed) as Site
      setSites(prev => [site, ...prev])
      setDomain('')
      setAdding(false)
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create site.')
    } finally {
      setCreating(false)
    }
  }

  const updateTheme = async (site: Site, theme: string) => {
    setSites(prev => prev.map(s => s.id === site.id ? { ...s, theme } : s))
    setSavingId(site.id)
    setSavedId(null)
    try {
      await api.sites.update(site.id, { theme })
      setSavedId(site.id)
      setTimeout(() => setSavedId(id => id === site.id ? null : id), 2000)
    } catch {
      // Revert on failure
      setSites(prev => prev.map(s => s.id === site.id ? { ...s, theme: site.theme } : s))
    } finally {
      setSavingId(null)
    }
  }

  return (
    <>
      <div className="page-header">
        <h1>Sites</h1>
        <button
          className={adding ? 'btn' : 'btn btn-primary'}
          onClick={() => { setAdding(v => !v); setCreateError(null) }}
        >
          {adding ? 'Cancel' : '+ Add Site'}
        </button>
      </div>

      {adding && (
        <form
          onSubmit={createSite}
          style={{
            marginBottom: '1.5rem',
            padding: '1.125rem 1.25rem',
            background: 'white',
            border: '1px solid var(--border)',
            borderRadius: 8,
            display: 'flex',
            gap: '0.625rem',
            alignItems: 'flex-start',
          }}
        >
          <div style={{ flex: 1 }}>
            <input
              type="text"
              placeholder="example.com"
              value={domain}
              onChange={e => setDomain(e.target.value)}
              disabled={creating}
              style={{ width: '100%' }}
              autoFocus
            />
            {createError && (
              <div className="error-msg" style={{ marginTop: '0.5rem' }}>
                {createError}
              </div>
            )}
          </div>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={creating || !domain.trim()}
          >
            {creating ? 'Creating…' : 'Create'}
          </button>
        </form>
      )}

      {loading ? (
        <div className="loading">Loading…</div>
      ) : error ? (
        <div className="error-msg">{error}</div>
      ) : sites.length === 0 ? (
        <div className="empty">No sites yet. Add one above.</div>
      ) : (
        <div className="table-card">
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Domain</th>
                <th>Theme</th>
                <th>Created</th>
                <th>Export</th>
              </tr>
            </thead>
            <tbody>
              {sites.map(s => {
                const theme = s.theme || 'auto'
                const bg = THEME_BG[theme] ?? '#888'
                const accent = THEME_ACCENT[theme] ?? '#888'
                const isSaving = savingId === s.id
                const isSaved = savedId === s.id
                const isExportOpen = exportOpenId === s.id
                const plan = typeof document !== 'undefined'
                  ? (document.documentElement.dataset.plan ?? 'hobby')
                  : 'hobby'
                const csvDisabled = plan === 'hobby'

                const toggleExport = () => {
                  if (isExportOpen) {
                    setExportOpenId(null)
                  } else {
                    setExportOpenId(s.id)
                    setExportState(defaultExport())
                  }
                }

                return (
                  <>
                    <tr key={s.id}>
                      <td>
                        <code style={{
                          fontSize: '0.75rem',
                          color: 'var(--muted)',
                          fontFamily: 'var(--f-mono)',
                          background: 'var(--surface)',
                          padding: '0.1875rem 0.4375rem',
                          borderRadius: 4,
                        }}>
                          {s.id}
                        </code>
                      </td>
                      <td style={{ fontWeight: 500 }}>{s.domain}</td>
                      <td>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                          <span
                            title={theme}
                            style={{
                              display: 'inline-block',
                              width: 22,
                              height: 22,
                              borderRadius: '50%',
                              background: `linear-gradient(135deg, ${bg} 40%, ${accent})`,
                              border: '2px solid var(--border)',
                              flexShrink: 0,
                            }}
                          />
                          <select
                            value={theme}
                            disabled={isSaving}
                            onChange={e => updateTheme(s, e.target.value)}
                            style={{
                              fontSize: '0.8125rem',
                              padding: '0.25rem 0.375rem',
                              borderRadius: 4,
                              border: '1px solid var(--border)',
                              background: 'var(--surface)',
                              color: 'var(--text)',
                              cursor: 'pointer',
                              opacity: isSaving ? 0.6 : 1,
                            }}
                          >
                            {THEME_OPTIONS.map(group => (
                              <optgroup key={group.group} label={group.group}>
                                {group.options.map(opt => (
                                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                                ))}
                              </optgroup>
                            ))}
                          </select>
                          {isSaving && (
                            <span style={{ fontSize: '0.75rem', color: 'var(--muted)' }}>Saving…</span>
                          )}
                          {isSaved && !isSaving && (
                            <span style={{ fontSize: '0.75rem', color: 'var(--success, #22c55e)' }}>Saved</span>
                          )}
                        </div>
                      </td>
                      <td style={{ whiteSpace: 'nowrap', color: 'var(--muted)', fontSize: '0.8125rem' }}>
                        {relativeTime(s.created_at)}
                      </td>
                      <td>
                        <button
                          className="btn"
                          style={{ fontSize: '0.8125rem', padding: '0.25rem 0.625rem' }}
                          onClick={toggleExport}
                        >
                          {isExportOpen ? 'Close' : 'Export'}
                        </button>
                      </td>
                    </tr>
                    <tr style={{ display: isExportOpen ? 'table-row' : 'none' }}>
                      <td colSpan={5} style={{ padding: '0.875rem 1rem', background: 'var(--surface)', borderTop: 'none' }}>
                        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.75rem', alignItems: 'flex-end' }}>
                          <div>
                            <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--muted)', marginBottom: '0.25rem' }}>Format</label>
                            <select
                              value={exportState.format}
                              onChange={e => setExportState(prev => ({ ...prev, format: e.target.value }))}
                              style={{ fontSize: '0.8125rem', padding: '0.25rem 0.375rem', borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)' }}
                            >
                              <option value="native">Quipthread JSON</option>
                              <option value="csv" disabled={csvDisabled}>
                                {csvDisabled ? 'CSV (Starter+)' : 'CSV'}
                              </option>
                            </select>
                          </div>
                          <div>
                            <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--muted)', marginBottom: '0.25rem' }}>Status</label>
                            <select
                              value={exportState.status}
                              onChange={e => setExportState(prev => ({ ...prev, status: e.target.value }))}
                              style={{ fontSize: '0.8125rem', padding: '0.25rem 0.375rem', borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)' }}
                            >
                              <option value="approved">Approved only</option>
                              <option value="all">All statuses</option>
                            </select>
                          </div>
                          <div>
                            <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--muted)', marginBottom: '0.25rem' }}>From</label>
                            <input
                              type="date"
                              value={exportState.from}
                              onChange={e => setExportState(prev => ({ ...prev, from: e.target.value }))}
                              style={{ fontSize: '0.8125rem', padding: '0.25rem 0.375rem', borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)' }}
                            />
                          </div>
                          <div>
                            <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--muted)', marginBottom: '0.25rem' }}>To</label>
                            <input
                              type="date"
                              value={exportState.to}
                              onChange={e => setExportState(prev => ({ ...prev, to: e.target.value }))}
                              style={{ fontSize: '0.8125rem', padding: '0.25rem 0.375rem', borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)' }}
                            />
                          </div>
                          <a
                            href={buildExportURL(s.id, exportState.format, {
                              status: exportState.status,
                              from: exportState.from ? exportState.from + 'T00:00:00Z' : undefined,
                              to: exportState.to ? exportState.to + 'T23:59:59Z' : undefined,
                            })}
                            download
                            className="btn btn-primary"
                            style={{ fontSize: '0.8125rem', padding: '0.25rem 0.875rem', textDecoration: 'none', display: 'inline-block' }}
                          >
                            Download
                          </a>
                        </div>
                      </td>
                    </tr>
                  </>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}
