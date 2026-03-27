import { useCallback, useEffect, useState } from 'preact/hooks'
import { api } from '../api'
import type { Comment, Site } from '../types'
import { relativeTime, stripHtml, truncate } from '../utils'

const PAGE_SIZE = 20

const ChevronIcon = () => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2.5"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <path d="M9 18l6-6-6-6" />
  </svg>
)

export default function ModerationQueue() {
  const [comments, setComments] = useState<Comment[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [tab, setTab] = useState<'pending' | 'flagged'>('pending')

  const [sites, setSites] = useState<Site[]>([])
  const [siteFilter, setSiteFilter] = useState('')

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [expanded, setExpanded] = useState<string | null>(null)
  const [editing, setEditing] = useState<string | null>(null)
  const [editContent, setEditContent] = useState('')
  const [replying, setReplying] = useState<string | null>(null)
  const [replyContent, setReplyContent] = useState('')

  const [acting, setActing] = useState<string | null>(null)
  const [bulkActing, setBulkActing] = useState(false)

  const fetchComments = useCallback(async (p: number, siteId: string, t: 'pending' | 'flagged') => {
    setLoading(true)
    setError(null)
    try {
      const res = await api.comments.list({
        flagged: t === 'flagged',
        status: t === 'pending' ? 'pending' : undefined,
        page: p,
        limit: PAGE_SIZE,
        siteId,
      })
      setComments(res.comments ?? [])
      setTotal(res.total)
      setPage(p)
      setSelected(new Set())
    } catch {
      setError('Failed to load comments.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    api.sites
      .list()
      .then((r) => setSites(r.sites ?? []))
      .catch(() => {})
  }, [])

  useEffect(() => {
    fetchComments(1, siteFilter, tab)
  }, [siteFilter, tab, fetchComments])

  const allSelected = comments.length > 0 && comments.every((c) => selected.has(c.id))

  const toggleSelectAll = () => {
    setSelected(allSelected ? new Set() : new Set(comments.map((c) => c.id)))
  }

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const removeFromList = (ids: string[]) => {
    const set = new Set(ids)
    setComments((prev) => prev.filter((c) => !set.has(c.id)))
    setTotal((t) => Math.max(0, t - ids.length))
    setSelected((prev) => {
      const n = new Set(prev)
      ids.forEach((id) => {
        n.delete(id)
      })
      return n
    })
  }

  const changeStatus = async (id: string, status: string) => {
    setActing(id)
    try {
      await api.comments.update(id, { status })
      removeFromList([id])
      if (expanded === id) setExpanded(null)
    } finally {
      setActing(null)
    }
  }

  const remove = async (id: string) => {
    if (!confirm('Delete this comment permanently?')) return
    setActing(id)
    try {
      await api.comments.delete(id)
      removeFromList([id])
      if (expanded === id) setExpanded(null)
    } finally {
      setActing(null)
    }
  }

  const bulkAction = async (status: 'approved' | 'rejected') => {
    setBulkActing(true)
    const ids = [...selected]
    try {
      await Promise.all(ids.map((id) => api.comments.update(id, { status })))
      removeFromList(ids)
    } finally {
      setBulkActing(false)
    }
  }

  const toggleExpand = (id: string) => {
    const opening = expanded !== id
    setExpanded(opening ? id : null)
    setEditing(null)
    setEditContent('')
    setReplying(null)
    setReplyContent('')
  }

  const startEdit = (c: Comment) => {
    setEditing(c.id)
    setEditContent(c.content)
    setReplying(null)
    setReplyContent('')
  }

  const saveEdit = async (id: string) => {
    if (!editContent.trim()) return
    setActing(id)
    try {
      const updated = (await api.comments.update(id, { content: editContent })) as Comment
      setComments((prev) => prev.map((c) => (c.id === id ? updated : c)))
      setEditing(null)
      setEditContent('')
    } finally {
      setActing(null)
    }
  }

  const startReply = (id: string) => {
    setReplying(id)
    setReplyContent('')
    setEditing(null)
    setEditContent('')
  }

  const submitReply = async (id: string) => {
    if (!replyContent.trim()) return
    setActing(id)
    try {
      await api.comments.reply(id, `<p>${replyContent.trim()}</p>`)
      setReplying(null)
      setReplyContent('')
    } finally {
      setActing(null)
    }
  }

  const busy = acting !== null || bulkActing
  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <>
      <div className="queue-tabs">
        {(['pending', 'flagged'] as const).map((t) => (
          <button
            type="button"
            key={t}
            className={`queue-tab${tab === t ? ' active' : ''}`}
            onClick={() => {
              if (tab !== t) {
                setTab(t)
                setExpanded(null)
                setEditing(null)
                setReplying(null)
              }
            }}
          >
            {t === 'pending' ? 'Pending' : 'Flagged'}
          </button>
        ))}
      </div>

      <div className="queue-toolbar">
        {sites.length > 1 && (
          <select
            value={siteFilter}
            onChange={(e) => setSiteFilter((e.target as HTMLSelectElement).value)}
          >
            <option value="">All sites</option>
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.domain}
              </option>
            ))}
          </select>
        )}
        <span className="page-count" style={{ marginLeft: 'auto' }}>
          {total} {tab === 'flagged' ? 'flagged' : 'pending'}
        </span>
      </div>

      {selected.size > 0 && (
        <div className="bulk-bar">
          <span style={{ color: 'var(--blue-text)', fontWeight: 500 }}>
            {selected.size} selected
          </span>
          <button
            type="button"
            className="btn btn-approve"
            disabled={busy}
            onClick={() => bulkAction('approved')}
          >
            Approve {selected.size}
          </button>
          <button
            type="button"
            className="btn btn-reject"
            disabled={busy}
            onClick={() => bulkAction('rejected')}
          >
            Reject {selected.size}
          </button>
          <button
            type="button"
            className="btn btn-ghost"
            style={{ marginLeft: 'auto' }}
            onClick={() => setSelected(new Set())}
          >
            Clear
          </button>
        </div>
      )}

      {loading ? (
        <div className="loading">Loading…</div>
      ) : error ? (
        <div className="error-msg">{error}</div>
      ) : comments.length === 0 ? (
        <div className="empty">
          {tab === 'flagged' ? 'No flagged comments.' : 'No pending comments.'}
        </div>
      ) : (
        <>
          <div className="mq-scroll">
            <div className="mq-grid">
              <div className="mq-header">
                <div>
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={toggleSelectAll}
                    title="Select all on this page"
                  />
                </div>
                <div>Author</div>
                <div>Comment</div>
                <div>Page</div>
                <div>Date</div>
                <div>Actions</div>
              </div>

              {comments.map((c) => (
                <div key={c.id} className={`mq-row${selected.has(c.id) ? ' selected' : ''}`}>
                  <div className="mq-cell mq-cell-check">
                    <input
                      type="checkbox"
                      checked={selected.has(c.id)}
                      onChange={() => toggleSelect(c.id)}
                    />
                  </div>

                  <div className="mq-cell" data-label="From" style={{ fontWeight: 500 }}>
                    {c.author_name || c.disqus_author || (
                      <span style={{ color: 'var(--muted)' }}>—</span>
                    )}
                    {!!c.flags && c.flags > 0 && (
                      <span
                        className="flag-badge"
                        title={`${c.flags} flag${c.flags === 1 ? '' : 's'}`}
                      >
                        {c.flags}
                      </span>
                    )}
                  </div>

                  <div className="mq-cell" data-label="Comment" style={{ gap: '0.25rem' }}>
                    <span
                      style={{
                        flex: 1,
                        minWidth: 0,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        color: 'var(--muted)',
                      }}
                    >
                      {truncate(stripHtml(c.content), 90)}
                    </span>
                    <button
                      type="button"
                      className={`expand-toggle${expanded === c.id ? ' open' : ''}`}
                      onClick={() => toggleExpand(c.id)}
                      title={expanded === c.id ? 'Collapse' : 'Expand'}
                    >
                      <ChevronIcon />
                    </button>
                  </div>

                  <div
                    className="mq-cell"
                    data-label="Page"
                    style={{ color: 'var(--muted)', fontSize: '0.8125rem' }}
                  >
                    <span
                      style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                    >
                      {c.page_title || c.page_id}
                    </span>
                  </div>

                  <div
                    className="mq-cell"
                    data-label="Date"
                    style={{ color: 'var(--muted)', fontSize: '0.8125rem', whiteSpace: 'nowrap' }}
                  >
                    {relativeTime(c.created_at)}
                  </div>

                  <div className="mq-cell mq-cell-actions">
                    <div className="actions">
                      <button
                        type="button"
                        className="btn btn-approve"
                        disabled={busy}
                        onClick={() => changeStatus(c.id, 'approved')}
                      >
                        Approve
                      </button>
                      <button
                        type="button"
                        className="btn btn-reject"
                        disabled={busy}
                        onClick={() => changeStatus(c.id, 'rejected')}
                      >
                        Reject
                      </button>
                      <button
                        type="button"
                        className="btn"
                        disabled={busy}
                        onClick={() => remove(c.id)}
                      >
                        Delete
                      </button>
                    </div>
                  </div>

                  {/* Accordion — always in DOM, animated via grid-template-rows */}
                  <div className={`mq-expand${expanded === c.id ? ' open' : ''}`}>
                    <div className="mq-expand-inner">
                      <div className="mq-expand-content">
                        {editing === c.id ? (
                          <div>
                            <div className="expand-label">Edit content</div>
                            <textarea
                              value={editContent}
                              onChange={(e) =>
                                setEditContent((e.target as HTMLTextAreaElement).value)
                              }
                              style={{
                                width: '100%',
                                minHeight: 100,
                                fontFamily: 'var(--f-mono)',
                                fontSize: '0.8125rem',
                                resize: 'vertical',
                              }}
                            />
                            <div className="expand-actions">
                              <button
                                type="button"
                                className="btn btn-primary"
                                disabled={acting === c.id}
                                onClick={() => saveEdit(c.id)}
                              >
                                Save changes
                              </button>
                              <button
                                type="button"
                                className="btn"
                                onClick={() => {
                                  setEditing(null)
                                  setEditContent('')
                                }}
                              >
                                Cancel
                              </button>
                            </div>
                          </div>
                        ) : replying === c.id ? (
                          <div>
                            <div className="expand-label">Reply as admin</div>
                            <textarea
                              value={replyContent}
                              onChange={(e) =>
                                setReplyContent((e.target as HTMLTextAreaElement).value)
                              }
                              placeholder="Write your reply…"
                              style={{ width: '100%', minHeight: 80, resize: 'vertical' }}
                            />
                            <div className="expand-hint">Published immediately as approved.</div>
                            <div className="expand-actions">
                              <button
                                type="button"
                                className="btn btn-primary"
                                disabled={acting === c.id}
                                onClick={() => submitReply(c.id)}
                              >
                                Send reply
                              </button>
                              <button
                                type="button"
                                className="btn"
                                onClick={() => {
                                  setReplying(null)
                                  setReplyContent('')
                                }}
                              >
                                Cancel
                              </button>
                            </div>
                          </div>
                        ) : (
                          <div>
                            <div
                              className="comment-prose"
                              dangerouslySetInnerHTML={{ __html: c.content }}
                            />
                            <div className="expand-actions">
                              <button type="button" className="btn" onClick={() => startEdit(c)}>
                                Edit
                              </button>
                              <button
                                type="button"
                                className="btn"
                                onClick={() => startReply(c.id)}
                              >
                                Reply
                              </button>
                            </div>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {totalPages > 1 && (
            <div className="pagination">
              <button
                type="button"
                className="btn"
                disabled={page <= 1}
                onClick={() => fetchComments(page - 1, siteFilter, tab)}
              >
                ←
              </button>
              <span>
                {page} / {totalPages}
              </span>
              <button
                type="button"
                className="btn"
                disabled={page >= totalPages}
                onClick={() => fetchComments(page + 1, siteFilter, tab)}
              >
                →
              </button>
            </div>
          )}
        </>
      )}
    </>
  )
}
