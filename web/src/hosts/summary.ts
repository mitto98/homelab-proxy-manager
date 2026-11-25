/** The fields of the form the summary reads. They may be incomplete or invalid while the user types. */
export interface HostDraft {
  domains?: string[]
  upstreamScheme?: string
  upstreamHost?: string
  upstreamPort?: number | string
  upstreamSkipTlsVerify?: boolean
  enabled?: boolean
}

function joinNames(names: string[]): string {
  if (names.length <= 1) return names.join('')
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
}

function validPort(port: number | string | undefined): port is number | string {
  const n = Number(port)
  return port !== '' && port !== undefined && Number.isInteger(n) && n >= 1 && n <= 65535
}

/**
 * Plain-language lines describing what Traefik will do for the host as it
 * stands in the form. Pure so it can be tested without the form. Stage 2 is
 * HTTP only, so nothing is said about certificates on the public side.
 */
export function describeHost(draft: HostDraft): string[] {
  const domains = draft.domains ?? []
  const host = (draft.upstreamHost ?? '').trim()
  const missing: string[] = []
  if (domains.length === 0) missing.push('a domain name')
  if (host === '') missing.push('an upstream host')
  if (!validPort(draft.upstreamPort)) missing.push('an upstream port')

  const lines: string[] = []
  const enabled = draft.enabled !== false
  if (!enabled) lines.push('This host is disabled: Traefik does not route its domains')

  if (missing.length > 0) {
    lines.push(`Add ${joinNames(missing)} to see where requests will go`)
  } else {
    const upstream = `${draft.upstreamScheme ?? 'http'}://${host}:${Number(draft.upstreamPort)}`
    lines.push(
      enabled
        ? `Requests for ${joinNames(domains)} are forwarded to ${upstream}`
        : `Once enabled, requests for ${joinNames(domains)} will be forwarded to ${upstream}`,
    )
  }
  if (draft.upstreamScheme === 'https' && draft.upstreamSkipTlsVerify) {
    lines.push('Certificate errors from this upstream are ignored')
  }
  return lines
}
