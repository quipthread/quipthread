import { describe, expect, test } from 'bun:test'
import {
  BillingStatusSchema,
  CommentListResponseSchema,
  UserListResponseSchema,
} from '../src/schemas'

const nativeComment = {
  id: 'comment-1',
  site_id: 'site-1',
  page_id: 'page-1',
  user_id: 'user-1',
  content: 'Native comment',
  status: 'approved',
  imported: false,
  upvotes: 0,
  user_voted: false,
  user_flagged: false,
  created_at: '2026-09-09T00:00:00Z',
  updated_at: '2026-09-09T00:00:00Z',
} as const

const nativeUser = {
  id: 'user-1',
  display_name: 'Reader',
  role: 'commenter',
  banned: false,
  shadow_banned: false,
  email_verified: false,
  created_at: '2026-09-09T00:00:00Z',
} as const

const selfHostedBilling = {
  plan: 'business',
  status: 'active',
  trial_ends_at: null,
  current_period_end: null,
  interval: '',
  comments_this_month: 0,
  comments_limit: -1,
  sites_count: 1,
  sites_limit: null,
} as const

describe('dashboard API schemas', () => {
  test('accepts native top-level comments when Go omits empty metadata', () => {
    const response = { comments: [nativeComment], total: 1, page: 1 }

    const parsed = CommentListResponseSchema.parse(response)

    expect(parsed.comments[0]).toMatchObject({
      page_url: '',
      page_title: '',
      parent_id: '',
      disqus_author: '',
      author_name: '',
      author_avatar: '',
    })
  })

  test('preserves populated comment metadata when the backend provides it', () => {
    const metadata = {
      page_url: 'https://example.com/article',
      page_title: 'Article',
      parent_id: 'parent-1',
      disqus_author: 'Imported reader',
      author_name: 'Reader',
      author_avatar: 'https://example.com/avatar.png',
      flags: 2,
    }
    const response = { comments: [{ ...nativeComment, ...metadata }], total: 1, page: 1 }

    const parsed = CommentListResponseSchema.parse(response)

    expect(parsed.comments[0]).toMatchObject(metadata)
  })

  test('accepts users when Go omits empty email and avatar', () => {
    const response = { users: [nativeUser], total: 1, page: 1 }

    const parsed = UserListResponseSchema.parse(response)

    expect(parsed.users[0]).toMatchObject({ email: '', avatar_url: '' })
  })

  test('preserves populated user contact fields when the backend provides them', () => {
    const contact = { email: 'reader@example.com', avatar_url: 'https://example.com/avatar.png' }
    const response = { users: [{ ...nativeUser, ...contact }], total: 1, page: 1 }

    const parsed = UserListResponseSchema.parse(response)

    expect(parsed.users[0]).toMatchObject(contact)
  })

  test('disables trial eligibility when self-hosted billing omits it', () => {
    const response = selfHostedBilling

    const parsed = BillingStatusSchema.parse(response)

    expect(parsed.trial_eligible).toBe(false)
  })

  test.each([true, false])('preserves explicit cloud trial eligibility %s', (eligible) => {
    const response = { ...selfHostedBilling, trial_eligible: eligible }

    const parsed = BillingStatusSchema.parse(response)

    expect(parsed.trial_eligible).toBe(eligible)
  })
})
