import { useState, useEffect } from 'preact/hooks'
import {
  AreaChart, Area,
  BarChart, Bar,
  PieChart, Pie, Cell,
  XAxis, YAxis,
  CartesianGrid, Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { api } from '../api'
import type { AnalyticsData, Site } from '../types'
import UpgradeGate from './UpgradeGate'

const PLAN_ORDER = ['hobby', 'starter', 'pro', 'business']

type Range = '7d' | '30d' | 'all'
const RANGES: { label: string; value: Range }[] = [
  { label: '7 days',   value: '7d' },
  { label: '30 days',  value: '30d' },
  { label: 'All time', value: 'all' },
]

const AMBER    = 'var(--amber)'
const AMBER_HI = 'var(--amber-hi)'

const STATUS_COLORS: Record<string, string> = {
  approved: '#4A7C59',
  rejected: '#C0392B',
  pending:  '#C06020',
}

const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function fmt12h(hour: number): string {
  if (hour === 0)  return '12a'
  if (hour < 12)  return `${hour}a`
  if (hour === 12) return '12p'
  return `${hour - 12}p`
}

function truncate(s: string, max = 32): string {
  return s.length > max ? s.slice(0, max - 1) + '…' : s
}

// ---- Layout helpers --------------------------------------------------------

function sectionStyle(): object {
  return {
    background: 'var(--card-bg)',
    border: '1px solid var(--border)',
    borderRadius: 10,
    padding: '1.25rem 1.5rem',
    marginBottom: '1.5rem',
  }
}

function sectionLabel(text: string) {
  return (
    <div style={{
      fontSize: '0.75rem', fontWeight: 600, textTransform: 'uppercase' as const,
      letterSpacing: '0.07em', color: 'var(--muted)', marginBottom: '1rem',
    }}>
      {text}
    </div>
  )
}

// ---- Tooltips --------------------------------------------------------------

function VolumeTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null
  return (
    <div style={{ background: 'var(--card-bg)', border: '1px solid var(--border)', borderRadius: 6, padding: '0.5rem 0.75rem', fontSize: '0.8125rem', boxShadow: '0 2px 8px rgba(0,0,0,0.08)' }}>
      <div style={{ color: 'var(--muted)', marginBottom: '0.2rem' }}>{label}</div>
      <div style={{ fontWeight: 600, color: 'var(--text)' }}>{payload[0].value} comment{payload[0].value !== 1 ? 's' : ''}</div>
    </div>
  )
}

function BarTooltip({ active, payload }: any) {
  if (!active || !payload?.length) return null
  const name = payload[0].payload.page_title ?? payload[0].payload.display_name ?? payload[0].payload.label ?? ''
  return (
    <div style={{ background: 'var(--card-bg)', border: '1px solid var(--border)', borderRadius: 6, padding: '0.5rem 0.75rem', fontSize: '0.8125rem', boxShadow: '0 2px 8px rgba(0,0,0,0.08)', maxWidth: 260 }}>
      <div style={{ color: 'var(--muted)', marginBottom: '0.2rem', wordBreak: 'break-all' as const }}>{name}</div>
      <div style={{ fontWeight: 600, color: 'var(--text)' }}>{payload[0].value} comment{payload[0].value !== 1 ? 's' : ''}</div>
    </div>
  )
}

function StatusTooltip({ active, payload }: any) {
  if (!active || !payload?.length) return null
  const { status, count } = payload[0].payload
  return (
    <div style={{ background: 'var(--card-bg)', border: '1px solid var(--border)', borderRadius: 6, padding: '0.5rem 0.75rem', fontSize: '0.8125rem', boxShadow: '0 2px 8px rgba(0,0,0,0.08)' }}>
      <div style={{ color: 'var(--muted)', marginBottom: '0.2rem', textTransform: 'capitalize' as const }}>{status}</div>
      <div style={{ fontWeight: 600, color: 'var(--text)' }}>{count}</div>
    </div>
  )
}

// ---- Subcomponents ---------------------------------------------------------

