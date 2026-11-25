import { VueQueryPlugin } from '@tanstack/vue-query'
import { DOMWrapper, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import HostSheet from './HostSheet.vue'
import type { Host } from '@/api/client'
import { createQueryClient } from '@/query'
import { json, mockApi, settle } from '@/test/helpers'
import { host } from '@/test/hosts'

// The sheet and the dialogs render in portals, so look in the whole document.
const dom = (sel: string) => {
  const el = document.body.querySelector(sel)
  if (!el) throw new Error(`nothing matches ${sel}`)
  return new DOMWrapper(el)
}
const byLabel = (text: string) => {
  const label = [...document.body.querySelectorAll('label')].find(
    (l) => l.textContent?.trim() === text,
  )
  const el = label && document.getElementById(label.htmlFor)
  if (!el) throw new Error(`no field labelled ${text}`)
  return new DOMWrapper(el as HTMLElement)
}
const button = (text: string) => {
  const el = [...document.body.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === text,
  )
  if (!el) throw new Error(`no button ${text}`)
  return new DOMWrapper(el)
}
const hasButton = (text: string) =>
  [...document.body.querySelectorAll('button')].some((b) => b.textContent?.trim() === text)
const chips = () =>
  [...document.body.querySelectorAll('[data-slot=sheet-content] [data-reka-collection-item]')].map(
    (e) => e.textContent?.trim(),
  )
const text = () => document.body.textContent ?? ''

let wrapper: ReturnType<typeof mountSheet>
function mountSheet(h: Host | null = null) {
  return mount(HostSheet, {
    props: { open: true, host: h },
    attachTo: document.body,
    global: { plugins: [[VueQueryPlugin, { queryClient: createQueryClient() }]] },
  })
}
async function open(h: Host | null = null) {
  wrapper = mountSheet(h)
  await settle()
}
afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
})

/** Types a domain the way a user does: text, then a key. */
async function addDomain(value: string, key = 'Enter') {
  const input = byLabel('Domain names')
  await input.setValue(value)
  await input.trigger('keydown', { key })
  await settle()
}
async function press(key: string) {
  document.body.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
  await settle()
}

const created = host({ id: 7, domains: ['app.example.com'] })

describe('opening', () => {
  it('starts a new host with the defaults, nothing to save and no unsaved marker', async () => {
    await open()
    expect(text()).toContain('Add Proxy Host')
    expect(chips()).toEqual([])
    expect((byLabel('Scheme').element as HTMLSelectElement).value).toBe('http')
    expect((byLabel('Port').element as HTMLInputElement).value).toBe('80')
    expect(byLabel('Enabled').attributes('aria-checked')).toBe('true')
    expect(hasButton('Delete')).toBe(false)
    expect(button('Save').attributes('disabled')).toBeDefined()
    expect(text()).not.toContain('Unsaved changes')
  })

  it('opens an existing host with its values and a Delete button', async () => {
    await open(
      host({
        id: 3,
        domains: ['nas.example.com', 'files.example.com'],
        upstreamScheme: 'https',
        upstreamHost: 'nas',
        upstreamPort: 5001,
        upstreamSkipTlsVerify: true,
        enabled: false,
      }),
    )
    expect(text()).toContain('Edit Proxy Host')
    expect(chips()).toEqual(['nas.example.com', 'files.example.com'])
    expect((byLabel('Scheme').element as HTMLSelectElement).value).toBe('https')
    expect((byLabel('Upstream host').element as HTMLInputElement).value).toBe('nas')
    expect((byLabel('Port').element as HTMLInputElement).value).toBe('5001')
    expect(byLabel('Skip upstream TLS verification').attributes('aria-checked')).toBe('true')
    expect(byLabel('Enabled').attributes('aria-checked')).toBe('false')
    expect(hasButton('Delete')).toBe(true)
  })

  it('has separated, titled sections', async () => {
    await open()
    const sections = [...document.body.querySelectorAll('section')]
    expect(sections.map((s) => s.querySelector('h3')?.textContent)).toEqual([
      'Details',
      'What will happen',
    ])
    for (const s of sections) expect(s.className).toContain('border')
  })
})

