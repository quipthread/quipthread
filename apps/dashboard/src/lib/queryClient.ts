import { QueryClient } from '@tanstack/preact-query'

// Module-level singleton shared across all Astro islands on the same page.
// Each island wraps itself in QueryClientProvider pointing at this instance,
// so they share a single cache — billing status fetched by NavPlanSync is
// reused by BillingPanel, AnalyticsPanel, etc. without additional requests.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000, // 30 seconds
      retry: 1,
      refetchOnWindowFocus: true,
    },
  },
})
