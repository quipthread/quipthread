// PUBLIC_API_URL is empty in production (same origin) and set to the backend
// URL in local development via dashboard/.env
export const API = (import.meta.env.PUBLIC_API_URL as string | undefined) ?? ''

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    credentials: 'include',
    ...init,
  })
  if (!res.ok) {
    const err = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(err.error ?? `Request failed: ${res.status}`)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

function json(method: string, path: string, body: unknown) {
  return req(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export const api = {
  me: () => req<{ id: string; display_name: string; role: string }>('/api/auth/me'),

  comments: {
    list: (params: {
      status?: string
      flagged?: boolean
      page?: number
      limit?: number
      siteId?: string
    }) => {
      const q = new URLSearchParams()
      if (params.flagged) q.set('status', 'flagged')
      else if (params.status) q.set('status', params.status)
      if (params.page) q.set('page', String(params.page))
      if (params.limit) q.set('limit', String(params.limit))
      if (params.siteId) q.set('siteId', params.siteId)
      return req<{ comments: import('./types').Comment[]; total: number; page: number }>(
        `/api/admin/comments?${q}`,
      )
    },
    update: (id: string, body: { status?: string; content?: string }) =>
      json('PATCH', `/api/admin/comments/${id}`, body),
    delete: (id: string) => req(`/api/admin/comments/${id}`, { method: 'DELETE' }),
    reply: (id: string, content: string) =>
      json('POST', `/api/admin/comments/${id}/reply`, { content }),
  },

  users: {
    list: (params: { page?: number; limit?: number }) => {
      const q = new URLSearchParams()
      if (params.page) q.set('page', String(params.page))
      if (params.limit) q.set('limit', String(params.limit))
      return req<{ users: import('./types').User[]; total: number; page: number }>(
        `/api/admin/users?${q}`,
      )
    },
    update: (id: string, body: { role?: string; banned?: boolean; shadow_banned?: boolean }) =>
      json('PATCH', `/api/admin/users/${id}`, body),
  },

  sites: {
    list: () => req<{ sites: import('./types').Site[] }>('/api/admin/sites'),
    create: (domain: string) => json('POST', '/api/admin/sites', { domain }),
    update: (id: string, body: { theme?: string; notify_interval?: number }) =>
      json('PATCH', `/api/admin/sites/${id}`, body),
    delete: (id: string) => req(`/api/admin/sites/${id}`, { method: 'DELETE' }),
  },

  modrules: {
    list: () => req<{ terms: import('./types').BlockedTerm[] }>('/api/admin/modrules/blocklist'),
    add: (term: string, isRegex = false) =>
      json('POST', '/api/admin/modrules/blocklist', { term, is_regex: isRegex }) as Promise<
        import('./types').BlockedTerm
      >,
    delete: (id: string) => req(`/api/admin/modrules/blocklist/${id}`, { method: 'DELETE' }),
    import: (url: string) =>
      json('POST', '/api/admin/modrules/blocklist/import', { url }) as Promise<{
        added: number
        skipped: number
      }>,
  },

  analytics: {
    get: (siteId: string, range: '7d' | '30d' | 'all') =>
      req<import('./types').AnalyticsData>(
        `/api/admin/analytics?siteId=${encodeURIComponent(siteId)}&range=${range}`,
      ),
  },

  billing: {
    status: () => req<import('./types').BillingStatus>('/api/billing/status'),
    checkout: (plan: string, interval: string) =>
      json('POST', '/api/billing/checkout', { plan, interval }) as Promise<{ url: string }>,
    portal: () => json('POST', '/api/billing/portal', {}) as Promise<{ url: string }>,
  },

  account: {
    get: () => req<import('./types').AccountInfo>('/api/admin/account'),
    updateProfile: (displayName: string) =>
      json('PATCH', '/api/admin/account/profile', { display_name: displayName }),
    updatePassword: (currentPassword: string, newPassword: string) =>
      json('PATCH', '/api/admin/account/password', {
        current_password: currentPassword,
        new_password: newPassword,
      }),
    disconnectIdentity: (provider: string) =>
      req(`/api/admin/account/identity/${provider}`, { method: 'DELETE' }),
    getSecurity: () => req<import('./types').SecuritySettings>('/api/admin/account/security'),
    updateSecurity: (turnstileSiteKey: string, turnstileSecretKey?: string) =>
      json('PATCH', '/api/admin/account/security', {
        turnstile_site_key: turnstileSiteKey,
        ...(turnstileSecretKey !== undefined ? { turnstile_secret_key: turnstileSecretKey } : {}),
      }),
  },

  imports: {
    disqus: (siteId: string, file: File) =>
      multipart<import('./types').ImportResult>('/api/admin/import/disqus', siteId, file),
    wordpress: (siteId: string, file: File) =>
      multipart<import('./types').ImportResult>('/api/admin/import/wordpress', siteId, file),
    remark42: (siteId: string, file: File) =>
      multipart<import('./types').ImportResult>('/api/admin/import/remark42', siteId, file),
    native: (siteId: string, file: File) =>
      multipart<import('./types').ImportResult>('/api/admin/import/native', siteId, file),
    quipthread: (siteId: string, file: File) =>
      multipart<import('./types').ImportResult>('/api/admin/import/quipthread', siteId, file),
    sqliteInspect: (file: File) => {
      const fd = new FormData()
      fd.append('file', file)
      return req<{ tables: import('./types').TableInfo[] }>('/api/admin/import/sqlite/inspect', {
        method: 'POST',
        body: fd,
      })
    },
    sqliteRun: (siteId: string, file: File, mapping: import('./types').ColumnMapping) => {
      const fd = new FormData()
      fd.append('siteId', siteId)
      fd.append('file', file)
      fd.append('mapping', JSON.stringify(mapping))
      return req<import('./types').ImportResult>('/api/admin/import/sqlite/run', {
        method: 'POST',
        body: fd,
      })
    },
  },
}

export function buildExportURL(
  siteId: string,
  format: string,
  opts: { status?: string; from?: string; to?: string; pageId?: string } = {},
): string {
  const base = `${import.meta.env.PUBLIC_API_URL ?? ''}/api/admin/export`
  const params = new URLSearchParams({ siteId, format })
  if (opts.status) params.set('status', opts.status)
  if (opts.from) params.set('from', opts.from)
  if (opts.to) params.set('to', opts.to)
  if (opts.pageId) params.set('pageId', opts.pageId)
  return `${base}?${params}`
}

function multipart<T>(path: string, siteId: string, file: File): Promise<T> {
  const fd = new FormData()
  fd.append('siteId', siteId)
  fd.append('file', file)
  return req<T>(path, { method: 'POST', body: fd })
}
