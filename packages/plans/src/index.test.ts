import { expect, test } from 'bun:test'
import {
  annualMonthlyCents,
  annualSavings,
  commentsLabel,
  formatPrice,
  getPlan,
  maxAnnualSavingsPercent,
  meetsPlan,
  PLANS,
  SELF_SERVE_PLANS,
  sitesLabel,
} from './index'

test('plans are ordered by rank with unique ids and ranks', () => {
  expect(PLANS.map((p) => p.id)).toEqual(['hobby', 'starter', 'pro', 'business', 'enterprise'])
  expect(new Set(PLANS.map((p) => p.rank)).size).toBe(PLANS.length)
})

test('limits grow with rank and paid plans charge less yearly than monthly', () => {
  for (const [i, plan] of PLANS.entries()) {
    const prev = PLANS[i - 1]
    if (!prev) continue
    const atLeast = (a: number, b: number) => a === -1 || (b !== -1 && a >= b)
    expect(atLeast(plan.limits.comments, prev.limits.comments)).toBe(true)
    expect(atLeast(plan.limits.sites, prev.limits.sites)).toBe(true)
  }
  expect(SELF_SERVE_PLANS.map((p) => p.id)).toEqual(['starter', 'pro', 'business'])
  for (const plan of SELF_SERVE_PLANS) {
    const prices = plan.prices
    if (!prices) throw new Error('self-serve plan without prices')
    expect(prices.year).toBeLessThan(prices.month * 12)
  }
})

test('meetsPlan compares ranks and includes enterprise', () => {
  expect(meetsPlan('enterprise', 'business')).toBe(true)
  expect(meetsPlan('pro', 'starter')).toBe(true)
  expect(meetsPlan('starter', 'pro')).toBe(false)
  expect(meetsPlan('selfhosted', 'hobby')).toBe(false)
  expect(meetsPlan(null, 'hobby')).toBe(false)
})

test('labels and prices', () => {
  expect(sitesLabel(getPlan('hobby'))).toBe('1 site')
  expect(sitesLabel(getPlan('business'))).toBe('Unlimited sites')
  expect(commentsLabel(getPlan('starter'))).toBe('10,000 comments / month')
  expect(formatPrice(900)).toBe('$9')
  expect(formatPrice(annualMonthlyCents({ month: 900, year: 9000 }))).toBe('$7.50')
  expect(annualSavings({ month: 2400, year: 22800 }).cents).toBe(6000)
  expect(maxAnnualSavingsPercent()).toBe(20)
})
