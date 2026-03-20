import { useState, useEffect, useRef } from 'preact/hooks'
import type { ComponentChildren } from 'preact'
import { api } from '../api'
import type { Site, TableInfo, ImportResult, ColumnMapping } from '../types'
import SelectDropdown from './SelectDropdown'

// ── Types ─────────────────────────────────────────────────────────────────────

type Source =
  | 'disqus'
  | 'wordpress'
  | 'remark42'
  | 'native'
  | 'quipthread'
  | 'sqlite'

type Phase = 'configure' | 'inspecting' | 'mapping' | 'importing' | 'done' | 'error'

const SOURCE_LABELS: Record<Source, string> = {
  disqus: 'Disqus',
  wordpress: 'WordPress WXR',
  remark42: 'Remark42',
  native: 'Quipthread JSON',
  quipthread: 'Quipthread SQLite',
  sqlite: 'Generic SQLite',
}

const SOURCE_HINTS: Record<Source, string> = {
  disqus: 'XML export from Disqus dashboard → Admin → Moderation → Export',
  wordpress: 'WXR file from WordPress Admin → Tools → Export',
  remark42: 'JSON export from Remark42 admin panel',
  native: 'Quipthread native JSON export (version 1)',
  quipthread: 'Quipthread SQLite database file (e.g. quipthread.db)',
  sqlite: 'Any SQLite file — map columns to Quipthread fields',
}

// Quipthread fields the user can map to source columns
const MAPPING_FIELDS: { key: string; label: string; required?: boolean; hint?: string }[] = [
  { key: 'content', label: 'Content', required: true, hint: 'Comment body (HTML or plain text)' },
  { key: 'page_url', label: 'Page URL', hint: 'Full URL of the page the comment belongs to' },
  { key: 'page_id', label: 'Page ID', hint: 'Explicit page identifier (defaults to URL path)' },
  { key: 'page_title', label: 'Page Title', hint: 'Display title of the page' },
  { key: 'author_name', label: 'Author Name', hint: 'Comment author display name' },
  { key: 'author_avatar', label: 'Author Avatar', hint: 'URL to the author\'s avatar image' },
  { key: 'parent_id', label: 'Parent ID', hint: 'ID of the parent comment for threaded replies' },
  { key: 'status', label: 'Status', hint: 'approved / pending / rejected (defaults to approved)' },
  { key: 'created_at', label: 'Created At', hint: 'Timestamp (ISO 8601, RFC 3339, or YYYY-MM-DD HH:MM:SS)' },
  { key: 'id', label: 'ID', hint: 'Source comment ID — used for dedup on re-import' },
]

// ── Sub-components ────────────────────────────────────────────────────────────

function Section({ title, children }: { title: string; children: ComponentChildren }) {
  return (
    <div class="import-section">
      <div class="import-section-title">{title}</div>
      {children}
    </div>
  )
}

function FileInput({
  accept,
  onFile,
  file,
}: {
  accept: string
  onFile: (f: File) => void
  file: File | null
}) {
  const ref = useRef<HTMLInputElement>(null)
  return (
    <div
      class={`file-drop ${file ? 'has-file' : ''}`}
      onClick={() => ref.current?.click()}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault()
        const f = e.dataTransfer?.files[0]
        if (f) onFile(f)
      }}
    >
      <input
        ref={ref}
        type="file"
        accept={accept}
        style="display:none"
        onChange={(e) => {
          const f = (e.target as HTMLInputElement).files?.[0]
          if (f) onFile(f)
        }}
      />
      {file ? (
        <span class="file-name">{file.name}</span>
      ) : (
        <span class="file-placeholder">Click to choose a file or drag and drop</span>
      )}
    </div>
  )
}

function Spinner() {
  return <div class="spinner" />
}

