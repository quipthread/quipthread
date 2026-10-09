import { useQuery, useQueryClient } from '@tanstack/preact-query'
import { useState } from 'preact/hooks'
import { api } from '../api'
import { queryKeys } from '../lib/queryKeys'
import type { Comment } from '../types'
import { relativeTime, stripHtml, truncate } from '../utils'
import ModerationQueue from './ModerationQueue'
import QueryProvider from './QueryProvider'
import PageHeader from './shared/PageHeader'

const STATUSES = ['pending', 'approved', 'rejected'] as const
type Status = (typeof STATUSES)[number]

const PAGE_SIZE = 20

function CommentsPanelInner() {
  const [status, setStatus] = useState<Status>('pending')
  const [page, setPage] = useState(1)
  const [acting, setActing] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.comments({ status, page, limit: PAGE_SIZE }),
    queryFn: () => api.comments.list({ status, page, limit: PAGE_SIZE }),
    enabled: status !== 'pending',
  })

  // Shares the cached sites query with ModerationQueue.
  const { data: sitesData } = useQuery({
    queryKey: queryKeys.sites(),
    queryFn: () => api.sites.list(),
  })
  const hasNoSites = sitesData !== undefined && sitesData.sites.length === 0

  const comments = data?.comments ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)

  function handleStatusChange(s: Status) {
    setStatus(s)
    setPage(1)
  }

  function removeFromCache(id: string) {
    queryClient.setQueryData<{ comments: Comment[]; total: number }>(
      queryKeys.comments({ status, page, limit: PAGE_SIZE }),
      (old) =>
        old
          ? {
              ...old,
              comments: old.comments.filter((c) => c.id !== id),
              total: Math.max(0, old.total - 1),
            }
          : old,
    )
    queryClient.invalidateQueries({ queryKey: queryKeys.allComments() })
  }

  const changeStatus = async (id: string, next: string) => {
    setActing(id)
    try {
      await api.comments.update(id, { status: next })
      removeFromCache(id)
    } finally {
      setActing(null)
    }
  }

  const remove = async (id: string) => {
    if (!confirm('Delete this comment permanently?')) return
    setActing(id)
    try {
      await api.comments.delete(id)
      removeFromCache(id)
    } finally {
      setActing(null)
    }
  }

  return (
    <>
      <PageHeader
        title="Comments"
        action={
          status !== 'pending' ? <span className="page-count">{total} total</span> : undefined
        }
      />

      {hasNoSites && (
        <div
          style={{
            background: 'var(--amber-bg)',
            border: '1px solid var(--amber-border)',
            borderRadius: 8,
            padding: '0.75rem 1rem',
            marginBottom: '1.5rem',
            display: 'flex',
            alignItems: 'baseline',
            gap: '0.5rem',
            flexWrap: 'wrap',
          }}
        >
          <span style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--amber)' }}>
            You haven't added a site yet.
          </span>
          <span style={{ fontSize: '0.8125rem', color: 'var(--muted)' }}>
            Add one to get your embed code. Comments appear here once it's on your pages.
          </span>
          <a
            href="/dashboard/onboarding"
            style={{
              fontSize: '0.8125rem',
              fontWeight: 600,
              color: 'var(--amber)',
              textDecoration: 'none',
              whiteSpace: 'nowrap',
            }}
          >
            Add a site &rarr;
          </a>
        </div>
      )}

      <div className="status-tabs">
        {STATUSES.map((s) => (
          <button
            type="button"
            key={s}
            className={status === s ? 'active' : ''}
            onClick={() => handleStatusChange(s)}
          >
            {s.charAt(0).toUpperCase() + s.slice(1)}
          </button>
        ))}
      </div>

      {status === 'pending' ? (
        <ModerationQueue />
      ) : isLoading ? (
        <div className="loading">Loading…</div>
      ) : isError ? (
        <div className="error-msg">Failed to load comments.</div>
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
                {comments.map((c) => (
                  <tr key={c.id}>
                    <td style={{ whiteSpace: 'nowrap', fontWeight: 500 }}>
                      {c.author_name || c.disqus_author || (
                        <span style={{ color: 'var(--muted)' }}>—</span>
                      )}
                    </td>
                    <td style={{ maxWidth: 300, color: 'var(--muted)' }}>
                      {truncate(stripHtml(c.content), 120)}
                    </td>
                    <td
                      style={{
                        maxWidth: 180,
                        wordBreak: 'break-all',
                        color: 'var(--muted)',
                        fontSize: '0.8125rem',
                      }}
                    >
                      {c.page_title || c.page_id}
                    </td>
                    <td
                      style={{ whiteSpace: 'nowrap', color: 'var(--muted)', fontSize: '0.8125rem' }}
                    >
                      {relativeTime(c.created_at)}
                    </td>
                    <td>
                      <div className="actions">
                        {status !== 'approved' && (
                          <button
                            type="button"
                            className="btn btn-approve"
                            disabled={acting === c.id}
                            onClick={() => changeStatus(c.id, 'approved')}
                          >
                            Approve
                          </button>
                        )}
                        {status !== 'rejected' && (
                          <button
                            type="button"
                            className="btn btn-reject"
                            disabled={acting === c.id}
                            onClick={() => changeStatus(c.id, 'rejected')}
                          >
                            Reject
                          </button>
                        )}
                        <button
                          type="button"
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
                type="button"
                className="btn"
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
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
                onClick={() => setPage((p) => p + 1)}
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

export default function CommentsPanel() {
  return (
    <QueryProvider>
      <CommentsPanelInner />
    </QueryProvider>
  )
}
