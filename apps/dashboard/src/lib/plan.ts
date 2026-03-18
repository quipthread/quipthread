export type Plan = 'hobby' | 'starter' | 'pro' | 'business'

const PLAN_ORDER: Plan[] = ['hobby', 'starter', 'pro', 'business']

// Hardcoded until M16 (Stripe) is wired; swap this for a live API call then.
export const CURRENT_PLAN: Plan = 'hobby'

export function canAccess(minPlan: Plan): boolean {
  return PLAN_ORDER.indexOf(CURRENT_PLAN) >= PLAN_ORDER.indexOf(minPlan)
}

export const PLAN_LABELS: Record<Plan, string> = {
  hobby: 'Hobby',
  starter: 'Starter',
  pro: 'Pro',
  business: 'Business',
}

export const PLAN_PRICES: Record<Plan, string> = {
  hobby: 'Free',
  starter: '$9 / month',
  pro: '$24 / month',
  business: '$79 / month',
}

export const PLAN_ANNUAL_PRICES: Record<Plan, string> = {
  hobby: 'Free',
  starter: '$90 / year',
  pro: '$230 / year',
  business: '$750 / year',
}

export interface PlanFeatures {
  sites: string
  comments: string
  highlights: string[]
}

export const PLAN_FEATURES: Record<Plan, PlanFeatures> = {
  hobby: {
    sites: '1 site',
    comments: '1,000 comments / month',
    highlights: [
      'All auth providers (GitHub, Google, email)',
      'Moderation queue',
      'Webhook notifications',
      'Email notifications',
      'Quick-approve via email link',
    ],
  },
  starter: {
    sites: '5 sites',
    comments: '10,000 comments / month',
    highlights: [
      'Everything in Hobby',
      'Custom SMTP configuration',
      'Comment export (JSON, CSV)',
      'Disqus import',
      'Basic analytics',
      'API access (read-only)',
    ],
  },
  pro: {
    sites: '20 sites',
    comments: '50,000 comments / month',
    highlights: [
      'Everything in Starter',
      'Moderation rules engine',
      'Comment reactions',
      'Full API access (read/write)',
      'Multiple webhooks per site',
      'Priority email support',
    ],
  },
  business: {
    sites: 'Unlimited sites',
    comments: '250,000 comments / month',
    highlights: [
      'Everything in Pro',
      'White-label embed',
      'Custom dashboard domain',
      'Dedicated database per site',
      'Advanced analytics',
      'Audit log',
      'SSO (Google / GitHub org)',
    ],
  },
}
