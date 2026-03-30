export interface User {
  id: string
  display_name: string
  email: string
  avatar_url: string
  role: string
  banned: boolean
  shadow_banned: boolean
  created_at: string
}

export interface Comment {
  id: string
  site_id: string
  page_id: string
  page_url: string
  page_title: string
  parent_id: string
  user_id: string
  content: string
  status: string
  author_name: string
  author_avatar: string
  disqus_author: string
  flags?: number
  created_at: string
  updated_at: string
}

export interface Site {
  id: string
  owner_id: string
  domain: string
  theme: string
  notify_interval?: number | null
  created_at: string
}

export interface Paginated<T> {
  comments?: T[]
  users?: T[]
  total: number
  page: number
  limit: number
}

export interface ColumnInfo {
  name: string
  type: string
  samples: string[]
}

export interface TableInfo {
  name: string
  columns: ColumnInfo[]
}

export interface ImportResult {
  users_inserted: number
  comments_inserted: number
}

export interface BillingStatus {
  plan: 'hobby' | 'starter' | 'pro' | 'business'
  status: 'active' | 'trialing' | 'past_due' | 'canceled'
  trial_ends_at: string | null
  current_period_end: string | null
  interval: string
  trial_eligible: boolean
  comments_this_month: number
  comments_limit: number
  sites_count: number
  sites_limit: number | null
}

export interface BlockedTerm {
  id: string
  term: string
  is_regex: boolean
  created_at: string
}

export interface VolumePoint {
  date: string
  count: number
}

export interface PageStat {
  page_id: string
  page_title: string
  count: number
}

export interface CommenterStat {
  display_name: string
  count: number
}

export interface StatusStat {
  status: string
  count: number
}

export interface PeakHourStat {
  hour: number
  count: number
}

export interface PeakDayStat {
  day: number
  count: number
}

export interface AnalyticsData {
  // Starter+
  volume: VolumePoint[]
  pages: PageStat[]
  commenters: CommenterStat[]
  // Pro+
  status_breakdown?: StatusStat[]
  peak_hours?: PeakHourStat[]
  peak_days?: PeakDayStat[]
  // Business+
  return_rate?: number
}

export interface ColumnMapping {
  table: string
  columns: Record<string, string>
  strip_domain: boolean
  wrap_in_p: boolean
}

export interface AccountInfo {
  id: string
  display_name: string
  email: string
  avatar_url: string
  providers: string[] // ['github', 'google', 'email']
  provider_usernames: Record<string, string> // e.g. { github: 'frankie' }
  configured_providers: string[] // providers with OAuth credentials configured on the server
}

export interface SecuritySettings {
  turnstile_site_key: string
  has_turnstile_secret: boolean
}

export interface TeamMember {
  id: string
  account_id: string
  email: string
  role: string
  invite_token: string
  accepted: boolean
  invited_at: string
  accepted_at: string | null
}
