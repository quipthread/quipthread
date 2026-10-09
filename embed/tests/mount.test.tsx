import { afterEach, beforeEach, expect, test } from 'bun:test'
import { act } from 'preact/test-utils'
import { initApi } from '../src/api'
import { mount, scan } from '../src/mount'

const requests: string[] = []
const originalFetch = globalThis.fetch
const originalError = console.error
const errors: unknown[][] = []

beforeEach(() => {
  initApi('https://comments.example.test')
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input)
    requests.push(url)
    const body = url.includes('/api/comments')
      ? { comments: [], total: 0 }
      : url.includes('/api/config')
        ? { turnstileSiteKey: '', theme: 'auto' }
        : null
    return new Response(JSON.stringify(body), {
      status: body ? 200 : 401,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as typeof fetch
  console.error = (...args: unknown[]) => {
    errors.push(args)
  }
})

afterEach(() => {
  globalThis.fetch = originalFetch
  console.error = originalError
  requests.length = 0
  errors.length = 0
  document.body.innerHTML = ''
})

function container(attrs: Record<string, string>): HTMLElement {
  const el = document.createElement('div')
  for (const [name, value] of Object.entries(attrs)) el.setAttribute(name, value)
  document.body.append(el)
  return el
}

// Preact's act() does not return the callback's value, unlike React's.
async function mountInAct(...args: Parameters<typeof mount>) {
  let handle: ReturnType<typeof mount> | undefined
  await act(async () => {
    handle = mount(...args)
  })
  if (!handle) throw new TypeError('mount returned no handle')
  return handle
}

async function settle() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

function pageRequests(pageId: string) {
  return requests.filter(
    (url) => url.includes('/api/comments?') && url.includes(`pageId=${encodeURIComponent(pageId)}`),
  )
}

test('scan mounts data-quipthread containers and claimed legacy #comments only', async () => {
  // Given: the new markup, the legacy markup, and a theme's own #comments.
  const current = container({ 'data-quipthread': '', 'data-site-id': 's1', 'data-page-id': '/a' })
  const theme = container({ id: 'comments' })
  const legacy = container({ id: 'comments', 'data-site-id': 's1', 'data-page-id': '/b' })

  // When: the page is scanned.
  await act(async () => scan())
  await settle()

  // Then: both widget containers render, and the theme's element is untouched.
  expect(current.querySelector('.qt-root')).not.toBeNull()
  expect(legacy.querySelector('.qt-root')).not.toBeNull()
  expect(theme.innerHTML).toBe('')
  expect(errors).toEqual([])
})

test('mounting an element twice keeps one widget and applies the new options', async () => {
  const el = container({ 'data-site-id': 's1', 'data-page-id': '/first' })

  const first = await mountInAct(el)
  const second = await mountInAct(el, { pageId: '/second' })
  await settle()
  await act(async () => scan())

  expect(second).toBe(first)
  expect(el.querySelectorAll('.qt-root')).toHaveLength(1)
  expect(pageRequests('/second').length).toBeGreaterThan(0)
})

test('options override data attributes, update switches threads, and unmount empties the container', async () => {
  const el = container({
    'data-site-id': 's1',
    'data-page-id': '/from-attribute',
    'data-qt-accent': '#111',
  })

  const handle = await mountInAct(el, { pageId: '/from-options', vars: { accent: '#222' } })
  await settle()
  expect(pageRequests('/from-attribute')).toEqual([])
  expect(pageRequests('/from-options').length).toBeGreaterThan(0)
  expect(el.querySelector<HTMLElement>('.qt-root')?.style.getPropertyValue('--qt-accent')).toBe(
    '#222',
  )

  await act(async () => handle.update({ pageId: '/next' }))
  await settle()
  expect(pageRequests('/next').length).toBeGreaterThan(0)

  await act(async () => handle.unmount())
  expect(el.innerHTML).toBe('')
  // A later mount starts a fresh widget.
  await mountInAct(el)
  expect(el.querySelectorAll('.qt-root')).toHaveLength(1)
})

test('a container without siteId or pageId is reported and not mounted', async () => {
  const el = container({ 'data-site-id': 's1' })
  const handle = await mountInAct(el)
  expect(el.innerHTML).toBe('')
  expect(errors.length).toBe(1)
  // The returned handle is inert.
  handle.update({ pageId: '/x' })
  handle.unmount()

  await mountInAct('#missing')
  expect(errors.length).toBe(2)
})
