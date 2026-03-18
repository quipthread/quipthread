import { useState } from 'react'
import { deleteComment } from './api'
import { CommentForm } from './CommentForm'
import type { Comment, User } from './types'
import type { useTranslations } from './i18n'

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
  size?: 'normal' | 'small'
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  onDelete?: (id: string) => void
  onReplySuccess?: (comment: Comment) => void
  showReply?: boolean
}

function CommentBody({
  comment,
  user,
  t,
  size = 'normal',
  siteId,
  pageId,
  pageUrl,
  pageTitle,
  onDelete,
  onReplySuccess,
  showReply = true,
}: CommentBodyProps) {
  const [showReplyForm, setShowReplyForm] = useState(false)

  const authorName = comment.author_name || comment.disqus_author || 'Anonymous'
  const avatarSrc = comment.author_avatar || undefined

  const handleDelete = async () => {
    if (!window.confirm('Delete this comment?')) return
    try {
      await deleteComment(comment.id)
      onDelete?.(comment.id)
    } catch {
      // Silently fail — the comment stays visible
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
        // Content is Tiptap HTML (safe subset) and must be approved before display.
        // The Link extension rejects non-http(s) hrefs at authoring time.
        dangerouslySetInnerHTML={{ __html: comment.content }}
      />
      <div className="qt-comment-actions">
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
          <button
            type="button"
            className="qt-btn qt-btn-danger-ghost"
            onClick={handleDelete}
          >
            {t.deleteComment}
          </button>
        )}
      </div>
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