function ResultCard({ result }: { result: ImportResult }) {
  return (
    <div class="result-card">
      <div class="result-row">
        <span class="result-label">Comments imported</span>
        <span class="result-value">{result.comments_inserted}</span>
      </div>
      <div class="result-row">
        <span class="result-label">Users created</span>
        <span class="result-value">{result.users_inserted}</span>
      </div>
    </div>
  )
}

// ── Column Mapper ─────────────────────────────────────────────────────────────

function ColumnMapper({
  tables,
  mapping,
  onMappingChange,
}: {
  tables: TableInfo[]
  mapping: ColumnMapping
  onMappingChange: (m: ColumnMapping) => void
}) {
  const selectedTable = tables.find((t) => t.name === mapping.table)
  const columns = selectedTable?.columns ?? []

  function set(updates: Partial<ColumnMapping>) {
    onMappingChange({ ...mapping, ...updates })
  }

  function setCol(field: string, col: string) {
    set({ columns: { ...mapping.columns, [field]: col } })
  }

  return (
    <div class="mapper">
      <div class="mapper-row">
        <label class="mapper-label">Source table</label>
        <select
          value={mapping.table}
          onChange={(e) => set({ table: (e.target as HTMLSelectElement).value, columns: {} })}
        >
          <option value="">— select table —</option>
          {tables.map((t) => (
            <option key={t.name} value={t.name}>
              {t.name}
            </option>
          ))}
        </select>
      </div>

      {selectedTable && (
        <>
          <div class="mapper-fields">
            {MAPPING_FIELDS.map(({ key, label, required, hint }) => (
              <div key={key} class="mapper-row">
                <div class="mapper-label">
                  {label}
                  {required && <span class="required">*</span>}
                  {hint && <span class="mapper-hint">{hint}</span>}
                </div>
                <select
                  value={mapping.columns[key] ?? ''}
                  onChange={(e) => setCol(key, (e.target as HTMLSelectElement).value)}
                >
                  <option value="">{required ? '— required —' : '— skip —'}</option>
                  {columns.map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.name}
                      {c.type ? ` (${c.type})` : ''}
                      {c.samples?.length ? ` — e.g. "${c.samples[0]}"` : ''}
                    </option>
                  ))}
                </select>
              </div>
            ))}
          </div>

          <div class="mapper-options">
            <label class="checkbox-label">
              <input
                type="checkbox"
                checked={mapping.strip_domain}
                onChange={(e) => set({ strip_domain: (e.target as HTMLInputElement).checked })}
              />
              Derive page ID from page URL path (strip domain)
            </label>
            <label class="checkbox-label">
              <input
                type="checkbox"
                checked={mapping.wrap_in_p}
                onChange={(e) => set({ wrap_in_p: (e.target as HTMLInputElement).checked })}
              />
              Wrap plain-text content in &lt;p&gt; tags
            </label>
          </div>
        </>
      )}
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export default function ImportPanel() {
  const [sites, setSites] = useState<Site[]>([])
  const [source, setSource] = useState<Source>('disqus')
  const [siteId, setSiteId] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [phase, setPhase] = useState<Phase>('configure')
  const [error, setError] = useState('')
  const [tables, setTables] = useState<TableInfo[]>([])
  const [mapping, setMapping] = useState<ColumnMapping>({
    table: '',
    columns: {},
    strip_domain: true,
    wrap_in_p: false,
  })
  const [result, setResult] = useState<ImportResult | null>(null)

  useEffect(() => {
    api.sites.list().then((r) => {
      setSites(r.sites ?? [])
      if (r.sites?.length) setSiteId(r.sites[0].id)
    })
  }, [])

  function reset() {
    setPhase('configure')
    setFile(null)
    setError('')
    setResult(null)
    setTables([])
    setMapping({ table: '', columns: {}, strip_domain: true, wrap_in_p: false })
  }

  function acceptForSource(): string {
    if (source === 'disqus' || source === 'wordpress') return '.xml,application/xml,text/xml'
    if (source === 'remark42' || source === 'native') return '.json,application/json'
    return '.db,.sqlite,.sqlite3'
  }

  async function handleRun() {
    if (!file) return
    if (!siteId) { setError('Select a site first.'); return }

    if (source === 'sqlite') {
      // Phase 1: inspect
      setPhase('inspecting')
      setError('')
      try {
        const schema = await api.imports.sqliteInspect(file)
        setTables(schema.tables)
        setPhase('mapping')
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Inspect failed')
        setPhase('error')
      }
      return
    }

    await runImport()
  }

  async function runImport() {
    if (!file) return
    setPhase('importing')
    setError('')
    try {
      let res: ImportResult
      switch (source) {
        case 'disqus':     res = await api.imports.disqus(siteId, file); break
        case 'wordpress':  res = await api.imports.wordpress(siteId, file); break
        case 'remark42':   res = await api.imports.remark42(siteId, file); break
        case 'native':     res = await api.imports.native(siteId, file); break
        case 'quipthread': res = await api.imports.quipthread(siteId, file); break
        case 'sqlite':     res = await api.imports.sqliteRun(siteId, file, mapping); break
        default: throw new Error('Unknown source')
      }
      setResult(res)
      setPhase('done')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Import failed')
      setPhase('error')
    }
  }

  const isLoading = phase === 'inspecting' || phase === 'importing'
  const canRun =
    !!file &&
    !!siteId &&
    (source !== 'sqlite' || phase !== 'mapping' || !!mapping.table)

  return (
    <div class="import-panel">
      <style>{styles}</style>

      <div class="page-header">
        <h1>Import Comments</h1>
      </div>

      {phase === 'done' && result && (
        <div class="import-done">
          <div class="done-header">Import complete</div>
          <ResultCard result={result} />
          <button class="btn btn-primary" style="margin-top:1.25rem" onClick={reset}>
            Import another
          </button>
        </div>
      )}

      {phase === 'error' && (
        <div class="import-error-wrap">
          <div class="error-msg">{error}</div>
          <button class="btn" onClick={reset}>Back</button>
        </div>
      )}

      {(phase === 'configure' || phase === 'inspecting' || phase === 'importing' || phase === 'mapping') && (
        <div class="import-form">
          {/* Source */}
          <Section title="1. Source format">
            <div class="source-grid">
              {(Object.keys(SOURCE_LABELS) as Source[]).map((s) => (
                <button
                  key={s}
                  class={`source-btn ${source === s ? 'active' : ''}`}
                  onClick={() => { setSource(s); reset() }}
                  disabled={isLoading}
                >
                  {SOURCE_LABELS[s]}
                </button>
              ))}
            </div>
            <div class="source-hint">{SOURCE_HINTS[source]}</div>
          </Section>

          {/* Site */}
          <Section title="2. Target site">
            {sites.length === 0 ? (
              <p class="import-hint">No sites found. <a href="/dashboard/sites">Create a site</a> first.</p>
            ) : (
              <SelectDropdown
                value={siteId}
                options={sites.map(s => ({ value: s.id, label: s.domain }))}
                onChange={setSiteId}
                disabled={isLoading}
              />
            )}
          </Section>

          {/* File */}
          <Section title={source === 'sqlite' && phase === 'mapping' ? '3. File (re-upload for import)' : '3. File'}>
            <FileInput accept={acceptForSource()} onFile={setFile} file={file} />
          </Section>

          {/* Column mapper (generic SQLite only, shown after inspect) */}
          {source === 'sqlite' && phase === 'mapping' && tables.length > 0 && (
            <Section title="4. Map columns">
              <ColumnMapper tables={tables} mapping={mapping} onMappingChange={setMapping} />
            </Section>
          )}

          {/* Action */}
          <div class="import-actions">
            {isLoading ? (
              <div class="loading-row">
                <Spinner />
                <span>{phase === 'inspecting' ? 'Reading schema…' : 'Importing…'}</span>
              </div>
            ) : phase === 'mapping' ? (
              <button
                class="btn btn-primary"
                disabled={!canRun || !file || !mapping.table}
                onClick={runImport}
              >
                Run import
              </button>
            ) : (
              <button class="btn btn-primary" disabled={!canRun} onClick={handleRun}>
                {source === 'sqlite' ? 'Inspect file' : 'Import'}
              </button>
            )}
            {error && phase !== 'error' && <div class="error-msg" style="margin-top:0.75rem">{error}</div>}
          </div>
        </div>
      )}
    </div>
  )
}

