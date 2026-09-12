import { useState } from 'react'
import { deleteComment } from './api'
import { CommentForm } from './CommentForm'
import type { useTranslations } from './i18n'
import type { Comment, User } from './types'

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return new Date(dateStr).toLocaleDateString()
}

interface AvatarProps {
  src?: string
  name: string
  size?: 'normal' | 'small'
}

function Avatar({ src, name, size = 'normal' }: AvatarProps) {
  const initial = name.charAt(0) || '?'
  const avatarClass = size === 'small' ? 'qt-reply-avatar' : 'qt-avatar'
  const placeholderClass =
    size === 'small' ? 'qt-reply-avatar-placeholder' : 'qt-avatar-placeholder'

  if (src) {
    return (
      <img
        src={src}
        alt={name}
        className={avatarClass}
        onError={(e) => {
          const el = e.currentTarget
          el.style.display = 'none'
          const sibling = el.nextElementSibling as HTMLElement | null
          if (sibling) sibling.style.display = 'flex'
        }}
      />
    )
  }
  return <div className={placeholderClass}>{initial}</div>
}

interface CommentBodyProps {
  comment: Comment
  user: User | null
  t: ReturnType<typeof useTranslations>
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  onDelete?: (id: string) => void
  onReplySuccess?: (comment: Comment) => void
  onVote?: (id: string) => Promise<void>
  onFlag?: (id: string) => Promise<void>
  showReply?: boolean
}

function CommentBody({
  comment,
  user,
  t,
  siteId,
  pageId,
  pageUrl,
  pageTitle,
  onDelete,
  onReplySuccess,
  onVote,
  onFlag,
  showReply = true,
}: CommentBodyProps) {
  const [showReplyForm, setShowReplyForm] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)

  const authorName = comment.author_name || comment.disqus_author || 'Anonymous'

  const handleDelete = async () => {
    if (!window.confirm('Delete this comment?')) return
    setMutationError(null)
    try {
      await deleteComment(comment.id, siteId)
      onDelete?.(comment.id)
    } catch {
      setMutationError('Unable to delete comment. Try again.')
    }
  }

  const handleVote = async () => {
    if (!onVote) return
    setMutationError(null)
    try {
      await onVote(comment.id)
    } catch {
      setMutationError('Unable to update vote. Try again.')
    }
  }

  const handleFlag = async () => {
    if (!onFlag) return
    setMutationError(null)
    try {
      await onFlag(comment.id)
    } catch {
      setMutationError('Unable to update flag. Try again.')
    }
  }

  const isOwner = user?.id === comment.user_id

  return (
    <>
      <div className="qt-comment-meta">
        <span className="qt-author">{authorName}</span>
        <span className="qt-timestamp">{relativeTime(comment.created_at)}</span>
      </div>
      <div
        className="qt-comment-content"
        // Content is Tiptap HTML (safe subset) approved before display; Link extension
        // rejects non-http(s) hrefs at authoring time.
        // biome-ignore lint/security/noDangerouslySetInnerHtml: intentional — server-approved Tiptap HTML
        dangerouslySetInnerHTML={{ __html: comment.content }}
      />
      <div className="qt-comment-actions">
        <button
          type="button"
          className={`qt-btn qt-vote-btn${comment.user_voted ? ' voted' : ''}`}
          onClick={handleVote}
          title={user ? (comment.user_voted ? 'Remove upvote' : 'Upvote') : 'Sign in to vote'}
          aria-pressed={comment.user_voted}
        >
          <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d="M12 4l8 16H4z" />
          </svg>
          {comment.upvotes > 0 && <span className="qt-vote-count">{comment.upvotes}</span>}
        </button>
        {showReply && user && !showReplyForm && (
          <button
            type="button"
            className="qt-btn qt-btn-ghost"
            onClick={() => setShowReplyForm(true)}
          >
            {t.reply}
          </button>
        )}
        {isOwner && (
          <button type="button" className="qt-btn qt-btn-danger-ghost" onClick={handleDelete}>
            {t.deleteComment}
          </button>
        )}
        {user && !isOwner && (
          <button
            type="button"
            className={`qt-btn qt-flag-btn${comment.user_flagged ? ' flagged' : ''}`}
            onClick={handleFlag}
            title={comment.user_flagged ? 'Remove flag' : 'Flag this comment'}
            aria-pressed={comment.user_flagged}
          >
            {comment.user_flagged ? t.flagged : t.flag}
          </button>
        )}
      </div>
      {mutationError && (
        <p className="qt-mutation-error" role="status">
          {mutationError}
        </p>
      )}
      {showReplyForm && (
        <div className="qt-reply-form">
          <CommentForm
            siteId={siteId}
            pageId={pageId}
            pageUrl={pageUrl}
            pageTitle={pageTitle}
            parentId={comment.id}
            t={t}
            onSuccess={(c) => {
              setShowReplyForm(false)
              onReplySuccess?.(c)
            }}
            onCancel={() => setShowReplyForm(false)}
          />
        </div>
      )}
    </>
  )
}

interface CommentItemProps {
  comment: Comment
  replies: Comment[]
  user: User | null
  t: ReturnType<typeof useTranslations>
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  onDelete: (id: string) => void
  onReplySuccess: (comment: Comment) => void
  onVote?: (id: string) => Promise<void>
  onFlag?: (id: string) => Promise<void>
}

export function CommentItem({
  comment,
  replies,
  user,
  t,
  siteId,
  pageId,
  pageUrl,
  pageTitle,
  onDelete,
  onReplySuccess,
  onVote,
  onFlag,
}: CommentItemProps) {
  const authorName = comment.author_name || comment.disqus_author || 'Anonymous'
  const avatarSrc = comment.author_avatar || undefined

  return (
    <li className="qt-comment">
      <Avatar src={avatarSrc} name={authorName} />
      <div className="qt-comment-body">
        <CommentBody
          comment={comment}
          user={user}
          t={t}
          siteId={siteId}
          pageId={pageId}
          pageUrl={pageUrl}
          pageTitle={pageTitle}
          onDelete={onDelete}
          onReplySuccess={onReplySuccess}
          onVote={onVote}
          onFlag={onFlag}
        />
        {replies.length > 0 && (
          <div className="qt-replies">
            {replies.map((reply) => {
              const replyAuthor = reply.author_name || reply.disqus_author || 'Anonymous'
              const replyAvatar = reply.author_avatar || undefined
              return (
                <div key={reply.id} className="qt-reply-comment">
                  <Avatar src={replyAvatar} name={replyAuthor} size="small" />
                  <div className="qt-comment-body">
                    <CommentBody
                      comment={reply}
                      user={user}
                      t={t}
                      siteId={siteId}
                      pageId={pageId}
                      pageUrl={pageUrl}
                      pageTitle={pageTitle}
                      onDelete={onDelete}
                      onReplySuccess={onReplySuccess}
                      onVote={onVote}
                      onFlag={onFlag}
                      showReply={false}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </li>
  )
}