function RangeToggle({ value, onChange }: { value: Range; onChange: (r: Range) => void }) {
  return (
    <div style={{ display: 'flex', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 6, padding: 2, gap: 2 }}>
      {RANGES.map(r => (
        <button key={r.value} onClick={() => onChange(r.value)} style={{
          background: value === r.value ? 'var(--card-bg)' : 'transparent',
          border: value === r.value ? '1px solid var(--border)' : '1px solid transparent',
          borderRadius: 4, padding: '0.25rem 0.625rem', cursor: 'pointer',
          fontFamily: 'var(--f-ui)', fontSize: '0.8125rem', fontWeight: 500,
          color: value === r.value ? 'var(--text)' : 'var(--muted)', transition: 'all 0.1s',
        }}>
          {r.label}
        </button>
      ))}
    </div>
  )
}

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div style={{ background: 'var(--card-bg)', border: '1px solid var(--border)', borderRadius: 10, padding: '1.25rem 1.5rem', flex: 1, minWidth: 160 }}>
      <div style={{ fontSize: '0.75rem', fontWeight: 600, textTransform: 'uppercase' as const, letterSpacing: '0.07em', color: 'var(--muted)', marginBottom: '0.5rem' }}>{label}</div>
      <div style={{ fontFamily: 'var(--f-display)', fontSize: '1.75rem', fontWeight: 700, color: AMBER, lineHeight: 1 }}>{value}</div>
      {sub && <div style={{ fontSize: '0.8125rem', color: 'var(--muted)', marginTop: '0.375rem' }}>{sub}</div>}
    </div>
  )
}

// ---- Main component --------------------------------------------------------

