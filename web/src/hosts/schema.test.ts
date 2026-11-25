import { describe, expect, it } from 'vitest'
import {
  formValuesToInput,
  hostFormSchema,
  hostToFormValues,
  newHostValues,
  type HostFormValues,
} from './schema'
import { host } from '@/test/hosts'

const valid: HostFormValues = {
  domains: ['app.example.com'],
  upstreamScheme: 'http',
  upstreamHost: '192.168.1.20',
  upstreamPort: 8080,
  upstreamSkipTlsVerify: false,
  enabled: true,
}
const messages = (v: unknown) => {
  const r = hostFormSchema.safeParse(v)
  return r.success ? [] : r.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`)
}

describe('hostFormSchema', () => {
  it('accepts a valid host', () => {
    expect(messages(valid)).toEqual([])
  })

  it('needs a domain, and no empty ones', () => {
    expect(messages({ ...valid, domains: [] })).toEqual(['domains: Add at least one domain'])
    expect(messages({ ...valid, domains: ['  '] })).toEqual([
      'domains.0: A domain must not be empty',
    ])
  })

  it('needs a known scheme and a non-empty upstream host', () => {
    expect(messages({ ...valid, upstreamScheme: 'ftp' })).toHaveLength(1)
    expect(messages({ ...valid, upstreamHost: '   ' })).toEqual([
      'upstreamHost: Enter the upstream host',
    ])
  })

  it('needs an integer port from 1 to 65535, also from a number input string', () => {
    for (const port of [0, 65536, -1, '']) {
      expect(messages({ ...valid, upstreamPort: port })).toEqual([
        'upstreamPort: The port must be between 1 and 65535',
      ])
    }
    expect(messages({ ...valid, upstreamPort: 1.5 })).toEqual([
      'upstreamPort: The port must be a whole number',
    ])
    expect(messages({ ...valid, upstreamPort: '8080' })).toEqual([])
    expect(messages({ ...valid, upstreamPort: 65535 })).toEqual([])
  })
})

describe('form values', () => {
  it('starts a new host as http, port 80, enabled, no domains', () => {
    expect(hostToFormValues(null)).toEqual(newHostValues)
    expect(newHostValues).toMatchObject({
      upstreamScheme: 'http',
      upstreamPort: 80,
      enabled: true,
      domains: [],
    })
  })

  it('does not share the domains array between new forms', () => {
    expect(hostToFormValues(null).domains).not.toBe(hostToFormValues(null).domains)
  })

  it('copies an existing host without its server-only fields', () => {
    expect(hostToFormValues(host({ id: 9, enabled: false }))).toEqual({ ...valid, enabled: false })
  })

  it('builds the request with every field, trimming the host', () => {
    expect(formValuesToInput({ ...valid, upstreamHost: ' nas ' })).toEqual({
      ...valid,
      upstreamHost: 'nas',
    })
  })

  it('never asks to skip verification for http', () => {
    expect(formValuesToInput({ ...valid, upstreamSkipTlsVerify: true }).upstreamSkipTlsVerify).toBe(
      false,
    )
    expect(
      formValuesToInput({ ...valid, upstreamScheme: 'https', upstreamSkipTlsVerify: true })
        .upstreamSkipTlsVerify,
    ).toBe(true)
  })
})
