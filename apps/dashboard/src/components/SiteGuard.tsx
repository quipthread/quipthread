import { useEffect } from 'preact/hooks'
import { api } from '../api'

// Redirects unauthenticated or zero-site users to the appropriate page.
// Renders nothing — mount via <SiteGuard client:load /> in AdminLayout.
export default function SiteGuard() {
  useEffect(() => {
    api
      .me()
      .then(() => api.sites.list())
      .then(({ sites }) => {
        if (sites.length === 0 && !window.location.pathname.includes('/onboarding')) {
          window.location.href = '/dashboard/onboarding'
        }
      })
      .catch(() => {})
  }, [])

  return null
}
