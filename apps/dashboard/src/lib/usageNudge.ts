import type { BillingStatus } from '../types'

export type UsageNudge = {
  readonly level: 'warning' | 'critical'
  readonly metric: 'comments' | 'sites'
  readonly message: string
  readonly detail: string
  readonly pct?: number
}

export function computeNudge(s: BillingStatus): UsageNudge | null {
  if (s.plan === 'selfhosted') return null
  if (s.comments_limit > 0) {
    const ratio = s.comments_this_month / s.comments_limit
    if (ratio >= 0.8) {
      return {
        level: ratio >= 0.95 ? 'critical' : 'warning',
        metric: 'comments',
        message: ratio >= 1 ? 'Monthly comment limit reached' : 'Approaching comment limit',
        detail: `${s.comments_this_month.toLocaleString()} of ${s.comments_limit.toLocaleString()} comments used.${ratio >= 1 ? ' New comments resume after an upgrade or the next UTC calendar month.' : ''}`,
        pct: Math.min(100, ratio * 100),
      }
    }
  }
  if (s.sites_limit !== null && s.sites_limit > 0 && s.sites_count >= s.sites_limit) {
    return {
      level: 'warning',
      metric: 'sites',
      message: 'Site limit reached',
      detail: `${s.sites_count.toLocaleString()} of ${s.sites_limit.toLocaleString()} sites used. Upgrade to add more sites.`,
    }
  }
  return null
}