describe('dirty state', () => {
  it('enables Save and shows the marker once changed, and clears both on reverting', async () => {
    await open(created)
    expect(button('Save').attributes('disabled')).toBeDefined()
    await byLabel('Upstream host').setValue('10.0.0.9')
    await settle()
    expect(button('Save').attributes('disabled')).toBeUndefined()
    expect(text()).toContain('Unsaved changes')
    await byLabel('Upstream host').setValue(created.upstreamHost)
    await settle()
    expect(button('Save').attributes('disabled')).toBeDefined()
    expect(text()).not.toContain('Unsaved changes')
  })

  it('counts a changed chip list as dirty and reverting it as clean', async () => {
    await open(created)
    await addDomain('www.example.com')
    expect(button('Save').attributes('disabled')).toBeUndefined()
    await dom('button[aria-label="Remove www.example.com"]').trigger('click')
    await settle()
    expect(button('Save').attributes('disabled')).toBeDefined()
  })
})

describe('domain chips', () => {
  it('adds on Enter, trimmed and lower-cased, and clears the input', async () => {
    await open()
    await addDomain('  App.Example.COM ')
    expect(chips()).toEqual(['app.example.com'])
    expect((byLabel('Domain names').element as HTMLInputElement).value).toBe('')
  })

  it('adds on a comma and on a space', async () => {
    await open()
    const input = byLabel('Domain names')
    for (const [typed, delimiter] of [
      ['a.example.com,', ','],
      ['b.example.com ', ' '],
    ] as const) {
      ;(input.element as HTMLInputElement).value = typed
      await input.trigger('input', { data: delimiter })
      await settle()
    }
    expect(chips()).toEqual(['a.example.com', 'b.example.com'])
  })

  it('adds on blur', async () => {
    await open()
    await byLabel('Domain names').setValue('blur.example.com')
    await byLabel('Domain names').trigger('blur')
    await settle()
    expect(chips()).toEqual(['blur.example.com'])
  })

  it('ignores blank input and refuses duplicates, saying so', async () => {
    await open()
    await addDomain('   ')
    expect(chips()).toEqual([])
    await addDomain('a.example.com')
    await addDomain('A.example.com')
    expect(chips()).toEqual(['a.example.com'])
    expect(text()).toContain('a.example.com is already in the list')
  })

  it('removes a chip with its button', async () => {
    await open(host({ id: 1, domains: ['a.example.com', 'b.example.com'] }))
    await dom('button[aria-label="Remove a.example.com"]').trigger('click')
    await settle()
    expect(chips()).toEqual(['b.example.com'])
  })

  it('has a labelled input reachable from the label', async () => {
    await open()
    expect(byLabel('Domain names').element.tagName).toBe('INPUT')
  })
})

describe('skip upstream TLS verification', () => {
  it('shows only for https and is cleared when switching back to http', async () => {
    await open(created)
    const hasSkip = () => text().includes('Skip upstream TLS verification')
    expect(hasSkip()).toBe(false)
    await byLabel('Scheme').setValue('https')
    await settle()
    expect(hasSkip()).toBe(true)
    await byLabel('Skip upstream TLS verification').trigger('click')
    await settle()
    expect(byLabel('Skip upstream TLS verification').attributes('aria-checked')).toBe('true')
    await byLabel('Scheme').setValue('http')
    await settle()
    expect(hasSkip()).toBe(false)
    await byLabel('Scheme').setValue('https')
    await settle()
    expect(byLabel('Skip upstream TLS verification').attributes('aria-checked')).toBe('false')
  })
})

