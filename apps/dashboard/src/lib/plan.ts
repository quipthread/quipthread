// Dashboard view of the shared plan catalog (@quipthread/plans).
import {
  PLANS as CATALOG,
  commentsLabel,
  formatPrice,
  maxAnnualSavingsPercent,
  meetsPlan,
  type PlanId,
  sitesLabel,
} from '@quipthread/plans'

export type Plan = PlanId

export { maxAnnualSavingsPercent, meetsPlan }

const byPlan = <T>(value: (plan: (typeof CATALOG)[number]) => T) =>
  Object.fromEntries(CATALOG.map((plan) => [plan.id, value(plan)])) as Record<Plan, T>

export const PLAN_LABELS: Record<Plan, string> = byPlan((plan) => plan.name)

const unpriced = (id: Plan) => (id === 'hobby' ? 'Free' : 'Contact us')

export const PLAN_PRICES: Record<Plan, string> = byPlan((plan) =>
  plan.prices ? `${formatPrice(plan.prices.month)} / month` : unpriced(plan.id),
)

export const PLAN_ANNUAL_PRICES: Record<Plan, string> = byPlan((plan) =>
  plan.prices ? `${formatPrice(plan.prices.year)} / year` : unpriced(plan.id),
)

export interface PlanFeatures {
  sites: string
  comments: string
  highlights: string[]
}

export const PLAN_FEATURES: Record<Plan, PlanFeatures> = byPlan((plan) => ({
  sites: sitesLabel(plan),
  comments: commentsLabel(plan),
  highlights: plan.features,
}))
