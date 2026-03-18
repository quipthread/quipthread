import type { Comment, CommentsResponse, CreateCommentInput, User, WidgetConfig } from './types'

let apiBase = ''

export function initApi(base: string) {
  apiBase = base
}

export function getApiBase(): string {
  return apiBase
}

export async function fetchConfig(siteId: string): Promise<WidgetConfig> {
  try {
    const res = await fetch(
      `${apiBase}/api/config?siteId=${encodeURIComponent(siteId)}`,
      { credentials: 'include' },
    )
    if (!res.ok) return { turnstileSiteKey: '', theme: 'auto' }
    return res.json()
  } catch {
    return { turnstileSiteKey: '', theme: 'auto' }
  }
}

export async function getMe(): Promise<User | null> {
  try {
    const res = await fetch(`${apiBase}/api/auth/me`, { credentials: 'include' })
    if (!res.ok) return null
    return res.json()
  } catch {
    return null
  }
}

export async function listComments(
  siteId: string,
  pageId: string,
  page: number,
  limit: number,
): Promise<CommentsResponse> {
  const params = new URLSearchParams({
    siteId,
    pageId,
    page: String(page),
    limit: String(limit),
  })
  const res = await fetch(`${apiBase}/api/comments?${params}`, { credentials: 'include' })
  if (!res.ok) throw new Error('Failed to load comments')
  return res.json()
}

export async function createComment(data: CreateCommentInput): Promise<Comment> {
  const res = await fetch(`${apiBase}/api/comments`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(err.error ?? 'Failed to create comment')
  }
  return res.json()
}

export async function deleteComment(id: string): Promise<void> {
  await fetch(`${apiBase}/api/comments/${id}`, {
    method: 'DELETE',
    credentials: 'include',
  })
}
