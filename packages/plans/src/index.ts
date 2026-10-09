// Plan data for the website, dashboard, and Stripe setup. catalog.json is the
// single source; the backend generates its copy from it (backend/plans).
import catalog from '../catalog.json'

export type PlanId = 'hobby' | 'starter' | 'pro' | 'business' | 'enterprise'

/** Prices in cents. Plans without prices are free (hobby) or sales-only (enterprise). */
export interface PlanPrices {
  month: number
  year: number
}

export interface Plan {
  id: PlanId
  name: string
  rank: number
  /** -1 means unlimited. */
  limits: { comments: number; sites: number }
  prices?: PlanPrices
  description: string
  features: string[]
}

export const CURRENCY: string = catalog.currency
export const TRIAL_DAYS: number = catalog.trialDays

/** Every plan, lowest rank first. */
export const PLANS: readonly Plan[] = [...(catalog.plans as Plan[])].sort((a, b) => a.rank - b.rank)

/** Plans sold through self-serve checkout. */
export const SELF_SERVE_PLANS: readonly Plan[] = PLANS.filter((plan) => plan.prices)

export function getPlan(id: PlanId): Plan {
  const plan = PLANS.find((p) => p.id === id)
  if (!plan) throw new Error(`unknown plan: ${id}`)
  return plan
}

/**
 * Whether plan includes everything in min. Unknown plans (including the
 * self-hosted build's "selfhosted") never qualify; callers decide what
 * self-hosted unlocks.
 */
export function meetsPlan(plan: string | null | undefined, min: PlanId): boolean {
  const current = PLANS.find((p) => p.id === plan)
  return current !== undefined && current.rank >= getPlan(min).rank
}

const number = new Intl.NumberFormat('en-US')

export function sitesLabel(plan: Plan): string {
  const { sites } = plan.limits
  if (sites === -1) return 'Unlimited sites'
  return `${number.format(sites)} ${sites === 1 ? 'site' : 'sites'}`
}

export function commentsLabel(plan: Plan): string {
  const { comments } = plan.limits
  return comments === -1 ? 'Unlimited comments' : `${number.format(comments)} comments / month`
}

/** "$9", "$7.50": whole dollars drop the cents. */
export function formatPrice(cents: number): string {
  const dollars = cents / 100
  return `$${Number.isInteger(dollars) ? dollars : dollars.toFixed(2)}`
}

/** What a year costs per month, in cents. */
export function annualMonthlyCents(prices: PlanPrices): number {
  return prices.year / 12
}

/** Savings of paying yearly instead of twelve monthly payments. */
export function annualSavings(prices: PlanPrices): { cents: number; percent: number } {
  const cents = prices.month * 12 - prices.year
  return { cents, percent: (cents / (prices.month * 12)) * 100 }
}

/** The largest yearly saving across self-serve plans, rounded down to a whole percent. */
export function maxAnnualSavingsPercent(): number {
  return Math.floor(
    Math.max(...SELF_SERVE_PLANS.map((plan) => annualSavings(plan.prices as PlanPrices).percent)),
  )
}
