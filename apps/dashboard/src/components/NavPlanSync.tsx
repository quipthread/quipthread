import { useQuery } from '@tanstack/preact-query'
import { useEffect } from 'preact/hooks'
import { api } from '../api'
import { queryClient } from '../lib/queryClient'
import { queryKeys } from '../lib/queryKeys'
import QueryProvider from './QueryProvider'

function NavPlanSyncInner() {
  const { data } = useQuery({
    queryKey: queryKeys.billingStatus(),
    queryFn: () => api.billing.status(),
    staleTime: 60_000,
  })

  useEffect(() => {
    if (!data) return
    document.documentElement.dataset.plan = data.plan
    localStorage.setItem('qt-plan', data.plan)
  }, [data])

  return null
}

export default function NavPlanSync() {
  return (
    <QueryProvider>
      <NavPlanSyncInner />
    </QueryProvider>
  )
}

// Exported so CheckoutSuccessModal can bust the billing cache after a successful
// checkout without needing to mount its own provider.
export { queryClient }
