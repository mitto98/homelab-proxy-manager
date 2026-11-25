import { afterEach, describe, expect, it } from 'vitest'
import { admin, json, mockApi, mountApp, settle } from '@/test/helpers'
import { host } from '@/test/hosts'

const signedIn = {
  'GET /api/status': () => json(200, { setupRequired: false, version: 'test' }),
  'GET /api/auth/me': () => json(200, admin),
}

// mountApp attaches to the body; do not let one test's app answer the next one's queries.
afterEach(() => {
  document.body.innerHTML = ''
})

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

// The sheet renders in a portal, so drive it through the document.
function click(text: string) {
  const el = [...document.body.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === text,
  )
  if (!el) throw new Error(`no button ${text}`)
  el.click()
}
function fill(label: string, value: string) {
  const l = [...document.body.querySelectorAll('label')].find(
    (x) => x.textContent?.trim() === label,
  )!
  const input = document.getElementById(l.htmlFor) as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('HostsPage management', () => {
  it('adds a host and the table shows it without a reload', async () => {
    let stored = [host({ id: 1 })]
    const { calls } = mockApi({
      ...signedIn,
      'GET /api/hosts': () => json(200, stored),
      'POST /api/hosts': (req) => {
        const body = req.body as Parameters<typeof host>[0]
        stored = [...stored, host({ ...body, id: 2 })]
        return json(201, stored[1])
      },
    })
    const { wrapper } = await mountApp('/hosts')
    await settle()
    expect(wrapper.text()).not.toContain('new.example.com')
    click('Add Proxy Host')
    await settle()
    fill('Upstream host', '10.0.0.5')
    const input = document.getElementById(
      [...document.body.querySelectorAll('label')].find(
        (x) => x.textContent?.trim() === 'Domain names',
      )!.htmlFor,
    ) as HTMLInputElement
    input.value = 'new.example.com'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }),
    )
    await settle()
    click('Save')
    await settle()
    expect(calls.filter((c) => c.method === 'GET' && c.path === '/api/hosts')).toHaveLength(2)
    expect(wrapper.text()).toContain('new.example.com')
    expect(document.body.querySelector('[data-slot=sheet-content]')).toBeNull()
    wrapper.unmount()
  })

  it('opens an edit with the row values and shows the change after saving', async () => {
    let stored = [host({ id: 1, upstreamPort: 8080 })]
    const { calls } = mockApi({
      ...signedIn,
      'GET /api/hosts': () => json(200, stored),
      'PUT /api/hosts/1': (req) => {
        stored = [host({ ...(req.body as object), id: 1 } as Parameters<typeof host>[0])]
        return json(200, stored[0])
      },
    })
    const { wrapper } = await mountApp('/hosts')
    await settle()
    click('Edit')
    await settle()
    expect(document.body.textContent).toContain('Edit Proxy Host')
    fill('Port', '9090')
    await settle()
    click('Save')
    await settle()
    expect(calls.filter((c) => c.method === 'PUT')).toHaveLength(1)
    expect(calls.filter((c) => c.method === 'GET' && c.path === '/api/hosts')).toHaveLength(2)
    expect(wrapper.text()).toContain('http://192.168.1.20:9090')
    wrapper.unmount()
  })

  it('deletes a host after confirming and the row disappears', async () => {
    let stored = [host({ id: 1 }), host({ id: 2, domains: ['other.example.com'] })]
    const { calls } = mockApi({
      ...signedIn,
      'GET /api/hosts': () => json(200, stored),
      'DELETE /api/hosts/2': () => {
        stored = [stored[0]!]
        return json(204)
      },
    })
    const { wrapper } = await mountApp('/hosts')
    await settle()
    document.querySelector<HTMLButtonElement>('[aria-label="Edit other.example.com"]')!.click()
    await settle()
    click('Delete')
    await settle()
    click('Delete host')
    await settle()
    expect(calls.filter((c) => c.method === 'DELETE')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('other.example.com')
    expect(wrapper.text()).toContain('app.example.com')
    wrapper.unmount()
  })
})
