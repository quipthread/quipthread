import { z } from 'zod'

// ---------------------------------------------------------------------------
// Core entities
// ---------------------------------------------------------------------------

export const UserSchema = z.object({
  id: z.string(),
  display_name: z.string(),
  email: z.string().default(''),
  avatar_url: z.string().default(''),
  role: z.string(),
  banned: z.boolean(),
  shadow_banned: z.boolean(),
  created_at: z.string(),
})
export type User = z.infer<typeof UserSchema>

export const CommentSchema = z.object({
  id: z.string(),
  site_id: z.string(),
  page_id: z.string(),
  page_url: z.string().default(''),
  page_title: z.string().default(''),
  parent_id: z.string().default(''),
  user_id: z.string(),
  content: z.string(),
  status: z.string(),
  author_name: z.string().default(''),
  author_avatar: z.string().default(''),
  disqus_author: z.string().default(''),
  flags: z.number().optional(),
  created_at: z.string(),
  updated_at: z.string(),
})
export type Comment = z.infer<typeof CommentSchema>

export const SiteSchema = z.object({
  id: z.string(),
  owner_id: z.string(),
  domain: z.string(),
  theme: z.string(),
  notify_interval: z.number().nullable().optional(),
  created_at: z.string(),
  sso_enabled: z.boolean(),
})
export type Site = z.infer<typeof SiteSchema>

export const BlockedTermSchema = z.object({
  id: z.string(),
  term: z.string(),
  is_regex: z.boolean(),
  created_at: z.string(),
})
export type BlockedTerm = z.infer<typeof BlockedTermSchema>

export const TeamMemberSchema = z.object({
  id: z.string(),
  account_id: z.string(),
  email: z.string(),
  role: z.string(),
  invite_token: z.string(),
  accepted: z.boolean(),
  invited_at: z.string(),
  accepted_at: z.string().nullable(),
})
export type TeamMember = z.infer<typeof TeamMemberSchema>

export const AccountInfoSchema = z.object({
  id: z.string(),
  display_name: z.string(),
  email: z.string(),
  avatar_url: z.string(),
  providers: z.array(z.string()),
  provider_usernames: z.record(z.string(), z.string()),
  configured_providers: z.array(z.string()),
})
export type AccountInfo = z.infer<typeof AccountInfoSchema>

export const SecuritySettingsSchema = z.object({
  turnstile_site_key: z.string(),
  has_turnstile_secret: z.boolean(),
})
export type SecuritySettings = z.infer<typeof SecuritySettingsSchema>

// plan includes 'enterprise' and 'selfhosted' which the existing types.ts was missing.
export const BillingStatusSchema = z.object({
  plan: z.enum(['hobby', 'starter', 'pro', 'business', 'enterprise', 'selfhosted']),
  status: z.enum(['active', 'trialing', 'past_due', 'canceled', '']),
  trial_ends_at: z.string().nullable(),
  current_period_end: z.string().nullable(),
  interval: z.string(),
  trial_eligible: z.boolean().default(false),
  comments_this_month: z.number(),
  comments_limit: z.number(),
  sites_count: z.number(),
  sites_limit: z.number().nullable(),
})
export type BillingStatus = z.infer<typeof BillingStatusSchema>

export const ImportResultSchema = z.object({
  users_inserted: z.number(),
  comments_inserted: z.number(),
})
export type ImportResult = z.infer<typeof ImportResultSchema>

export const ColumnInfoSchema = z.object({
  name: z.string(),
  type: z.string(),
  samples: z.array(z.string()),
})
export type ColumnInfo = z.infer<typeof ColumnInfoSchema>

export const TableInfoSchema = z.object({
  name: z.string(),
  columns: z.array(ColumnInfoSchema),
})
export type TableInfo = z.infer<typeof TableInfoSchema>

// ---------------------------------------------------------------------------
// Analytics
// ---------------------------------------------------------------------------

export const VolumePointSchema = z.object({ date: z.string(), count: z.number() })
export type VolumePoint = z.infer<typeof VolumePointSchema>

export const PageStatSchema = z.object({
  page_id: z.string(),
  page_title: z.string(),
  count: z.number(),
})
export type PageStat = z.infer<typeof PageStatSchema>

export const CommenterStatSchema = z.object({ display_name: z.string(), count: z.number() })
export type CommenterStat = z.infer<typeof CommenterStatSchema>

export const StatusStatSchema = z.object({ status: z.string(), count: z.number() })
export type StatusStat = z.infer<typeof StatusStatSchema>

export const PeakHourStatSchema = z.object({ hour: z.number(), count: z.number() })
export type PeakHourStat = z.infer<typeof PeakHourStatSchema>

export const PeakDayStatSchema = z.object({ day: z.number(), count: z.number() })
export type PeakDayStat = z.infer<typeof PeakDayStatSchema>

export const AnalyticsDataSchema = z.object({
  volume: z.array(VolumePointSchema),
  pages: z.array(PageStatSchema),
  commenters: z.array(CommenterStatSchema),
  status_breakdown: z.array(StatusStatSchema).optional(),
  peak_hours: z.array(PeakHourStatSchema).optional(),
  peak_days: z.array(PeakDayStatSchema).optional(),
  return_rate: z.number().optional(),
})
export type AnalyticsData = z.infer<typeof AnalyticsDataSchema>

// ---------------------------------------------------------------------------
// API response envelopes
// ---------------------------------------------------------------------------

export const MeResponseSchema = z.object({
  id: z.string(),
  display_name: z.string(),
  role: z.string(),
})

export const CommentListResponseSchema = z.object({
  comments: z.array(CommentSchema),
  total: z.number(),
  page: z.number(),
})

export const UserListResponseSchema = z.object({
  users: z.array(UserSchema),
  total: z.number(),
  page: z.number(),
})

export const SiteListResponseSchema = z.object({
  sites: z.array(SiteSchema),
})

export const BlockedTermListResponseSchema = z.object({
  terms: z.array(BlockedTermSchema),
})

export const ModRulesImportResponseSchema = z.object({
  added: z.number(),
  skipped: z.number(),
})

export const TeamMemberListResponseSchema = z.object({
  members: z.array(TeamMemberSchema),
})

export const TableListResponseSchema = z.object({
  tables: z.array(TableInfoSchema),
})

export const CheckoutResponseSchema = z.object({ url: z.string() })
export const PortalResponseSchema = z.object({ url: z.string() })
