import { useCallback, useEffect, useState } from 'preact/hooks'
import { api, buildExportURL } from '../api'
import type { Site } from '../types'
import { relativeTime } from '../utils'
import PageHeader from './shared/PageHeader'

const THEME_LABEL: Record<string, string> = {
  auto: 'Auto',
  light: 'Light',
  dark: 'Dark',
  'catppuccin-latte': 'Latte',
  'catppuccin-frappe': 'Frappé',
  'catppuccin-macchiato': 'Macchiato',
  'catppuccin-mocha': 'Mocha',
  dracula: 'Dracula',
  nord: 'Nord',
  'gruvbox-light': 'Gruvbox Light',
  'gruvbox-dark': 'Gruvbox Dark',
  'tokyo-night': 'Tokyo Night',
  'rose-pine': 'Rosé Pine',
  'rose-pine-dawn': 'Rosé Pine Dawn',
  'solarized-light': 'Solarized Light',
  'solarized-dark': 'Solarized Dark',
  'one-dark': 'One Dark',
}

const THEME_BG: Record<string, string> = {
  auto: '#888888',
  light: '#f7f4ef',
  dark: '#0f0f0f',
  'catppuccin-latte': '#eff1f5',
  'catppuccin-frappe': '#303446',
  'catppuccin-macchiato': '#24273a',
  'catppuccin-mocha': '#1e1e2e',
  dracula: '#282a36',
  nord: '#2e3440',
  'gruvbox-light': '#fbf1c7',
  'gruvbox-dark': '#282828',
  'tokyo-night': '#1a1b26',
  'rose-pine': '#191724',
  'rose-pine-dawn': '#faf4ed',
  'solarized-light': '#fdf6e3',
  'solarized-dark': '#002b36',
  'one-dark': '#282c34',
}

const THEME_ACCENT: Record<string, string> = {
  auto: '#e07f32',
  light: '#c06020',
  dark: '#e07f32',
  'catppuccin-latte': '#1e66f5',
  'catppuccin-frappe': '#8caaee',
  'catppuccin-macchiato': '#8aadf4',
  'catppuccin-mocha': '#89b4fa',
  dracula: '#bd93f9',
  nord: '#88c0d0',
  'gruvbox-light': '#d65d0e',
  'gruvbox-dark': '#d79921',
  'tokyo-night': '#7aa2f7',
  'rose-pine': '#c4a7e7',
  'rose-pine-dawn': '#907aa9',
  'solarized-light': '#268bd2',
  'solarized-dark': '#268bd2',
  'one-dark': '#61afef',
}

type ExportState = {
  format: string
  status: string
  from: string
  to: string
}

const defaultExport = (): ExportState => ({
  format: 'native',
  status: 'approved',
  from: '',
  to: '',
})

const NOTIFY_OPTIONS = [
  { label: '1 min', value: 60 },
  { label: '5 min', value: 300 },
  { label: '15 min', value: 900 },
  { label: '30 min', value: 1800 },
  { label: '1 hour', value: 3600 },
]

type OpenAccordion = { siteId: string; type: 'export' | 'notify' } | null

