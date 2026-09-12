import { describe, expect, test } from 'bun:test'
import AnalyticsPanel from '../src/components/AnalyticsPanel'
import ModRulesPanel from '../src/components/ModRulesPanel'
import { billing, dashboardFixture } from './dashboardTestUtils'

describe('dashboard query failures', () => {
  const fixture = dashboardFixture()

  for (const Panel of [AnalyticsPanel, ModRulesPanel]) {
    test(`${Panel.name} offers billing retry when the plan request fails`, async () => {
      let billingFails = true
      fixture.respondWith((url) => {
        if (url.pathname === '/api/billing/status') {
          return billingFails
            ? Response.json({ error: 'unavailable' }, { status: 503 })
            : Response.json(billing)
        }
        if (url.pathname === '/api/admin/sites') return Response.json({ sites: [] })
        return Response.json({ terms: [] })
      })

      await fixture.mount(<Panel />)

      expect(fixture.container.querySelector('.loading')?.textContent ?? null).toBeNull()
      expect(fixture.container.querySelector('[role="alert"]')?.textContent).toContain('billing')
      billingFails = false
      await fixture.click('Retry')
      expect(
        fixture.requests.filter((request) => request === 'GET /api/billing/status'),
      ).toHaveLength(2)
      expect(fixture.container.querySelector('[role="alert"]')?.textContent ?? null).toBeNull()
    })
  }

  test('moderation rules distinguish a list failure from an empty blocklist and can retry', async () => {
    let listFails = true
    fixture.respondWith((url) => {
      if (url.pathname === '/api/billing/status') return Response.json(billing)
      return listFails
        ? Response.json({ error: 'unavailable' }, { status: 503 })
        : Response.json({
            terms: [{ id: 'term-1', term: 'spam', is_regex: false, created_at: '' }],
          })
    })

    await fixture.mount(<ModRulesPanel />)

    expect(fixture.container.querySelector('[role="alert"]')?.textContent).toContain(
      'blocked terms',
    )
    expect(fixture.container.textContent).not.toContain('No blocked terms yet')
    listFails = false
    await fixture.click('Retry')
    expect(fixture.container.querySelector('[role="alert"]')?.textContent ?? null).toBeNull()
    expect(fixture.container.textContent).toContain('spam')
  })

  test('analytics retries its own failed query without refetching successful billing', async () => {
    let analyticsFails = true
    fixture.respondWith((url) => {
      if (url.pathname === '/api/billing/status') return Response.json(billing)
      if (url.pathname === '/api/admin/sites') {
        return Response.json({
          sites: [
            {
              id: 'site-1',
              owner_id: 'owner-1',
              domain: 'example.test',
              theme: 'light',
              created_at: '',
              sso_enabled: false,
            },
          ],
        })
      }
      return analyticsFails
        ? Response.json({ error: 'unavailable' }, { status: 503 })
        : Response.json({ volume: [], pages: [], commenters: [] })
    })
    await fixture.mount(<AnalyticsPanel />)

    expect(fixture.container.querySelector('[role="alert"]')?.textContent).toContain('analytics')
    analyticsFails = false
    await fixture.click('Retry')

    expect(fixture.container.querySelector('[role="alert"]')?.textContent ?? null).toBeNull()
    expect(
      fixture.requests.filter((request) => request === 'GET /api/billing/status'),
    ).toHaveLength(1)
    expect(
      fixture.requests.filter((request) => request.includes('/api/admin/analytics?')),
    ).toHaveLength(2)
  })
})
