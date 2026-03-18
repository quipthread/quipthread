import { useState, useEffect, useCallback } from 'preact/hooks'
import { api } from '../api'
import { relativeTime, stripHtml, truncate } from '../utils'
import type { Comment } from '../types'
import ModerationQueue from './ModerationQueue'

const STATUSES = ['pending', 'approved', 'rejected'] as const
type Status = (typeof STATUSES)[number]

const PAGE_SIZE = 20

export default function CommentsPanel() {
  const [status, setStatus] = useState<Status>('pending')

  // State for approved / rejected tabs only — pending is owned by ModerationQueue
  const [comments, setComments] = useState<Comment[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState<string | null>(null)

  const fetchComments = useCallback(async (p: number, s: Status) => {
    if (s === 'pending') return
    setLoading(true)
    setError(null)
    try {
      const res = await api.comments.list({ status: s, page: p, limit: PAGE_SIZE })
      setComments(res.comments ?? [])
      setTotal(res.total)
      setPage(p)
    } catch {
      setError('Failed to load comments.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (status !== 'pending') fetchComments(1, status)
  }, [status, fetchComments])

  const changeStatus = async (id: string, next: string) => {
    setActing(id)
    try {
      await api.comments.update(id, { status: next })
      setComments(prev => prev.filter(c => c.id !== id))
      setTotal(t => Math.max(0, t - 1))
    } finally {
      setActing(null)
    }
  }

  const remove = async (id: string) => {
    if (!confirm('Delete this comment permanently?')) return
    setActing(id)
    try {
      await api.comments.delete(id)
      setComments(prev => prev.filter(c => c.id !== id))
      setTotal(t => Math.max(0, t - 1))
    } finally {
      setActing(null)
    }
  }

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <>
      <div className="page-header">
        <h1>Comments</h1>
        {status !== 'pending' && (
          <span className="page-count">{total} total</span>
        )}
      </div>

      <div className="status-tabs">
        {STATUSES.map(s => (
          <button
            key={s}
            className={status === s ? 'active' : ''}
            onClick={() => setStatus(s)}
          >
            {s.charAt(0).toUpperCase() + s.slice(1)}
          </button>
        ))}
      </div>

      {status === 'pending' ? (
        <ModerationQueue />
      ) : loading ? (
        <div className="loading">Loading…</div>
      ) : error ? (
        <div className="error-msg">{error}</div>
      ) : comments.length === 0 ? (
        <div className="empty">No {status} comments.</div>
      ) : (
        <>
          <div className="table-card">
            <table>
              <thead>
                <tr>
                  <th>Author</th>
                  <th>Comment</th>
                  <th>Page</th>
                  <th>Date</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {comments.map(c => (
                  <tr key={c.id}>
                    <td style={{ whiteSpace: 'nowrap', fontWeight: 500 }}>
                      {c.author_name || c.disqus_author || (
                        <span style={{ color: 'var(--muted)' }}>—</span>
                      )}
                    </td>
                    <td style={{ maxWidth: 300, color: 'var(--muted)' }}>
                      {truncate(stripHtml(c.content), 120)}
                    </td>
                    <td style={{ maxWidth: 180, wordBreak: 'break-all', color: 'var(--muted)', fontSize: '0.8125rem' }}>
                      {c.page_title || c.page_id}
                    </td>
                    <td style={{ whiteSpace: 'nowrap', color: 'var(--muted)', fontSize: '0.8125rem' }}>
                      {relativeTime(c.created_at)}
                    </td>
                    <td>
                      <div className="actions">
                        {status !== 'approved' && (
                          <button
                            className="btn btn-approve"
                            disabled={acting === c.id}
                            onClick={() => changeStatus(c.id, 'approved')}
                          >
                            Approve
                          </button>
                        )}
                        {status !== 'rejected' && (
                          <button
                            className="btn btn-reject"
                            disabled={acting === c.id}
                            onClick={() => changeStatus(c.id, 'rejected')}
                          >
                            Reject
                          </button>
                        )}
                        <button
                          className="btn"
                          disabled={acting === c.id}
                          onClick={() => remove(c.id)}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="pagination">
              <button
                className="btn"
                disabled={page <= 1}
                onClick={() => fetchComments(page - 1, status)}
              >
                ←
              </button>
              <span>{page} / {totalPages}</span>
              <button
                className="btn"
                disabled={page >= totalPages}
                onClick={() => fetchComments(page + 1, status)}
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
