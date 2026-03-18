import { useState, useEffect, useCallback } from 'react'
import { listComments } from './api'
import { CommentItem } from './CommentItem'
import type { Comment, User } from './types'
import type { useTranslations } from './i18n'

const PAGE_SIZE = 20

interface TreeNode {
  comment: Comment
  replies: Comment[]
}

function buildTree(comments: Comment[]): TreeNode[] {
  const byId = new Map<string, Comment>()
  for (const c of comments) byId.set(c.id, c)

  // Walk up to find the top-level ancestor of a comment
  const rootId = (c: Comment): string => {
    let current = c
    while (current.parent_id && byId.has(current.parent_id)) {
      current = byId.get(current.parent_id)!
    }
    return current.id
  }

  const roots: Comment[] = []
  const replyMap = new Map<string, Comment[]>()

  for (const c of comments) {
    if (!c.parent_id) {
      roots.push(c)
    } else {
      const ancestor = rootId(c)
      const bucket = replyMap.get(ancestor) ?? []
      bucket.push(c)
      replyMap.set(ancestor, bucket)
    }
  }

  return roots.map((c) => ({ comment: c, replies: replyMap.get(c.id) ?? [] }))
}

interface CommentListProps {
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  user: User | null
  t: ReturnType<typeof useTranslations>
  refreshKey: number
}

export function CommentList({
  siteId,
  pageId,
  pageUrl,
  pageTitle,
  user,
  t,
  refreshKey,
}: CommentListProps) {
  const [comments, setComments] = useState<Comment[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchComments = useCallback(
    async (p: number) => {
      setLoading(true)
      setError(null)
      try {
        const res = await listComments(siteId, pageId, p, PAGE_SIZE)
        setComments(res.comments ?? [])
        setTotal(res.total)
        setPage(p)
      } catch {
        setError(t.loadError)
      } finally {
        setLoading(false)
      }
    },
    [siteId, pageId, t.loadError],
  )

  useEffect(() => {
    fetchComments(1)
  }, [fetchComments, refreshKey])

  const handleDelete = (id: string) => {
    setComments((prev) => prev.filter((c) => c.id !== id))
    setTotal((prev) => Math.max(0, prev - 1))
  }

  const handleReplySuccess = (comment: Comment) => {
    setComments((prev) => [...prev, comment])
    setTotal((prev) => prev + 1)
  }

  if (loading) return <div className="qt-loading">Loading…</div>
  if (error) return <div className="qt-error">{error}</div>

  const tree = buildTree(comments)
  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <>
      {tree.length === 0 ? (
        <div className="qt-empty">{t.noComments}</div>
      ) : (
        <ul className="qt-comment-list">
          {tree.map(({ comment, replies }) => (
            <CommentItem
              key={comment.id}
              comment={comment}
              replies={replies}
              user={user}
              t={t}
              siteId={siteId}
              pageId={pageId}
              pageUrl={pageUrl}
              pageTitle={pageTitle}
              onDelete={handleDelete}
              onReplySuccess={handleReplySuccess}
            />
          ))}
        </ul>
      )}
      {totalPages > 1 && (
        <div className="qt-pagination">
          <button
            className="qt-btn qt-btn-secondary"
            disabled={page <= 1}
            onClick={() => fetchComments(page - 1)}
          >
            ←
          </button>
          <span style={{ fontSize: '0.875rem', color: 'var(--qt-text-muted)' }}>
            {page} / {totalPages}
          </span>
          <button
            className="qt-btn qt-btn-secondary"
            disabled={page >= totalPages}
            onClick={() => fetchComments(page + 1)}
          >
            →
          </button>
        </div>
      )}
    </>
  )
}
