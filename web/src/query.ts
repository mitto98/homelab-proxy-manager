import { QueryClient } from '@tanstack/vue-query'
import type { Api } from '@/api/client'

export const statusKey = ['status'] as const
export const hostsKey = ['hosts'] as const

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
}

/** GET /status. Once setup is done it cannot become required again, so trust that answer. */
export function fetchStatus(qc: QueryClient, api: Api) {
  return qc.fetchQuery({
    queryKey: statusKey,
    queryFn: api.getStatus,
    staleTime: (q) => (q.state.data?.setupRequired === false ? Infinity : 0),
  })
}
