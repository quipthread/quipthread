import { useRef, useState, useEffect } from 'react'
import { Editor, type EditorRef } from './Editor'
import { createComment } from './api'
import type { Comment, CreateCommentInput } from './types'
import type { useTranslations } from './i18n'

interface CommentFormProps {
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  parentId?: string
  placeholder?: string
  t: ReturnType<typeof useTranslations>
  onSuccess: (comment: Comment) => void
  onCancel?: () => void
  turnstileSiteKey?: string
}

function draftKey(siteId: string, pageId: string, parentId?: string): string {
  return `qt-draft:${siteId}:${pageId}${parentId ? `:${parentId}` : ''}`
}

export function CommentForm({
  siteId,
  pageId,
  pageUrl,
  pageTitle,
  parentId,
  placeholder,
  t,
  onSuccess,
  onCancel,
  turnstileSiteKey,
}: CommentFormProps) {
  const editorRef = useRef<EditorRef>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pendingNotice, setPendingNotice] = useState(false)
  const key = draftKey(siteId, pageId, parentId)

  // Turnstile widget state.
  const turnstileContainerRef = useRef<HTMLDivElement>(null)
  const turnstileWidgetId = useRef<string | null>(null)
  const turnstileToken = useRef<string | null>(null)

  useEffect(() => {
    if (!turnstileSiteKey || !turnstileContainerRef.current || !window.turnstile) return

    turnstileWidgetId.current = window.turnstile.render(turnstileContainerRef.current, {
      sitekey: turnstileSiteKey,
      callback: (token) => { turnstileToken.current = token },
      'expired-callback': () => { turnstileToken.current = null },
      'error-callback': () => { turnstileToken.current = null },
    })

    return () => {
      if (turnstileWidgetId.current !== null) {
        window.turnstile?.remove(turnstileWidgetId.current)
        turnstileWidgetId.current = null
      }
    }
  }, [turnstileSiteKey])

  const initialContent = (() => {
    try {
      return localStorage.getItem(key) ?? ''
    } catch {
      return ''
    }
  })()

  // Persist draft on unmount if content remains
  useEffect(() => {
    return () => {
      const html = editorRef.current?.getHTML() ?? ''
      const isEmpty = editorRef.current?.isEmpty() ?? true
      try {
        if (isEmpty) {
          localStorage.removeItem(key)
        } else {
          localStorage.setItem(key, html)
        }
      } catch {
        // localStorage unavailable (cross-origin iframe, private browsing, etc.)
      }
    }
  }, [key])

  const handleChange = (_html: string, isEmpty: boolean) => {
    if (isEmpty) {
      try { localStorage.removeItem(key) } catch { /* noop */ }
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const html = editorRef.current?.getHTML() ?? ''
    const isEmpty = editorRef.current?.isEmpty() ?? true
    if (isEmpty) return

    setSubmitting(true)
    setError(null)
    setPendingNotice(false)

    try {
      const input: CreateCommentInput = {
        site_id: siteId,
        page_id: pageId,
        content: html,
        ...(pageUrl ? { page_url: pageUrl } : {}),
        ...(pageTitle ? { page_title: pageTitle } : {}),
        ...(parentId ? { parent_id: parentId } : {}),
        ...(turnstileToken.current ? { turnstile_token: turnstileToken.current } : {}),
      }
      const comment = await createComment(input)
      try { localStorage.removeItem(key) } catch { /* noop */ }
      editorRef.current?.clear()

      // Reset the Turnstile widget so the next submission gets a fresh token.
      if (turnstileWidgetId.current !== null) {
        window.turnstile?.reset(turnstileWidgetId.current)
        turnstileToken.current = null
      }

      if (comment.status !== 'approved') {
        setPendingNotice(true)
      } else {
        onSuccess(comment)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t.submitError)
    } finally {
      setSubmitting(false)
    }
  }

  if (pendingNotice) {
    return (
      <div className="qt-pending-notice">
        {t.awaitingApproval}
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit}>
      <Editor
        ref={editorRef}
        placeholder={placeholder ?? t.leaveComment}
        initialContent={initialContent}
        onChange={handleChange}
      />
      {error && <p className="qt-error">{error}</p>}
      {turnstileSiteKey && (
        <div ref={turnstileContainerRef} style={{ height: 0, overflow: 'hidden' }} />
      )}
      <div className="qt-form-actions">
        {onCancel && (
          <button type="button" className="qt-btn qt-btn-secondary" onClick={onCancel}>
            {t.cancel}
          </button>
        )}
        <button type="submit" className="qt-btn qt-btn-primary" disabled={submitting}>
          {submitting ? t.submitting : t.submit}
        </button>
      </div>
    </form>
  )
}