export default function SitesPanel() {
  const [sites, setSites] = useState<Site[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [domain, setDomain] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [openAccordion, setOpenAccordion] = useState<OpenAccordion>(null)
  const [exportState, setExportState] = useState<ExportState>(defaultExport())
  const [savingNotifyId, setSavingNotifyId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  const handleDelete = async (site: Site) => {
    if (!confirm(`Delete site "${site.domain}"? This cannot be undone.`)) return
    setDeletingId(site.id)
    try {
      await api.sites.delete(site.id)
      setSites((prev) => prev.filter((s) => s.id !== site.id))
    } catch {
      alert('Failed to delete site.')
    } finally {
      setDeletingId(null)
    }
  }

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

  useEffect(() => {
    fetchSites()
  }, [fetchSites])

  const createSite = async (e?: Event) => {
    e?.preventDefault()
    const trimmed = domain.trim()
    if (!trimmed) return
    setCreating(true)
    setCreateError(null)
    try {
      const site = (await api.sites.create(trimmed)) as Site
      setSites((prev) => [site, ...prev])
      setDomain('')
      setAdding(false)
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create site.')
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <PageHeader
        title="Sites"
        action={
          <button
            type="button"
            className={adding ? 'btn' : 'btn btn-primary'}
            onClick={() => {
              setAdding((v) => !v)
              setCreateError(null)
            }}
          >
            {adding ? 'Cancel' : '+ Add Site'}
          </button>
        }
      />

      {adding && (
        <form
          onSubmit={createSite}
          style={{
            marginBottom: '1.5rem',
            padding: '1.125rem 1.25rem',
            background: 'var(--card-bg)',
            border: '1px solid var(--border)',
            borderRadius: 8,
            display: 'flex',
            gap: '0.625rem',
            alignItems: 'center',
          }}
        >
          <div style={{ flex: 1 }}>
            <input
              type="text"
              placeholder="example.com"
              value={domain}
              onChange={(e) => setDomain((e.target as HTMLInputElement).value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  createSite()
                }
              }}
              disabled={creating}
              style={{ width: '100%' }}
            />
            {createError && (
              <div className="error-msg" style={{ marginTop: '0.5rem' }}>
                {createError}
              </div>
            )}
          </div>
          <button type="submit" className="btn btn-primary" disabled={creating || !domain.trim()}>
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
                <th>Notifications</th>
                <th></th>
              </tr>
            </thead>
            {sites.map((s) => {
              const theme = s.theme || 'auto'
              const bg = THEME_BG[theme] ?? '#888'
              const accent = THEME_ACCENT[theme] ?? '#888'
              const plan =
                typeof document !== 'undefined'
                  ? (document.documentElement.dataset.plan ?? 'hobby')
                  : 'hobby'
              const csvDisabled = plan === 'hobby'

              const isExportOpen = openAccordion?.siteId === s.id && openAccordion.type === 'export'
              const isNotifyOpen = openAccordion?.siteId === s.id && openAccordion.type === 'notify'

              const toggleExport = () => {
                if (!isExportOpen) setExportState(defaultExport())
                setOpenAccordion(isExportOpen ? null : { siteId: s.id, type: 'export' })
              }
              const toggleNotify = () =>
                setOpenAccordion(isNotifyOpen ? null : { siteId: s.id, type: 'notify' })

              const currentInterval = s.notify_interval ?? 300
              const notifyDisabled = plan !== 'pro' && plan !== 'business' && plan !== 'enterprise'

              const saveInterval = async (value: number) => {
                setSavingNotifyId(s.id)
                try {
                  const updated = (await api.sites.update(s.id, { notify_interval: value })) as Site
                  setSites((prev) =>
                    prev.map((x) =>
                      x.id === s.id ? { ...x, notify_interval: updated.notify_interval } : x,
                    ),
                  )
                } catch {
                  // leave existing value on error
                } finally {
                  setSavingNotifyId(null)
                }
              }

              return (
                <tbody key={s.id}>
                  <tr>
                    <td data-label="ID">
                      <code
                        style={{
                          fontSize: '0.75rem',
                          color: 'var(--muted)',
                          fontFamily: 'var(--f-mono)',
                          background: 'var(--surface)',
                          padding: '0.1875rem 0.4375rem',
                          borderRadius: 4,
                        }}
                      >
                        {s.id}
                      </code>
                    </td>
                    <td data-label="Domain" style={{ fontWeight: 500 }}>
                      {s.domain}
                    </td>
                    <td data-label="Theme">
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '0.5rem',
                          flexWrap: 'wrap' as const,
                        }}
                      >
                        <span
                          title={theme}
                          style={{
                            display: 'inline-block',
                            width: 18,
                            height: 18,
                            borderRadius: '50%',
                            background: `linear-gradient(135deg, ${bg} 40%, ${accent})`,
                            border: '2px solid var(--border)',
                            flexShrink: 0,
                          }}
                        />
                        <span style={{ fontSize: '0.8125rem', color: 'var(--text)' }}>
                          {THEME_LABEL[theme] ?? theme}
                        </span>
                        <a
                          href="/dashboard/preview"
                          className="btn"
                          style={{
                            fontSize: '0.75rem',
                            padding: '0.1875rem 0.5rem',
                            textDecoration: 'none',
                          }}
                        >
                          Configure
                        </a>
                      </div>
                    </td>
                    <td
                      data-label="Created"
                      style={{ color: 'var(--muted)', fontSize: '0.8125rem' }}
                    >
                      {relativeTime(s.created_at)}
                    </td>
                    <td data-label="Export">
                      <button
                        type="button"
                        className="btn"
                        style={{ fontSize: '0.8125rem', padding: '0.25rem 0.625rem' }}
                        onClick={toggleExport}
                      >
                        {isExportOpen ? 'Close' : 'Export'}
                      </button>
                    </td>
                    <td data-label="Notifications">
                      <button
                        type="button"
                        className="btn"
                        style={{ fontSize: '0.8125rem', padding: '0.25rem 0.625rem' }}
                        onClick={toggleNotify}
                      >
                        {isNotifyOpen ? 'Close' : 'Notify'}
                      </button>
                    </td>
                    <td data-label="Delete">
                      <button
                        type="button"
                        className="btn btn-reject"
                        style={{ fontSize: '0.8125rem', padding: '0.25rem 0.625rem' }}
                        disabled={deletingId === s.id}
                        onClick={() => handleDelete(s)}
                      >
                        {deletingId === s.id ? 'Deleting…' : 'Delete'}
                      </button>
                    </td>
                  </tr>
                  <tr
                    className="table-accordion-row"
                    style={{ display: isExportOpen ? undefined : 'none' }}
                  >
                    <td
                      colSpan={7}
                      style={{
                        padding: '0.875rem 1rem',
                        background: 'var(--surface)',
                        borderTop: 'none',
                      }}
                    >
                      <div
                        style={{
                          display: 'flex',
                          flexWrap: 'wrap',
                          gap: '0.75rem',
                          alignItems: 'flex-end',
                        }}
                      >
                        <div>
                          <label
                            htmlFor={`export-format-${s.id}`}
                            style={{
                              display: 'block',
                              fontSize: '0.75rem',
                              color: 'var(--muted)',
                              marginBottom: '0.25rem',
                            }}
                          >
                            Format
                          </label>
                          <select
                            id={`export-format-${s.id}`}
                            value={exportState.format}
                            onChange={(e) =>
                              setExportState((prev) => ({
                                ...prev,
                                format: (e.target as HTMLSelectElement).value,
                              }))
                            }
                            style={{
                              fontSize: '0.8125rem',
                              padding: '0.25rem 0.375rem',
                              borderRadius: 4,
                              border: '1px solid var(--border)',
                              background: 'var(--bg)',
                              color: 'var(--text)',
                            }}
                          >
                            <option value="native">Quipthread JSON</option>
                            <option value="csv" disabled={csvDisabled}>
                              {csvDisabled ? 'CSV (Starter+)' : 'CSV'}
                            </option>
                          </select>
                        </div>
                        <div>
                          <label
                            htmlFor={`export-status-${s.id}`}
                            style={{
                              display: 'block',
                              fontSize: '0.75rem',
                              color: 'var(--muted)',
                              marginBottom: '0.25rem',
                            }}
                          >
                            Status
                          </label>
                          <select
                            id={`export-status-${s.id}`}
                            value={exportState.status}
                            onChange={(e) =>
                              setExportState((prev) => ({
                                ...prev,
                                status: (e.target as HTMLSelectElement).value,
                              }))
                            }
                            style={{
                              fontSize: '0.8125rem',
                              padding: '0.25rem 0.375rem',
                              borderRadius: 4,
                              border: '1px solid var(--border)',
                              background: 'var(--bg)',
                              color: 'var(--text)',
                            }}
                          >
                            <option value="approved">Approved only</option>
                            <option value="all">All statuses</option>
                          </select>
                        </div>
                        <div>
                          <label
                            htmlFor={`export-from-${s.id}`}
                            style={{
                              display: 'block',
                              fontSize: '0.75rem',
                              color: 'var(--muted)',
                              marginBottom: '0.25rem',
                            }}
                          >
                            From
                          </label>
                          <input
                            id={`export-from-${s.id}`}
                            type="date"
                            value={exportState.from}
                            onChange={(e) =>
                              setExportState((prev) => ({
                                ...prev,
                                from: (e.target as HTMLInputElement).value,
                              }))
                            }
                            style={{
                              fontSize: '0.8125rem',
                              padding: '0.25rem 0.375rem',
                              borderRadius: 4,
                              border: '1px solid var(--border)',
                              background: 'var(--bg)',
                              color: 'var(--text)',
                            }}
                          />
                        </div>
                        <div>
                          <label
                            htmlFor={`export-to-${s.id}`}
                            style={{
                              display: 'block',
                              fontSize: '0.75rem',
                              color: 'var(--muted)',
                              marginBottom: '0.25rem',
                            }}
                          >
                            To
                          </label>
                          <input
                            id={`export-to-${s.id}`}
                            type="date"
                            value={exportState.to}
                            onChange={(e) =>
                              setExportState((prev) => ({
                                ...prev,
                                to: (e.target as HTMLInputElement).value,
                              }))
                            }
                            style={{
                              fontSize: '0.8125rem',
                              padding: '0.25rem 0.375rem',
                              borderRadius: 4,
                              border: '1px solid var(--border)',
                              background: 'var(--bg)',
                              color: 'var(--text)',
                            }}
                          />
                        </div>
                        <a
                          href={buildExportURL(s.id, exportState.format, {
                            status: exportState.status,
                            from: exportState.from ? `${exportState.from}T00:00:00Z` : undefined,
                            to: exportState.to ? `${exportState.to}T23:59:59Z` : undefined,
                          })}
                          download
                          className="btn btn-primary"
                          style={{
                            fontSize: '0.8125rem',
                            padding: '0.25rem 0.875rem',
                            textDecoration: 'none',
                            display: 'inline-block',
                          }}
                        >
                          Download
                        </a>
                      </div>
                    </td>
                  </tr>
                  <tr
                    className="table-accordion-row"
                    style={{ display: isNotifyOpen ? undefined : 'none' }}
                  >
                    <td
                      colSpan={7}
                      style={{
                        padding: '0.875rem 1rem',
                        background: 'var(--surface)',
                        borderTop: 'none',
                      }}
                    >
                      {notifyDisabled ? (
                        <p style={{ margin: 0, fontSize: '0.8125rem', color: 'var(--muted)' }}>
                          Configurable notification intervals are available on <strong>Pro+</strong>
                          . All sites notify every 5 minutes by default.
                        </p>
                      ) : (
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                          <div>
                            <label
                              htmlFor={`notify-interval-${s.id}`}
                              style={{
                                display: 'block',
                                fontSize: '0.75rem',
                                color: 'var(--muted)',
                                marginBottom: '0.25rem',
                              }}
                            >
                              Dispatch interval
                            </label>
                            <select
                              id={`notify-interval-${s.id}`}
                              value={currentInterval}
                              disabled={savingNotifyId === s.id}
                              onChange={(e) =>
                                saveInterval(Number((e.target as HTMLSelectElement).value))
                              }
                              style={{
                                fontSize: '0.8125rem',
                                padding: '0.25rem 0.375rem',
                                borderRadius: 4,
                                border: '1px solid var(--border)',
                                background: 'var(--bg)',
                                color: 'var(--text)',
                              }}
                            >
                              {NOTIFY_OPTIONS.map((o) => (
                                <option key={o.value} value={o.value}>
                                  {o.label}
                                </option>
                              ))}
                            </select>
                          </div>
                          {savingNotifyId === s.id && (
                            <span style={{ fontSize: '0.8125rem', color: 'var(--muted)' }}>
                              Saving…
                            </span>
                          )}
                        </div>
                      )}
                    </td>
                  </tr>
                </tbody>
              )
            })}
          </table>
        </div>
      )}
    </>
  )
}