// ── Styles ────────────────────────────────────────────────────────────────────

const styles = `
.import-panel {
  max-width: 760px;
}

.import-form {
  display: flex;
  flex-direction: column;
  gap: 2rem;
}

.import-section {
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
}

.import-section-title {
  font-size: 0.8125rem;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--muted);
}

.source-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.source-btn {
  padding: 0.375rem 0.875rem;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  color: var(--text);
  font-family: var(--f-ui);
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.1s, border-color 0.1s, color 0.1s;
}

.source-btn:hover:not(:disabled):not(.active) {
  background: var(--surface);
}

.source-btn.active {
  background: var(--ink);
  color: var(--paper);
  border-color: var(--ink);
}

.source-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.source-hint {
  font-size: 0.8125rem;
  color: var(--muted);
  line-height: 1.5;
}

.file-drop {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 72px;
  border: 1.5px dashed var(--border);
  border-radius: 8px;
  background: var(--surface);
  cursor: pointer;
  padding: 1rem;
  transition: border-color 0.15s, background 0.15s;
}

.file-drop:hover {
  border-color: var(--amber);
  background: var(--amber-bg);
}

.file-drop.has-file {
  border-style: solid;
  border-color: var(--amber-border);
  background: var(--amber-bg);
}

.file-placeholder {
  font-size: 0.875rem;
  color: var(--muted);
}

.file-name {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--amber);
  word-break: break-all;
  text-align: center;
}

.import-actions {
  padding-top: 0.5rem;
}

.loading-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-size: 0.9375rem;
  color: var(--muted);
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2.5px solid var(--border);
  border-top-color: var(--amber);
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
  flex-shrink: 0;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.import-done {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 2rem;
  max-width: 480px;
}

.done-header {
  font-family: var(--f-display);
  font-size: 1.125rem;
  font-weight: 600;
  margin-bottom: 1.25rem;
  color: var(--text);
}

.result-card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.result-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.625rem 0;
  border-bottom: 1px solid var(--surface);
  font-size: 0.9375rem;
}

.result-row:last-child {
  border-bottom: none;
}

.result-label {
  color: var(--muted);
}

.result-value {
  font-weight: 600;
  font-size: 1rem;
  font-variant-numeric: tabular-nums;
  color: var(--text);
}

.import-error-wrap {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-width: 560px;
}

.import-hint {
  font-size: 0.875rem;
  color: var(--muted);
  margin: 0;
}

.import-hint a {
  color: var(--amber);
}

/* Column mapper */
.mapper {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.mapper-fields {
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 1rem 1.25rem;
}

.mapper-row {
  display: grid;
  grid-template-columns: 220px 1fr;
  align-items: start;
  gap: 1rem;
}

.mapper-label {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text);
  display: flex;
  flex-direction: column;
  gap: 0.1875rem;
  padding-top: 0.45rem;
}

.mapper-hint {
  font-size: 0.75rem;
  font-weight: 400;
  color: var(--muted);
  line-height: 1.4;
}

.required {
  color: var(--amber);
  margin-left: 2px;
}

.mapper-row select {
  width: 100%;
}

@media (max-width: 600px) {
  .mapper-row {
    grid-template-columns: 1fr;
    gap: 0.375rem;
  }

  .mapper-label {
    padding-top: 0;
  }
}

.mapper-options {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: var(--text);
  cursor: pointer;
}
`
