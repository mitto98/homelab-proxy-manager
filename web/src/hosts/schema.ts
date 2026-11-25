import { z } from 'zod'
import type { Host, HostInput } from '@/api/client'

// Mirrors only the simple rules of HostInput in api/openapi.yaml and
// internal/model/host.go. The server stays the final validator (domain syntax,
// duplicates across hosts, host name syntax), and its errors are shown on the
// matching field.
export const hostFormSchema = z.object({
  domains: z
    .array(z.string().trim().min(1, 'A domain must not be empty'))
    .min(1, 'Add at least one domain'),
  upstreamScheme: z.enum(['http', 'https']),
  upstreamHost: z.string().trim().min(1, 'Enter the upstream host'),
  // A number input yields '' when empty, which coerces to 0 and fails the range check.
  upstreamPort: z.coerce
    .number({ invalid_type_error: 'Enter a port number' })
    .int('The port must be a whole number')
    .min(1, 'The port must be between 1 and 65535')
    .max(65535, 'The port must be between 1 and 65535'),
  upstreamSkipTlsVerify: z.boolean(),
  enabled: z.boolean(),
})

export type HostFormValues = z.output<typeof hostFormSchema>

/** The request fields a 400 can name that have an input on the form. */
export const hostFormFields = [
  'domains',
  'upstreamScheme',
  'upstreamHost',
  'upstreamPort',
  'upstreamSkipTlsVerify',
  'enabled',
] as const
export type HostFormField = (typeof hostFormFields)[number]

/** What a new host starts with. */
export const newHostValues: HostFormValues = {
  domains: [],
  upstreamScheme: 'http',
  upstreamHost: '',
  upstreamPort: 80,
  upstreamSkipTlsVerify: false,
  enabled: true,
}

export function hostToFormValues(host: Host | null): HostFormValues {
  if (!host) return { ...newHostValues, domains: [] }
  return {
    domains: [...host.domains],
    upstreamScheme: host.upstreamScheme,
    upstreamHost: host.upstreamHost,
    upstreamPort: host.upstreamPort,
    upstreamSkipTlsVerify: host.upstreamSkipTlsVerify,
    enabled: host.enabled,
  }
}

export function formValuesToInput(v: HostFormValues): HostInput {
  return {
    domains: v.domains,
    upstreamScheme: v.upstreamScheme,
    upstreamHost: v.upstreamHost.trim(),
    upstreamPort: v.upstreamPort,
    // Only meaningful for https; the server clears it for http as well.
    upstreamSkipTlsVerify: v.upstreamScheme === 'https' && v.upstreamSkipTlsVerify,
    enabled: v.enabled,
  }
}
