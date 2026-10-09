import type {
  Comment,
  CommentsResponse,
  CreateCommentInput,
  SortOrder,
  User,
  WidgetConfig,
} from './types'

let apiBase = ''

export function initApi(base: string) {
  apiBase = base
}

// Embed sessions are site-bound bearer tokens kept in the publisher page's
// localStorage, one per site. The API never uses cookies for the embed, so
// sign-in works where third-party cookies are blocked (Safari, Firefox).
const memoryTokens = new Map<string, string>()

function tokenKey(siteId: string): string {
  return `qt-token:${siteId}`
}

function getToken(siteId: string): string | null {
  try {
    return localStorage.getItem(tokenKey(siteId))
  } catch {
    return null
  }
}

export function setToken(siteId: string, token: string) {
  try {
    localStorage.setItem(tokenKey(siteId), token)
  } catch {
    // Storage unavailable (private mode, blocked site data): the session
    // lasts only until the next page load.
    memoryTokens.set(siteId, token)
  }
}

export function clearToken(siteId: string) {
  memoryTokens.delete(siteId)
  try {
    localStorage.removeItem(tokenKey(siteId))
  } catch {
    // Nothing to clear.
  }
}

function authHeaders(siteId: string): Record<string, string> {
  const token = getToken(siteId) ?? memoryTokens.get(siteId)
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export class UnauthorizedError extends Error {
  constructor() {
    super('unauthorized')
  }
}

// Requests with a token that the server rejects drop it so the widget falls
// back to the signed-out state instead of retrying a dead session.
function checkAuth(res: Response, siteId: string) {
  if (res.status === 401) {
    clearToken(siteId)
    throw new UnauthorizedError()
  }
}

export type AuthResult = { token: string } | { error: string } | null

// consumeAuthFragment reads the OAuth result the server appended to this
// page's URL (#qt_auth=<token> or #qt_auth_error=<code>) and removes it from
// the address bar and history entry immediately.
export function consumeAuthFragment(): AuthResult {
  const hash = window.location.hash
  if (!hash.startsWith('#qt_auth')) return null
  const params = new URLSearchParams(hash.slice(1))
  const token = params.get('qt_auth')
  const error = params.get('qt_auth_error')
  try {
    history.replaceState(history.state, '', window.location.pathname + window.location.search)
  } catch {
    window.location.hash = ''
  }
  if (token) return { token }
  if (error) return { error }
  return null
}

export function loginUrl(provider: 'github' | 'google', siteId: string): string {
  const params = new URLSearchParams({
    audience: 'embed',
    siteId,
    returnTo: window.location.origin + window.location.pathname + window.location.search,
  })
  return `${apiBase}/auth/${provider}/login?${params}`
}

export async function fetchConfig(siteId: string): Promise<WidgetConfig> {
  try {
    const res = await fetch(`${apiBase}/api/config?siteId=${encodeURIComponent(siteId)}`)
    if (!res.ok) return { turnstileSiteKey: '', theme: 'auto' }
    return res.json()
  } catch {
    return { turnstileSiteKey: '', theme: 'auto' }
  }
}

export async function getMe(siteId: string): Promise<User | null> {
  if (!getToken(siteId) && !memoryTokens.has(siteId)) return null
  try {
    const res = await fetch(`${apiBase}/api/auth/embed/me?siteId=${encodeURIComponent(siteId)}`, {
      headers: authHeaders(siteId),
    })
    if (res.status === 401) {
      clearToken(siteId)
      return null
    }
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
  sort: SortOrder = 'newest',
): Promise<CommentsResponse> {
  const params = new URLSearchParams({
    siteId,
    pageId,
    page: String(page),
    limit: String(limit),
    sort,
  })
  const res = await fetch(`${apiBase}/api/comments?${params}`, { headers: authHeaders(siteId) })
  if (!res.ok) throw new Error('Failed to load comments')
  return res.json()
}

export async function toggleVote(
  commentId: string,
  siteId: string,
): Promise<{ upvotes: number; user_voted: boolean }> {
  const res = await fetch(
    `${apiBase}/api/comments/${commentId}/vote?siteId=${encodeURIComponent(siteId)}`,
    { method: 'POST', headers: authHeaders(siteId) },
  )
  checkAuth(res, siteId)
  if (!res.ok) throw new Error('Failed to vote')
  return res.json()
}

export async function flagComment(
  commentId: string,
  siteId: string,
): Promise<{ user_flagged: boolean }> {
  const res = await fetch(
    `${apiBase}/api/comments/${commentId}/flag?siteId=${encodeURIComponent(siteId)}`,
    { method: 'POST', headers: authHeaders(siteId) },
  )
  checkAuth(res, siteId)
  if (!res.ok) throw new Error('Failed to flag comment')
  return res.json()
}

export async function createComment(data: CreateCommentInput): Promise<Comment> {
  const res = await fetch(`${apiBase}/api/comments?siteId=${encodeURIComponent(data.site_id)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders(data.site_id) },
    body: JSON.stringify(data),
  })
  checkAuth(res, data.site_id)
  if (!res.ok) {
    const err = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(err.error ?? 'Failed to create comment')
  }
  return res.json()
}

export async function deleteComment(id: string, siteId: string): Promise<void> {
  const res = await fetch(`${apiBase}/api/comments/${id}?siteId=${encodeURIComponent(siteId)}`, {
    method: 'DELETE',
    headers: authHeaders(siteId),
  })
  checkAuth(res, siteId)
  if (!res.ok) throw new Error('Failed to delete comment')
}

export async function ssoLogin(siteId: string, ssoToken: string): Promise<User | null> {
  try {
    const res = await fetch(`${apiBase}/api/auth/sso`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ site_id: siteId, token: ssoToken }),
    })
    if (!res.ok) return null
    const { token, ...user } = (await res.json()) as User & { token: string }
    setToken(siteId, token)
    return user
  } catch {
    return null
  }
}
