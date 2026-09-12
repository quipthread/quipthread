import { describe, expect, test } from 'bun:test'
import { computeNudge } from '../src/lib/usageNudge'
import { BillingStatusSchema } from '../src/schemas'

const status = BillingStatusSchema.parse({
  plan: 'pro',
  status: 'active',
  trial_ends_at: null,
  current_period_end: null,
  interval: 'month',
  trial_eligible: false,
  comments_this_month: 40000,
  comments_limit: 50000,
  sites_count: 2,
  sites_limit: 20,
})

describe('customer usage thresholds', () => {
  test('warns paid accounts at 80 percent', () => {
    expect(computeNudge(status)?.level).toBe('warning')
  })
  test('does not round below the warning boundary up', () => {
    expect(computeNudge({ ...status, comments_this_month: 39999 })).toBeNull()
  })
  test('marks the paid limit critical and caps progress', () => {
    expect(computeNudge({ ...status, comments_this_month: 50001 })).toMatchObject({
      level: 'critical',
      metric: 'comments',
      pct: 100,
    })
  })
  test('handles finite site caps and unlimited allowances', () => {
    expect(computeNudge({ ...status, comments_this_month: 0, sites_count: 20 })?.metric).toBe(
      'sites',
    )
    expect(computeNudge({ ...status, comments_limit: -1, sites_limit: null })).toBeNull()
  })
})
