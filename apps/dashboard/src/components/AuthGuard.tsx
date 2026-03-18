import { useState, useEffect } from 'preact/hooks'
import type { ComponentChildren } from 'preact'
import { api, API } from '../api'

interface AuthGuardProps {
  children: ComponentChildren
}

export default function AuthGuard({ children }: AuthGuardProps) {
  const [status, setStatus] = useState<'loading' | 'ok' | 'redirect'>('loading')

  useEffect(() => {
    api.me()
      .then((user) => {
        if (user.role === 'admin') {
          setStatus('ok')
        } else {
          setStatus('redirect')
        }
      })
      .catch(() => setStatus('redirect'))
  }, [])

  useEffect(() => {
    if (status === 'redirect') {
      window.location.href = '/login'
    }
  }, [status])

  if (status === 'loading' || status === 'redirect') {
    return null
  }

  return <>{children}</>
}
