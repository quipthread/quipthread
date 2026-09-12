import '../../../embed/tests/setup'
import { afterEach, beforeEach, spyOn } from 'bun:test'
import { notifyManager } from '@tanstack/preact-query'
import { render, type VNode } from 'preact'
import { act } from 'preact/test-utils'
import { queryClient } from '../src/lib/queryClient'

export const billing = {
  plan: 'pro',
  status: 'active',
  trial_ends_at: null,
  current_period_end: null,
  interval: '',
  comments_this_month: 0,
  comments_limit: 1000,
  sites_count: 1,
  sites_limit: 3,
}

export const comment = {
  id: 'comment-1',
  site_id: 'site-1',
  page_id: 'page-1',
  user_id: 'user-1',
  content: 'A comment to moderate',
  status: 'pending',
  created_at: '2026-09-09T00:00:00Z',
  updated_at: '2026-09-09T00:00:00Z',
}

export function dashboardFixture() {
  const container = document.createElement('div')
  let respond: (url: URL, init?: RequestInit) => Response | Promise<Response> = () =>
    Response.json({})
  const requests: string[] = []
  let fetchSpy: ReturnType<typeof spyOn<typeof globalThis, 'fetch'>>

  beforeEach(() => {
    document.body.append(container)
    queryClient.clear()
    queryClient.setDefaultOptions({ queries: { retry: false, staleTime: 30_000 } })
    notifyManager.setScheduler(queueMicrotask)
    requests.length = 0
    const fixtureFetch = Object.assign(
      async (input: string | URL | Request, init?: RequestInit) => {
        const url = new URL(String(input), 'https://dashboard.example.test')
        requests.push(`${init?.method ?? 'GET'} ${url.pathname}${url.search}`)
        return respond(url, init)
      },
      { preconnect: globalThis.fetch.preconnect },
    )
    fetchSpy = spyOn(globalThis, 'fetch').mockImplementation(fixtureFetch)
  })

  afterEach(() => {
    act(() => render(null, container))
    container.remove()
    queryClient.clear()
    fetchSpy.mockRestore()
    notifyManager.setScheduler((callback) => setTimeout(callback, 0))
  })

  return {
    container,
    requests,
    respondWith(handler: typeof respond) {
      respond = handler
    },
    async mount(view: VNode) {
      await act(async () => render(view, container))
      await this.settle()
    },
    async settle() {
      for (let round = 0; round < 30; round++) {
        await act(async () => {
          await new Promise<void>((resolve) => setImmediate(resolve))
        })
        if (!queryClient.isFetching()) return
      }
      throw new Error('Dashboard queries did not settle')
    },
    async click(label: string) {
      const button = [...container.querySelectorAll('button')].find(
        (candidate) => candidate.textContent?.trim() === label,
      )
      if (!button) throw new Error(`Button not found: ${label}`)
      await act(async () => button.click())
      await this.settle()
    },
  }
}
