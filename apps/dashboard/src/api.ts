import type { z } from 'zod'
import {
  AccountInfoSchema,
  AnalyticsDataSchema,
  BillingStatusSchema,
  BlockedTermListResponseSchema,
  BlockedTermSchema,
  CheckoutResponseSchema,
  CommentListResponseSchema,
  ImportResultSchema,
  MeResponseSchema,
  ModRulesImportResponseSchema,
  PortalResponseSchema,
  SecuritySettingsSchema,
  SiteListResponseSchema,
  SiteSchema,
  TableListResponseSchema,
  TeamMemberListResponseSchema,
  TeamMemberSchema,
  UserListResponseSchema,
} from './schemas'
import type { ColumnMapping } from './types'

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

// parse fetches a path and validates the response through a Zod schema.
// Use this for all GET endpoints (and mutations) that return a known shape.
async function parse<T>(schema: z.ZodType<T>, path: string, init?: RequestInit): Promise<T> {
  const data = await req<unknown>(path, init)
  return schema.parse(data)
}

function json(method: string, path: string, body: unknown) {
  return req(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

function jsonParse<T>(schema: z.ZodType<T>, method: string, path: string, body: unknown) {
  return parse(schema, path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export const api = {
  me: () => parse(MeResponseSchema, '/api/auth/dashboard/me'),

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
      return parse(CommentListResponseSchema, `/api/admin/comments?${q}`)
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
      return parse(UserListResponseSchema, `/api/admin/users?${q}`)
    },
    update: (id: string, body: { role?: string; banned?: boolean; shadow_banned?: boolean }) =>
      json('PATCH', `/api/admin/users/${id}`, body),
  },

  sites: {
    list: () => parse(SiteListResponseSchema, '/api/admin/sites'),
    create: (domain: string) => jsonParse(SiteSchema, 'POST', '/api/admin/sites', { domain }),
    update: (id: string, body: { theme?: string; notify_interval?: number }) =>
      json('PATCH', `/api/admin/sites/${id}`, body),
    delete: (id: string) => req(`/api/admin/sites/${id}`, { method: 'DELETE' }),
    generateSso: (id: string) =>
      req<{ sso_secret: string }>(`/api/admin/sites/${id}/sso`, { method: 'POST' }),
    disableSso: (id: string) => req(`/api/admin/sites/${id}/sso`, { method: 'DELETE' }),
  },

  modrules: {
    list: () => parse(BlockedTermListResponseSchema, '/api/admin/modrules/blocklist'),
    add: (term: string, isRegex = false) =>
      jsonParse(BlockedTermSchema, 'POST', '/api/admin/modrules/blocklist', {
        term,
        is_regex: isRegex,
      }),
    delete: (id: string) => req(`/api/admin/modrules/blocklist/${id}`, { method: 'DELETE' }),
    import: (url: string) =>
      jsonParse(ModRulesImportResponseSchema, 'POST', '/api/admin/modrules/blocklist/import', {
        url,
      }),
  },

  analytics: {
    get: (siteId: string, range: '7d' | '30d' | 'all') =>
      parse(
        AnalyticsDataSchema,
        `/api/admin/analytics?siteId=${encodeURIComponent(siteId)}&range=${range}`,
      ),
  },

  billing: {
    status: () => parse(BillingStatusSchema, '/api/billing/status'),
    checkout: (plan: string, interval: string) =>
      jsonParse(CheckoutResponseSchema, 'POST', '/api/billing/checkout', { plan, interval }),
    portal: () => jsonParse(PortalResponseSchema, 'POST', '/api/billing/portal', {}),
  },

  account: {
    get: () => parse(AccountInfoSchema, '/api/admin/account'),
    updateProfile: (displayName: string) =>
      json('PATCH', '/api/admin/account/profile', { display_name: displayName }),
    updatePassword: (currentPassword: string, newPassword: string) =>
      json('PATCH', '/api/admin/account/password', {
        current_password: currentPassword,
        new_password: newPassword,
      }),
    disconnectIdentity: (provider: string) =>
      req(`/api/admin/account/identity/${provider}`, { method: 'DELETE' }),
    getSecurity: () => parse(SecuritySettingsSchema, '/api/admin/account/security'),
    updateSecurity: (turnstileSiteKey: string, turnstileSecretKey?: string) =>
      json('PATCH', '/api/admin/account/security', {
        turnstile_site_key: turnstileSiteKey,
        ...(turnstileSecretKey !== undefined ? { turnstile_secret_key: turnstileSecretKey } : {}),
      }),
  },

  invitations: {
    list: () => parse(TeamMemberListResponseSchema, '/api/admin/invitations'),
    create: (email: string) =>
      jsonParse(TeamMemberSchema, 'POST', '/api/admin/invitations', { email }),
    delete: (id: string) => req(`/api/admin/invitations/${id}`, { method: 'DELETE' }),
  },

  imports: {
    disqus: (siteId: string, file: File) =>
      multipartParse(ImportResultSchema, '/api/admin/import/disqus', siteId, file),
    wordpress: (siteId: string, file: File) =>
      multipartParse(ImportResultSchema, '/api/admin/import/wordpress', siteId, file),
    remark42: (siteId: string, file: File) =>
      multipartParse(ImportResultSchema, '/api/admin/import/remark42', siteId, file),
    native: (siteId: string, file: File) =>
      multipartParse(ImportResultSchema, '/api/admin/import/native', siteId, file),
    quipthread: (siteId: string, file: File) =>
      multipartParse(ImportResultSchema, '/api/admin/import/quipthread', siteId, file),
    sqliteInspect: (file: File) => {
      const fd = new FormData()
      fd.append('file', file)
      return parse(TableListResponseSchema, '/api/admin/import/sqlite/inspect', {
        method: 'POST',
        body: fd,
      })
    },
    sqliteRun: (siteId: string, file: File, mapping: ColumnMapping) => {
      const fd = new FormData()
      fd.append('siteId', siteId)
      fd.append('file', file)
      fd.append('mapping', JSON.stringify(mapping))
      return parse(ImportResultSchema, '/api/admin/import/sqlite/run', {
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

function multipartParse<T>(
  schema: z.ZodType<T>,
  path: string,
  siteId: string,
  file: File,
): Promise<T> {
  const fd = new FormData()
  fd.append('siteId', siteId)
  fd.append('file', file)
  return parse(schema, path, { method: 'POST', body: fd })
}
