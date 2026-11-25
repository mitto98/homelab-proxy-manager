import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { api, type HostInput } from '@/api/client'
import { hostsKey } from '@/query'

/** All hosts, oldest first (the server's order). */
export function useHostsQuery() {
  return useQuery({ queryKey: hostsKey, queryFn: api.listHosts })
}

// Every mutation refetches the list so the table reflects what the server
// stored (it lower-cases domains and clears skip-verify for http).
function useInvalidateHosts() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: hostsKey })
}

export function useCreateHost() {
  const invalidate = useInvalidateHosts()
  return useMutation({
    mutationFn: (input: HostInput) => api.createHost(input),
    onSuccess: invalidate,
  })
}

export function useUpdateHost() {
  const invalidate = useInvalidateHosts()
  return useMutation({
    mutationFn: (v: { id: number; input: HostInput }) => api.updateHost(v.id, v.input),
    onSuccess: invalidate,
  })
}

export function useDeleteHost() {
  const invalidate = useInvalidateHosts()
  return useMutation({
    mutationFn: (id: number) => api.deleteHost(id),
    onSuccess: invalidate,
  })
}