export default function AnalyticsPanel() {
  const [plan, setPlan]           = useState<string | null>(null)
  const [sites, setSites]         = useState<Site[]>([])
  const [siteId, setSiteId]       = useState<string>('')
  const [range, setRange]         = useState<Range>('30d')
  const [data, setData]           = useState<AnalyticsData | null>(null)
  const [loading, setLoading]     = useState(false)
  const [error, setError]         = useState<string | null>(null)
  const [isDark, setIsDark]       = useState(false)

  const isPro      = plan ? PLAN_ORDER.indexOf(plan) >= PLAN_ORDER.indexOf('pro') : false
  const isBusiness = plan === 'business'
  const hasAccess  = plan ? PLAN_ORDER.indexOf(plan) >= PLAN_ORDER.indexOf('starter') : false

  const gridColor   = isDark ? '#2E2C29' : '#D9D4CB'
  const axisColor   = isDark ? '#8A8480' : '#7A7570'
  const cursorColor = isDark ? 'rgba(224,127,50,0.12)' : '#F5E0CE'

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    setIsDark(mq.matches)
    const handler = (e: MediaQueryListEvent) => setIsDark(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])

  useEffect(() => {
    Promise.all([api.billing.status(), api.sites.list()])
      .then(([status, { sites }]) => {
        setPlan(status.plan)
        setSites(sites)
        if (sites.length > 0) setSiteId(sites[0].id)
      })
      .catch(() => setPlan('hobby'))
  }, [])

  useEffect(() => {
    if (!siteId || !hasAccess) return
    setLoading(true)
    setError(null)
    api.analytics.get(siteId, range)
      .then(d => { setData(d); setLoading(false) })
      .catch(() => { setError('Failed to load analytics.'); setLoading(false) })
  }, [siteId, range, hasAccess])

  if (plan === null) return <div className="loading">Loading…</div>

  if (!hasAccess) {
    return (
      <UpgradeGate
        feature="Analytics"
        description="Track comment volume over time, see your most active pages, top commenters, and more."
        minPlan="starter"
      />
    )
  }

  const isEmpty = data && data.volume.length === 0 && data.pages.length === 0

  return (
    <div>
      {/* Header */}
      <div className="page-header" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' as const, gap: '0.75rem' }}>
        <h1>Analytics</h1>
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
          {(sites.length > 1 || isBusiness) && (
            <select
              value={siteId}
              onChange={e => setSiteId((e.target as HTMLSelectElement).value)}
              className="input"
              style={{ margin: 0 }}
            >
              {isBusiness && <option value="all">All sites</option>}
              {sites.map(s => <option key={s.id} value={s.id}>{s.domain}</option>)}
            </select>
          )}
          <RangeToggle value={range} onChange={setRange} />
        </div>
      </div>

      {error && (
        <div style={{ background: 'var(--red-bg)', color: 'var(--red-text)', padding: '0.75rem 1rem', borderRadius: 6, marginBottom: '1rem', fontSize: '0.875rem' }}>
          {error}
        </div>
      )}

      {loading && <div style={{ color: 'var(--muted)', fontSize: '0.875rem', padding: '2rem 0' }}>Loading…</div>}

      {!loading && isEmpty && (
        <div className="empty">No comments yet for this site in the selected time range.</div>
      )}

      {!loading && data && !isEmpty && (
        <>
          {/* Business stat cards */}
          {isBusiness && data.return_rate !== undefined && (
            <div style={{ display: 'flex', gap: '0.875rem', marginBottom: '1.5rem', flexWrap: 'wrap' as const }}>
              <StatCard
                label="Return commenter rate"
                value={`${Math.round(data.return_rate)}%`}
                sub="commenters with more than one comment"
              />
              <StatCard
                label="Total commenters"
                value={data.commenters.length > 0 ? `${data.commenters.length}+` : '—'}
                sub="unique voices in this period"
              />
            </div>
          )}

          {/* Volume over time */}
          <div style={sectionStyle()}>
            {sectionLabel('Comment volume')}
            <ResponsiveContainer width="100%" height={220}>
              <AreaChart data={data.volume} margin={{ top: 4, right: 8, left: -16, bottom: 0 }}>
                <defs>
                  <linearGradient id="qt-amber-fill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%"  stopColor={AMBER_HI} stopOpacity={0.25} />
                    <stop offset="95%" stopColor={AMBER_HI} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke={gridColor} vertical={false} />
                <XAxis dataKey="date" tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false}
                  interval="preserveStartEnd"
                  tickFormatter={d => new Date(d + 'T00:00:00').toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
                />
                <YAxis tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} allowDecimals={false} />
                <Tooltip content={<VolumeTooltip />} />
                <Area type="monotone" dataKey="count" stroke={AMBER} strokeWidth={2}
                  fill="url(#qt-amber-fill)" dot={false}
                  activeDot={{ r: 4, fill: AMBER_HI, stroke: 'var(--card-bg)', strokeWidth: 2 }}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>

          {/* Top pages */}
          {data.pages.length > 0 && (
            <div style={sectionStyle()}>
              {sectionLabel('Top pages')}
              <ResponsiveContainer width="100%" height={Math.max(120, data.pages.length * 36)}>
                <BarChart data={data.pages.map(p => ({ ...p, label: truncate(p.page_title) }))}
                  layout="vertical" margin={{ top: 0, right: 16, left: 8, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke={gridColor} horizontal={false} />
                  <XAxis type="number" tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} allowDecimals={false} />
                  <YAxis type="category" dataKey="label" width={140} tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} />
                  <Tooltip content={<BarTooltip />} cursor={{ fill: cursorColor }} />
                  <Bar dataKey="count" fill={AMBER} radius={[0, 3, 3, 0]} maxBarSize={24} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}

          {/* Top commenters */}
          {data.commenters.length > 0 && (
            <div style={sectionStyle()}>
              {sectionLabel('Top commenters')}
              <ResponsiveContainer width="100%" height={Math.max(120, data.commenters.length * 36)}>
                <BarChart data={data.commenters.map(c => ({ ...c, label: truncate(c.display_name) }))}
                  layout="vertical" margin={{ top: 0, right: 16, left: 8, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke={gridColor} horizontal={false} />
                  <XAxis type="number" tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} allowDecimals={false} />
                  <YAxis type="category" dataKey="label" width={140} tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} />
                  <Tooltip content={<BarTooltip />} cursor={{ fill: cursorColor }} />
                  <Bar dataKey="count" fill={AMBER_HI} radius={[0, 3, 3, 0]} maxBarSize={24} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}

          {/* Pro+ features — locked gate for Starter */}
          {!isPro ? (
            <UpgradeGate
              feature="Advanced Analytics"
              description="Unlock spam & rejection rate breakdown, peak activity by hour and day of week, and more — upgrade to Pro."
              minPlan="pro"
            />
          ) : (
            <>
              {/* Status breakdown */}
              {data.status_breakdown && data.status_breakdown.length > 0 && (
                <div style={sectionStyle()}>
                  {sectionLabel('Comment status breakdown')}
                  <div style={{ display: 'flex', alignItems: 'center', gap: '2rem', flexWrap: 'wrap' as const }}>
                    <ResponsiveContainer width={200} height={200}>
                      <PieChart>
                        <Pie data={data.status_breakdown} dataKey="count" nameKey="status"
                          cx="50%" cy="50%" outerRadius={80} innerRadius={44}>
                          {data.status_breakdown.map((entry, i) => (
                            <Cell key={i} fill={STATUS_COLORS[entry.status] ?? axisColor} />
                          ))}
                        </Pie>
                        <Tooltip content={<StatusTooltip />} />
                      </PieChart>
                    </ResponsiveContainer>
                    <div style={{ display: 'flex', flexDirection: 'column' as const, gap: '0.5rem' }}>
                      {data.status_breakdown.map(s => (
                        <div key={s.status} style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.875rem' }}>
                          <span style={{ width: 10, height: 10, borderRadius: 2, background: STATUS_COLORS[s.status] ?? axisColor, flexShrink: 0 }} />
                          <span style={{ textTransform: 'capitalize' as const, color: 'var(--text)' }}>{s.status}</span>
                          <span style={{ color: 'var(--muted)', marginLeft: 'auto', paddingLeft: '1rem' }}>{s.count.toLocaleString()}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              )}

              {/* Peak activity — hours */}
              {data.peak_hours && (
                <div style={sectionStyle()}>
                  {sectionLabel('Peak activity — hour of day (UTC)')}
                  <ResponsiveContainer width="100%" height={160}>
                    <BarChart data={data.peak_hours.map(h => ({ ...h, label: fmt12h(h.hour) }))}
                      margin={{ top: 0, right: 8, left: -16, bottom: 0 }}>
                      <CartesianGrid strokeDasharray="3 3" stroke={gridColor} vertical={false} />
                      <XAxis dataKey="label" tick={{ fontSize: 10, fill: axisColor }} tickLine={false} axisLine={false} interval={1} />
                      <YAxis tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} allowDecimals={false} />
                      <Tooltip content={<BarTooltip />} cursor={{ fill: cursorColor }} />
                      <Bar dataKey="count" fill={AMBER} radius={[2, 2, 0, 0]} maxBarSize={28} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              )}

              {/* Peak activity — day of week */}
              {data.peak_days && (
                <div style={sectionStyle()}>
                  {sectionLabel('Peak activity — day of week')}
                  <ResponsiveContainer width="100%" height={160}>
                    <BarChart data={data.peak_days.map(d => ({ ...d, label: DAY_LABELS[d.day] }))}
                      margin={{ top: 0, right: 8, left: -16, bottom: 0 }}>
                      <CartesianGrid strokeDasharray="3 3" stroke={gridColor} vertical={false} />
                      <XAxis dataKey="label" tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} />
                      <YAxis tick={{ fontSize: 11, fill: axisColor }} tickLine={false} axisLine={false} allowDecimals={false} />
                      <Tooltip content={<BarTooltip />} cursor={{ fill: cursorColor }} />
                      <Bar dataKey="count" fill={AMBER_HI} radius={[2, 2, 0, 0]} maxBarSize={48} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  )
}
