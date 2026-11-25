import type { Host } from '@/api/client'

/** A Host as the API returns it; override what a test cares about. */
export function host(over: Partial<Host> & { id: number }): Host {
  return {
    kind: 'proxy',
    enabled: true,
    domains: ['app.example.com'],
    upstreamScheme: 'http',
    upstreamHost: '192.168.1.20',
    upstreamPort: 8080,
    upstreamSkipTlsVerify: false,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  }
}
