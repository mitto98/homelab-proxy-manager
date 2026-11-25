import { describe, expect, it } from 'vitest'
import { describeHost } from './summary'

const complete = {
  domains: ['app.example.com'],
  upstreamScheme: 'http',
  upstreamHost: '192.168.1.20',
  upstreamPort: 8080,
  upstreamSkipTlsVerify: false,
  enabled: true,
}

describe('describeHost', () => {
  it('says where one domain is forwarded', () => {
    expect(describeHost(complete)).toEqual([
      'Requests for app.example.com are forwarded to http://192.168.1.20:8080',
    ])
  })

  it('lists several domains in plain language', () => {
    const two = describeHost({ ...complete, domains: ['app.example.com', 'www.example.com'] })
    expect(two[0]).toBe(
      'Requests for app.example.com and www.example.com are forwarded to http://192.168.1.20:8080',
    )
    const three = describeHost({
      ...complete,
      domains: ['a.example.com', 'b.example.com', 'c.example.com'],
    })
    expect(three[0]).toContain('a.example.com, b.example.com and c.example.com are forwarded')
  })

  it('mentions ignored certificate errors only for https with skip on', () => {
    const https = { ...complete, upstreamScheme: 'https', upstreamPort: 443 }
    expect(describeHost(https)).toEqual([
      'Requests for app.example.com are forwarded to https://192.168.1.20:443',
    ])
    expect(describeHost({ ...https, upstreamSkipTlsVerify: true })).toEqual([
      'Requests for app.example.com are forwarded to https://192.168.1.20:443',
      'Certificate errors from this upstream are ignored',
    ])
    // A stale flag on an http upstream means nothing.
    expect(describeHost({ ...complete, upstreamSkipTlsVerify: true })).toHaveLength(1)
  })

  it('says a disabled host is not routed and what it would do once enabled', () => {
    expect(describeHost({ ...complete, enabled: false })).toEqual([
      'This host is disabled: Traefik does not route its domains',
      'Once enabled, requests for app.example.com will be forwarded to http://192.168.1.20:8080',
    ])
  })

  it('hints at what is missing while the form is incomplete', () => {
    expect(describeHost({ ...complete, domains: [], upstreamHost: '', upstreamPort: '' })).toEqual([
      'Add a domain name, an upstream host and an upstream port to see where requests will go',
    ])
    expect(describeHost({ ...complete, upstreamHost: '  ' })).toEqual([
      'Add an upstream host to see where requests will go',
    ])
    expect(describeHost({ ...complete, domains: [], upstreamPort: 70000 })).toEqual([
      'Add a domain name and an upstream port to see where requests will go',
    ])
    expect(describeHost({})[0]).toContain('Add a domain name')
  })

  it('never mentions certificates on the public side', () => {
    const text = describeHost({ ...complete, enabled: false }).join(' ')
    expect(text).not.toMatch(/ssl|redirect|hsts/i)
  })
})
