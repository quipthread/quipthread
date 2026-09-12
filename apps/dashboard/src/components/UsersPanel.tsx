import { useQuery, useQueryClient } from '@tanstack/preact-query'
import { useState } from 'preact/hooks'
import { api } from '../api'
import { queryKeys } from '../lib/queryKeys'
import type { User } from '../types'
import { relativeTime } from '../utils'
import QueryProvider from './QueryProvider'
import PageHeader from './shared/PageHeader'

const PAGE_SIZE = 20

function UsersPanelInner() {
  const [page, setPage] = useState(1)
  const [acting, setActing] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const { data: meData } = useQuery({
    queryKey: queryKeys.me(),
    queryFn: () => api.me(),
    staleTime: 60_000,
  })
  const currentUserId = meData?.id ?? null

  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.users({ page, limit: PAGE_SIZE }),
    queryFn: () => api.users.list({ page, limit: PAGE_SIZE }),
  })

  const users = data?.users ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)

  function updateUserInCache(updated: User) {
    queryClient.setQueryData<{ users: User[]; total: number }>(
      queryKeys.users({ page, limit: PAGE_SIZE }),
      (old) =>
        old ? { ...old, users: old.users.map((u) => (u.id === updated.id ? updated : u)) } : old,
    )
  }

  const toggleBan = async (user: User) => {
    setActing(user.id)
    try {
      const updated = (await api.users.update(user.id, { banned: !user.banned })) as User
      updateUserInCache(updated)
    } finally {
      setActing(null)
    }
  }

  const toggleShadowBan = async (user: User) => {
    setActing(user.id)
    try {
      const updated = (await api.users.update(user.id, {
        shadow_banned: !user.shadow_banned,
      })) as User
      updateUserInCache(updated)
    } finally {
      setActing(null)
    }
  }

  const toggleAdmin = async (user: User) => {
    const next = user.role === 'admin' ? 'user' : 'admin'
    setActing(user.id)
    try {
      const updated = (await api.users.update(user.id, { role: next })) as User
      updateUserInCache(updated)
    } finally {
      setActing(null)
    }
  }

  return (
    <>
      <PageHeader title="Users" action={<span className="page-count">{total} total</span>} />

      {isLoading ? (
        <div className="loading">Loading…</div>
      ) : isError ? (
        <div className="error-msg">Failed to load users.</div>
      ) : users.length === 0 ? (
        <div className="empty">No users found.</div>
      ) : (
        <>
          <div className="table-card">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th>Joined</th>
                  <th>Actions</th>
                </tr>
              </thead>
              {users.map((u) => (
                <tbody key={u.id}>
                  <tr>
                    <td data-label="Name" style={{ fontWeight: 500 }}>
                      {u.display_name || <span style={{ color: 'var(--muted)' }}>—</span>}
                    </td>
                    <td
                      data-label="Email"
                      style={{
                        color: 'var(--muted)',
                        fontSize: '0.8125rem',
                        wordBreak: 'break-all',
                      }}
                    >
                      {u.email || <span style={{ color: 'var(--muted)' }}>—</span>}
                    </td>
                    <td data-label="Role">
                      <span
                        className={`badge ${u.role === 'admin' ? 'badge-admin' : 'badge-user'}`}
                      >
                        {u.role}
                      </span>
                    </td>
                    <td data-label="Status">
                      {u.banned ? (
                        <span className="badge badge-banned">Banned</span>
                      ) : u.shadow_banned ? (
                        <span className="badge badge-shadow">Shadow Banned</span>
                      ) : (
                        <span style={{ color: 'var(--muted)', fontSize: '0.8125rem' }}>—</span>
                      )}
                    </td>
                    <td
                      data-label="Joined"
                      style={{ color: 'var(--muted)', fontSize: '0.8125rem' }}
                    >
                      {relativeTime(u.created_at)}
                    </td>
                    <td data-label="Actions">
                      <div className="actions">
                        <button
                          type="button"
                          className="btn"
                          disabled={acting === u.id || u.id === currentUserId}
                          onClick={() => toggleAdmin(u)}
                        >
                          {u.role === 'admin' ? 'Demote' : 'Make Admin'}
                        </button>
                        <button
                          type="button"
                          className={`btn${u.banned ? '' : ' btn-reject'}`}
                          disabled={acting === u.id || u.id === currentUserId}
                          onClick={() => toggleBan(u)}
                        >
                          {u.banned ? 'Unban' : 'Ban'}
                        </button>
                        <button
                          type="button"
                          className="btn"
                          disabled={acting === u.id || u.id === currentUserId || u.banned}
                          onClick={() => toggleShadowBan(u)}
                          title={u.banned ? 'Cannot shadow ban an already-banned user' : undefined}
                        >
                          {u.shadow_banned ? 'Unshadow' : 'Shadow Ban'}
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              ))}
            </table>
          </div>

          {totalPages > 1 && (
            <div className="pagination">
              <button
                type="button"
                className="btn"
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
              >
                ←
              </button>
              <span>
                {page} / {totalPages}
              </span>
              <button
                type="button"
                className="btn"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
              >
                →
              </button>
            </div>
          )}
        </>
      )}
    </>
  )
}

export default function UsersPanel() {
  return (
    <QueryProvider>
      <UsersPanelInner />
    </QueryProvider>
  )
}
