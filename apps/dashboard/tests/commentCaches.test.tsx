import { describe, expect, spyOn, test } from 'bun:test'
import { options } from 'preact'
import { act } from 'preact/test-utils'
import CommentsPanel from '../src/components/CommentsPanel'
import { queryClient } from '../src/lib/queryClient'
import { queryKeys } from '../src/lib/queryKeys'
import { comment, dashboardFixture } from './dashboardTestUtils'

describe('comment moderation cache consistency', () => {
  const fixture = dashboardFixture()

  test('approving a pending comment refreshes an already visited approved tab and other cached pages', async () => {
    let status = 'pending'
    const otherPage = queryKeys.comments({
      status: 'pending',
      page: 2,
      limit: 20,
      siteId: 'site-1',
    })
    fixture.respondWith((url, init) => {
      if (url.pathname === '/api/admin/sites') return Response.json({ sites: [] })
      if (init?.method === 'PATCH') {
        status = 'approved'
        return Response.json({ ...comment, status })
      }
      const comments = url.searchParams.get('status') === status ? [{ ...comment, status }] : []
      return Response.json({ comments, total: comments.length, page: 1 })
    })
    await fixture.mount(<CommentsPanel />)
    await fixture.click('Approved')
    await fixture.click('Pending')
    queryClient.setQueryData(otherPage, { comments: [], total: 21, page: 2 })

    await fixture.click('Approve')
    await fixture.click('Approved')

    expect(fixture.container.textContent).toContain(comment.content)
    expect(queryClient.getQueryState(otherPage)?.isInvalidated).toBe(true)
  })

  test('rejecting an approved comment refreshes an already visited rejected tab', async () => {
    let status = 'approved'
    fixture.respondWith((url, init) => {
      if (url.pathname === '/api/admin/sites') return Response.json({ sites: [] })
      if (init?.method === 'PATCH') {
        status = 'rejected'
        return Response.json({ ...comment, status })
      }
      const comments = url.searchParams.get('status') === status ? [{ ...comment, status }] : []
      return Response.json({ comments, total: comments.length, page: 1 })
    })
    await fixture.mount(<CommentsPanel />)
    await fixture.click('Rejected')
    await fixture.click('Approved')

    await fixture.click('Reject')
    await fixture.click('Rejected')

    expect(fixture.container.textContent).toContain(comment.content)
  })

  for (const action of ['Delete pending', 'Delete approved', 'Approve selected', 'Edit', 'Reply']) {
    test(`${action} invalidates related status, flagged, site, and page caches`, async () => {
      fixture.respondWith((url, init) => {
        if (url.pathname === '/api/admin/sites') return Response.json({ sites: [] })
        if (init?.method) return Response.json(comment)
        return Response.json({ comments: [comment], total: 1, page: 1 })
      })
      await fixture.mount(<CommentsPanel />)
      if (action === 'Delete approved') await fixture.click('Approved')
      const relatedKeys = [
        queryKeys.comments({ status: 'rejected', page: 2, limit: 20 }),
        queryKeys.comments({ flagged: true, page: 1, limit: 20, siteId: 'site-1' }),
      ]
      for (const key of relatedKeys) queryClient.setQueryData(key, { comments: [], total: 0 })
      const confirmSpy = spyOn(globalThis, 'confirm').mockReturnValue(true)

      try {
        switch (action) {
          case 'Delete pending':
          case 'Delete approved':
            await fixture.click('Delete')
            break
          case 'Approve selected': {
            const checkbox =
              fixture.container.querySelector<HTMLInputElement>('input[type="checkbox"]')
            if (!checkbox) throw new Error('Select all checkbox missing')
            await act(async () => checkbox.click())
            await fixture.click('Approve 1')
            break
          }
          case 'Edit':
          case 'Reply': {
            await fixture.click(action)
            const textarea = fixture.container.querySelector('textarea')
            if (!textarea) throw new Error('Comment textarea missing')
            await act(async () => {
              textarea.value = 'Updated content'
              textarea.dispatchEvent(new window.Event('change', { bubbles: true }))
            })
            await fixture.click(action === 'Edit' ? 'Save changes' : 'Send reply')
            break
          }
        }
      } finally {
        confirmSpy.mockRestore()
      }

      for (const key of relatedKeys)
        expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true)
      expect(queryClient.getQueryState(queryKeys.sites())?.isInvalidated).toBe(false)
    })
  }

  test('a failed bulk action waits for slower successful updates before invalidating caches', async () => {
    const slowUpdate = Promise.withResolvers<Response>()
    let approveSelected: (() => unknown) | undefined
    const previousHook = options.vnode
    options.vnode = (vnode) => {
      previousHook?.(vnode)
      const props: unknown = vnode.props
      if (
        vnode.type === 'button' &&
        props &&
        typeof props === 'object' &&
        'children' in props &&
        Array.isArray(props.children) &&
        props.children[0] === 'Approve ' &&
        'onClick' in props &&
        typeof props.onClick === 'function'
      ) {
        const onClick = props.onClick
        approveSelected = () => onClick()
      }
    }
    fixture.respondWith((url, init) => {
      if (url.pathname === '/api/admin/sites') return Response.json({ sites: [] })
      if (init?.method === 'PATCH') {
        return url.pathname.endsWith('comment-1')
          ? Response.json({ error: 'Could not approve comment' }, { status: 503 })
          : slowUpdate.promise
      }
      return Response.json({
        comments: [comment, { ...comment, id: 'comment-2' }],
        total: 2,
        page: 1,
      })
    })
    const relatedKey = queryKeys.comments({ status: 'approved', page: 1, limit: 20 })
    let operation: Promise<unknown> | undefined
    let finished = false
    let failure: unknown

    try {
      await fixture.mount(<CommentsPanel />)
      const checkbox = fixture.container.querySelector<HTMLInputElement>('input[type="checkbox"]')
      if (!checkbox) throw new Error('Select all checkbox missing')
      await act(async () => checkbox.click())
      queryClient.setQueryData(relatedKey, { comments: [], total: 0 })
      if (!approveSelected) throw new Error('Bulk approve action missing')

      operation = Promise.resolve(approveSelected()).catch((error: unknown) => {
        failure = error
        finished = true
      })
      await fixture.settle()
      expect(finished).toBe(false)
      slowUpdate.resolve(Response.json({ ...comment, id: 'comment-2', status: 'approved' }))
      await operation

      expect(failure).toBeInstanceOf(Error)
      expect(queryClient.getQueryState(relatedKey)?.isInvalidated).toBe(true)
    } finally {
      slowUpdate.resolve(Response.json({ ...comment, id: 'comment-2', status: 'approved' }))
      await operation
      await fixture.settle()
      options.vnode = previousHook
    }
  })
})
