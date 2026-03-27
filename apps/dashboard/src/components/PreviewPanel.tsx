import { useEffect, useRef, useState } from 'preact/hooks'
import { API, api } from '../api'
import type { AnalyticsData, Site } from '../types'
import EmbedCodeGenerator from './EmbedCodeGenerator'
import ThemeSwatches from './ThemeSwatches'

const PLAN_ORDER = ['hobby', 'starter', 'pro', 'business']

function sectionStyle() {
  return {
    background: 'var(--card-bg)',
    border: '1px solid var(--border)',
    borderRadius: 8,
    padding: '1.25rem',
  }
}

function sectionLabel(text: string, suffix?: string) {
  return (
    <div
      style={{
        fontSize: '0.75rem',
        fontWeight: 600,
        textTransform: 'uppercase' as const,
        letterSpacing: '0.07em',
        color: 'var(--muted)',
        marginBottom: '0.875rem',
      }}
    >
      {text}
      {suffix && (
        <span style={{ fontWeight: 400, textTransform: 'none', marginLeft: '0.5rem' }}>
          {suffix}
        </span>
      )}
    </div>
  )
}

function lastSeenLabel(date: string): string {
  const d = new Date(`${date}T00:00:00`)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

export default function PreviewPanel() {
  const [plan, setPlan] = useState<string | null>(null)
  const [sites, setSites] = useState<Site[]>([])
  const [activeSiteId, setActiveSiteId] = useState('')
  const [activeTheme, setActiveTheme] = useState('auto')
  const [saving, setSaving] = useState(false)
  const [analyticsData, setAnalyticsData] = useState<AnalyticsData | null>(null)
  const [analyticsLoading, setAnalyticsLoading] = useState(false)
  const [mobileTab, setMobileTab] = useState<'configure' | 'preview'>('configure')
  const [isMobileLayout, setIsMobileLayout] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement>(null)

  const hasAnalytics = plan !== null && PLAN_ORDER.indexOf(plan) >= PLAN_ORDER.indexOf('starter')
  const activeSite = sites.find((s) => s.id === activeSiteId)
  const apiBase = typeof window !== 'undefined' ? API || window.location.origin : ''

  // Initial load: billing status + sites list
  useEffect(() => {
    Promise.all([api.billing.status(), api.sites.list()])
      .then(([status, { sites: list }]) => {
        setPlan(status.plan)
        setSites(list)
        if (list.length > 0) {
          setActiveSiteId(list[0].id)
          setActiveTheme(list[0].theme || 'auto')
        }
      })
      .catch(() => setPlan('hobby'))
  }, [])

  // Sync active theme when the selected site changes
  useEffect(() => {
    if (!activeSiteId) return
    const site = sites.find((s) => s.id === activeSiteId)
    if (site) setActiveTheme(site.theme || 'auto')
  }, [activeSiteId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Mobile layout detection
  useEffect(() => {
    const check = () => setIsMobileLayout(window.innerWidth <= 768)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  // Fetch analytics for installation detection (Starter+ only)
  useEffect(() => {
    if (!activeSiteId || !hasAnalytics) {
      setAnalyticsData(null)
      return
    }
    setAnalyticsLoading(true)
    api.analytics
      .get(activeSiteId, '7d')
      .then((d) => {
        setAnalyticsData(d)
        setAnalyticsLoading(false)
      })
      .catch(() => {
        setAnalyticsData(null)
        setAnalyticsLoading(false)
      })
  }, [activeSiteId, hasAnalytics])

  const changeTheme = async (theme: string) => {
    if (!activeSiteId || saving) return
    const previous = activeTheme
    setActiveTheme(theme)
    setSaving(true)
    try {
      await api.sites.update(activeSiteId, { theme })
      setSites((prev) => prev.map((s) => (s.id === activeSiteId ? { ...s, theme } : s)))
    } catch {
      setActiveTheme(previous)
    } finally {
      setSaving(false)
    }
    iframeRef.current?.contentWindow?.postMessage({ type: 'qt:theme', theme }, '*')
  }

  const handleSiteChange = (siteId: string) => {
    setActiveSiteId(siteId)
    setAnalyticsData(null)
  }

  // Derive installation detection info from the analytics volume series
  const detectedPages = analyticsData?.pages.length ?? 0
  const lastSeen: string | null = (() => {
    if (!analyticsData?.volume?.length) return null
    const nonZero = [...analyticsData.volume].reverse().find((v) => v.count > 0)
    return nonZero?.date ?? null
  })()

  if (plan === null) return <div className="loading">Loading…</div>

  return (
    <div>
      {/* Page header */}
      <div className="page-header">
        <h1>Preview</h1>
        {sites.length > 1 && (
          <select
            value={activeSiteId}
            onChange={(e) => handleSiteChange((e.target as HTMLSelectElement).value)}
            style={{ fontSize: '0.875rem' }}
          >
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.domain}
              </option>
            ))}
          </select>
        )}
      </div>

      {sites.length === 0 ? (
        <div className="empty">
          No sites yet.{' '}
          <a href="/dashboard/sites" style={{ color: 'var(--amber)' }}>
            Add a site
          </a>{' '}
          to use the preview.
        </div>
      ) : (
        <>
          {isMobileLayout && (
            <div className="status-tabs">
              <button
                type="button"
                className={mobileTab === 'configure' ? 'active' : ''}
                onClick={() => setMobileTab('configure')}
              >
                Configure
              </button>
              <button
                type="button"
                className={mobileTab === 'preview' ? 'active' : ''}
                onClick={() => setMobileTab('preview')}
              >
                Preview
              </button>
            </div>
          )}

          <div className="preview-layout">
            {/* Left panel — theme + embed code */}
            {(!isMobileLayout || mobileTab === 'configure') && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
                <div className="preview-section" style={sectionStyle()}>
                  {sectionLabel('Theme', saving ? 'Saving…' : undefined)}
                  <ThemeSwatches value={activeTheme} onChange={changeTheme} disabled={saving} />
                </div>

                <div className="preview-section" style={sectionStyle()}>
                  {sectionLabel('Embed Code')}
                  {activeSite && <EmbedCodeGenerator siteId={activeSite.id} apiBase={apiBase} />}
                </div>
              </div>
            )}

            {/* Right panel — iframe preview + installation detection */}
            {(!isMobileLayout || mobileTab === 'preview') && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                <div
                  style={{
                    background: 'var(--card-bg)',
                    border: '1px solid var(--border)',
                    borderRadius: 8,
                    overflow: 'hidden',
                  }}
                >
                  {activeSiteId ? (
                    <iframe
                      ref={iframeRef}
                      key={activeSiteId}
                      src={`/embed-preview?siteId=${encodeURIComponent(activeSiteId)}`}
                      className="preview-iframe"
                      title="Embed preview"
                    />
                  ) : (
                    <div className="loading">Select a site to preview.</div>
                  )}
                </div>

                <div className="preview-section" style={sectionStyle()}>
                  {sectionLabel('Installation')}
                  {!hasAnalytics ? (
                    <p style={{ fontSize: '0.875rem', color: 'var(--muted)', margin: 0 }}>
                      Install the snippet on the left to get started.
                    </p>
                  ) : analyticsLoading ? (
                    <p style={{ fontSize: '0.875rem', color: 'var(--muted)', margin: 0 }}>
                      Checking…
                    </p>
                  ) : detectedPages > 0 ? (
                    <div style={{ fontSize: '0.875rem', color: 'var(--text)' }}>
                      <span style={{ color: 'var(--green-text)', fontWeight: 600 }}>Detected</span>{' '}
                      on {detectedPages} page{detectedPages !== 1 ? 's' : ''}
                      {lastSeen && (
                        <span style={{ color: 'var(--muted)' }}>
                          {' '}
                          · Last seen {lastSeenLabel(lastSeen)}
                        </span>
                      )}
                    </div>
                  ) : (
                    <p style={{ fontSize: '0.875rem', color: 'var(--muted)', margin: 0 }}>
                      Not yet detected — have you installed the snippet?
                    </p>
                  )}
                </div>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}
