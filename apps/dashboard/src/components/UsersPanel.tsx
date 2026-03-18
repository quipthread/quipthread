import { useState, useEffect, useCallback } from 'preact/hooks'
import { api } from '../api'
import { relativeTime } from '../utils'
import type { User } from '../types'

const PAGE_SIZE = 20

export default function UsersPanel() {
  const [users, setUsers] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [acting, setActing] = useState<string | null>(null)

  const fetchUsers = useCallback(async (p: number) => {
    setLoading(true)
    setError(null)
    try {
      const res = await api.users.list({ page: p, limit: PAGE_SIZE })
      setUsers(res.users ?? [])
      setTotal(res.total)
      setPage(p)
    } catch {
      setError('Failed to load users.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchUsers(1) }, [fetchUsers])

  const toggleBan = async (user: User) => {
    setActing(user.id)
    try {
      const updated = await api.users.update(user.id, { banned: !user.banned }) as User
      setUsers(prev => prev.map(u => u.id === user.id ? updated : u))
    } finally {
      setActing(null)
    }
  }

  const toggleAdmin = async (user: User) => {
    const next = user.role === 'admin' ? 'user' : 'admin'
    setActing(user.id)
    try {
      const updated = await api.users.update(user.id, { role: next }) as User
      setUsers(prev => prev.map(u => u.id === user.id ? updated : u))
    } finally {
      setActing(null)
    }
  }

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <>
      <div className="page-header">
        <h1>Users</h1>
        <span className="page-count">{total} total</span>
      </div>

      {loading ? (
        <div className="loading">Loading…</div>
      ) : error ? (
        <div className="error-msg">{error}</div>
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
              <tbody>
                {users.map(u => (
                  <tr key={u.id}>
                    <td style={{ fontWeight: 500, whiteSpace: 'nowrap' }}>
                      {u.display_name || <span style={{ color: 'var(--muted)' }}>—</span>}
                    </td>
                    <td style={{ color: 'var(--muted)', fontSize: '0.8125rem' }}>
                      {u.email || <span style={{ color: 'var(--muted)' }}>—</span>}
                    </td>
                    <td>
                      <span className={`badge ${u.role === 'admin' ? 'badge-admin' : 'badge-user'}`}>
                        {u.role}
                      </span>
                    </td>
                    <td>
                      {u.banned && <span className="badge badge-banned">Banned</span>}
                    </td>
                    <td style={{ whiteSpace: 'nowrap', color: 'var(--muted)', fontSize: '0.8125rem' }}>
                      {relativeTime(u.created_at)}
                    </td>
                    <td>
                      <div className="actions">
                        <button
                          className="btn"
                          disabled={acting === u.id}
                          onClick={() => toggleAdmin(u)}
                        >
                          {u.role === 'admin' ? 'Demote' : 'Make Admin'}
                        </button>
                        <button
                          className={`btn${u.banned ? '' : ' btn-reject'}`}
                          disabled={acting === u.id}
                          onClick={() => toggleBan(u)}
                        >
                          {u.banned ? 'Unban' : 'Ban'}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="pagination">
              <button
                className="btn"
                disabled={page <= 1}
                onClick={() => fetchUsers(page - 1)}
              >
                ←
              </button>
              <span>{page} / {totalPages}</span>
              <button
                className="btn"
                disabled={page >= totalPages}
                onClick={() => fetchUsers(page + 1)}
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
