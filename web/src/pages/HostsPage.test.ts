import { describe, expect, it } from 'vitest'
import { admin, json, mockApi, mountApp, settle } from '@/test/helpers'
import { host } from '@/test/hosts'

const signedIn = {
  'GET /api/status': () => json(200, { setupRequired: false, version: 'test' }),
  'GET /api/auth/me': () => json(200, admin),
}

describe('HostsPage', () => {
  it('loads the hosts into the table', async () => {
    mockApi({ ...signedIn, 'GET /api/hosts': () => json(200, [host({ id: 1 })]) })
    const { wrapper } = await mountApp('/hosts')
    await settle()
    expect(wrapper.find('h1').text()).toBe('Proxy Hosts')
    expect(wrapper.text()).toContain('app.example.com')
  })

  it('shows the empty state for an empty list', async () => {
    mockApi({ ...signedIn, 'GET /api/hosts': () => json(200, []) })
    const { wrapper } = await mountApp('/hosts')
    await settle()
    expect(wrapper.text()).toContain('No proxy hosts yet')
  })

  it('shows an error when the list cannot be loaded', async () => {
    mockApi({ ...signedIn, 'GET /api/hosts': () => json(500, { error: 'internal error' }) })
    const { wrapper } = await mountApp('/hosts')
    await settle()
    expect(wrapper.get('[role=alert]').text()).toContain('internal error')
  })
})
