import { useEffect } from 'preact/hooks'
import { api } from '../api'

export default function NavPlanSync() {
  useEffect(() => {
    api.billing.status()
      .then(status => {
        document.documentElement.dataset.plan = status.plan
        localStorage.setItem('qt-plan', status.plan)
      })
      .catch(() => {})
  }, [])
  return null
}
