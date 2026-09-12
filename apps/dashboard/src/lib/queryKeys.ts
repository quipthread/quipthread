// Typed query key factories. Centralising keys here prevents string drift
// and ensures components sharing a query always reference the same cache entry.

type CommentsParams = {
  status?: string
  flagged?: boolean
  page: number
  limit: number
  siteId?: string
}

type UsersParams = { page: number; limit: number }

export const queryKeys = {
  billingStatus: () => ['billing', 'status'] as const,
  sites: () => ['sites'] as const,
  me: () => ['me'] as const,
  allComments: () => ['comments'] as const,
  comments: (params: CommentsParams) => ['comments', params] as const,
  users: (params: UsersParams) => ['users', params] as const,
  analytics: (siteId: string, range: string) => ['analytics', siteId, range] as const,
  modrules: () => ['modrules'] as const,
  account: () => ['account'] as const,
  security: () => ['security'] as const,
  invitations: () => ['invitations'] as const,
}
