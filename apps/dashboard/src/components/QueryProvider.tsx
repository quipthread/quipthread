import { QueryClientProvider } from '@tanstack/preact-query'
import type { ComponentChildren } from 'preact'
import { queryClient } from '../lib/queryClient'

// Thin wrapper so each island component can declare its own provider
// without importing queryClient directly. All islands share the same
// queryClient instance (module-level singleton), so their caches are unified.
export default function QueryProvider({ children }: { children: ComponentChildren }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}