describe('validation', () => {
  it('shows the messages, sends nothing and keeps the form open', async () => {
    const { calls } = mockApi({})
    await open()
    await byLabel('Port').setValue('')
    await settle()
    await dom('form').trigger('submit')
    await settle()
    expect(text()).toContain('Add at least one domain')
    expect(text()).toContain('Enter the upstream host')
    expect(text()).toContain('The port must be between 1 and 65535')
    expect(calls).toHaveLength(0)
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('rejects an out-of-range or fractional port', async () => {
    mockApi({})
    await open(created)
    for (const [value, message] of [
      ['70000', 'The port must be between 1 and 65535'],
      ['80.5', 'The port must be a whole number'],
    ] as const) {
      await byLabel('Port').setValue(value)
      await dom('form').trigger('submit')
      await settle()
      expect(text()).toContain(message)
    }
  })
})

describe('requests', () => {
  it('creates with the exact payload, defaults included, then closes', async () => {
    const { calls } = mockApi({ 'POST /api/hosts': () => json(201, created) })
    await open()
    await addDomain('App.Example.com')
    await byLabel('Upstream host').setValue('  192.168.1.20 ')
    await settle()
    await button('Save').trigger('click')
    await settle()
    expect(calls).toHaveLength(1)
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/hosts',
      contentType: 'application/json',
    })
    expect(calls[0]!.body).toEqual({
      domains: ['app.example.com'],
      upstreamScheme: 'http',
      upstreamHost: '192.168.1.20',
      upstreamPort: 80,
      upstreamSkipTlsVerify: false,
      enabled: true,
    })
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('updates with the exact payload to the host id', async () => {
    const { calls } = mockApi({ 'PUT /api/hosts/7': () => json(200, created) })
    await open(created)
    await byLabel('Port').setValue('9000')
    await byLabel('Enabled').trigger('click')
    await settle()
    await button('Save').trigger('click')
    await settle()
    expect(calls).toHaveLength(1)
    expect(calls[0]!.body).toEqual({
      domains: ['app.example.com'],
      upstreamScheme: 'http',
      upstreamHost: '192.168.1.20',
      upstreamPort: 9000,
      upstreamSkipTlsVerify: false,
      enabled: false,
    })
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('does not send skip-verify for an http host that had it on https', async () => {
    const { calls } = mockApi({ 'PUT /api/hosts/7': () => json(200, created) })
    await open({
      ...created,
      upstreamScheme: 'https',
      upstreamPort: 443,
      upstreamSkipTlsVerify: true,
    })
    await byLabel('Scheme').setValue('http')
    await settle()
    await button('Save').trigger('click')
    await settle()
    expect(calls[0]!.body).toMatchObject({ upstreamScheme: 'http', upstreamSkipTlsVerify: false })
  })

  it('sends skip-verify for an https host', async () => {
    const { calls } = mockApi({ 'PUT /api/hosts/7': () => json(200, created) })
    await open({ ...created, upstreamScheme: 'https', upstreamPort: 443 })
    await byLabel('Skip upstream TLS verification').trigger('click')
    await settle()
    await button('Save').trigger('click')
    await settle()
    expect(calls[0]!.body).toMatchObject({ upstreamScheme: 'https', upstreamSkipTlsVerify: true })
  })

  it('disables Save and Cancel while a save is running', async () => {
    let release!: () => void
    const gate = new Promise<void>((r) => (release = r))
    mockApi({
      'PUT /api/hosts/7': async () => {
        await gate
        return json(200, created)
      },
    })
    await open(created)
    await byLabel('Port').setValue('9000')
    await settle()
    await button('Save').trigger('click')
    await settle()
    expect(button('Save').attributes('disabled')).toBeDefined()
    expect(button('Cancel').attributes('disabled')).toBeDefined()
    release()
    await settle()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})

describe('server errors', () => {
  async function saveChange(route: string, response: () => Response) {
    mockApi({ [route]: response })
    await open(created)
    await byLabel('Port').setValue('9000')
    await settle()
    await button('Save').trigger('click')
    await settle()
  }
  const messageOf = (label: string) => {
    const id = byLabel(label).attributes('aria-describedby') ?? ''
    return id
      .split(' ')
      .map((i) => document.getElementById(i)?.textContent?.trim() ?? '')
      .join(' ')
      .trim()
  }

  it('shows a 400 on the field it names and keeps the input', async () => {
    await saveChange('PUT /api/hosts/7', () =>
      json(400, { error: 'upstream port must be between 1 and 65535', field: 'upstreamPort' }),
    )
    expect(messageOf('Port')).toBe('upstream port must be between 1 and 65535')
    expect((byLabel('Port').element as HTMLInputElement).value).toBe('9000')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('shows a 400 about the domains under the chip input', async () => {
    await saveChange('PUT /api/hosts/7', () =>
      json(400, { error: '"-x-" is not a valid domain name', field: 'domains' }),
    )
    expect(messageOf('Domain names')).toBe('"-x-" is not a valid domain name')
    expect(messageOf('Upstream host')).toBe('')
  })

  it('shows a 409 on the domains field', async () => {
    await saveChange('PUT /api/hosts/7', () =>
      json(409, {
        error: 'domain "app.example.com" is already used by another host',
        field: 'domains',
      }),
    )
    expect(messageOf('Domain names')).toBe(
      'domain "app.example.com" is already used by another host',
    )
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(chips()).toEqual(['app.example.com'])
  })

  it('shows a 409 on the domains field even without a field name', async () => {
    await saveChange('PUT /api/hosts/7', () => json(409, { error: 'domain taken' }))
    expect(messageOf('Domain names')).toBe('domain taken')
  })

  it('shows a generic message in the form for an unexpected error, keeping input', async () => {
    await saveChange('PUT /api/hosts/7', () => json(500, { error: 'internal error' }))
    expect(dom('[role=alert]').text()).toBe('Something went wrong. Try again.')
    expect((byLabel('Port').element as HTMLInputElement).value).toBe('9000')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(button('Save').attributes('disabled')).toBeUndefined()
  })
})

describe('closing', () => {
  it('Cancel discards without asking', async () => {
    const { calls } = mockApi({})
    await open(created)
    await byLabel('Port').setValue('9000')
    await settle()
    await button('Cancel').trigger('click')
    await settle()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(calls).toHaveLength(0)
    expect(text()).not.toContain('Discard your changes?')
  })

  it('Esc closes a clean form straight away', async () => {
    await open(created)
    await press('Escape')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('Esc on a dirty form asks first; keeping the form open loses nothing', async () => {
    await open(created)
    await byLabel('Port').setValue('9000')
    await settle()
    await press('Escape')
    expect(text()).toContain('Discard your changes?')
    expect(wrapper.emitted('close')).toBeUndefined()
    await button('Keep editing').trigger('click')
    await settle()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(text()).not.toContain('Discard your changes?')
    expect((byLabel('Port').element as HTMLInputElement).value).toBe('9000')
  })

  it('confirming the discard closes the sheet', async () => {
    await open(created)
    await byLabel('Port').setValue('9000')
    await settle()
    await press('Escape')
    await button('Discard').trigger('click')
    await settle()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('a click on the overlay on a dirty form asks first', async () => {
    await open(created)
    await byLabel('Port').setValue('9000')
    await settle()
    dom('[data-slot=sheet-overlay]').element.dispatchEvent(
      new MouseEvent('pointerdown', { bubbles: true }),
    )
    await settle()
    expect(text()).toContain('Discard your changes?')
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})

describe('delete', () => {
  it('asks for confirmation, then deletes and closes', async () => {
    const { calls } = mockApi({ 'DELETE /api/hosts/7': () => json(204) })
    await open(created)
    await button('Delete').trigger('click')
    await settle()
    expect(text()).toContain('Delete this proxy host?')
    expect(calls).toHaveLength(0)
    await button('Delete host').trigger('click')
    await settle()
    expect(calls).toHaveLength(1)
    expect(calls[0]).toMatchObject({
      method: 'DELETE',
      path: '/api/hosts/7',
      contentType: 'application/json',
    })
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('does nothing when the confirmation is declined', async () => {
    const { calls } = mockApi({})
    await open(created)
    await button('Delete').trigger('click')
    await settle()
    await button('Keep host').trigger('click')
    await settle()
    expect(calls).toHaveLength(0)
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(text()).not.toContain('Delete this proxy host?')
  })

  it('shows a failure inside the form and stays open', async () => {
    mockApi({ 'DELETE /api/hosts/7': () => json(404, { error: 'host not found' }) })
    await open(created)
    await button('Delete').trigger('click')
    await settle()
    await button('Delete host').trigger('click')
    await settle()
    expect(dom('[role=alert]').text()).toBe('host not found')
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})

describe('what will happen', () => {
  const summary = () =>
    [...document.body.querySelectorAll('[data-testid=summary] li')].map((l) => l.textContent)

  it('follows the form as it changes', async () => {
    await open()
    expect(summary()).toEqual([
      'Add a domain name and an upstream host to see where requests will go',
    ])
    await addDomain('app.example.com')
    await byLabel('Upstream host').setValue('192.168.1.20')
    await settle()
    expect(summary()).toEqual([
      'Requests for app.example.com are forwarded to http://192.168.1.20:80',
    ])
    await addDomain('www.example.com')
    await byLabel('Port').setValue('8080')
    await byLabel('Scheme').setValue('https')
    await settle()
    await byLabel('Skip upstream TLS verification').trigger('click')
    await byLabel('Enabled').trigger('click')
    await settle()
    expect(summary()).toEqual([
      'This host is disabled: Traefik does not route its domains',
      'Once enabled, requests for app.example.com and www.example.com will be forwarded to https://192.168.1.20:8080',
      'Certificate errors from this upstream are ignored',
    ])
  })
})
