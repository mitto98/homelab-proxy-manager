import { describe, expect, it, vi } from 'vitest'
import { navItems } from '@/nav'
import { admin, json, mockApi, mountApp, settle } from '@/test/helpers'
import { THEME_KEY } from '@/stores/preferences'

const signedIn = () => ({
  'GET /api/status': () => json(200, { setupRequired: false, version: 'test' }),
  'GET /api/auth/me': () => json(200, admin),
})

describe('app shell', () => {
  it('lists one nav link per entry of the nav data, and nothing else', async () => {
    mockApi(signedIn())
    const { wrapper } = await mountApp('/hosts')
    const links = wrapper.findAll('nav a')
    expect(links.map((l) => l.text())).toEqual(navItems.map((n) => n.label))
    expect(links.map((l) => l.attributes('href'))).toEqual(navItems.map((n) => n.to))
  })

  it('shows the signed-in user name and a page header', async () => {
    mockApi(signedIn())
    const { wrapper } = await mountApp('/hosts')
    expect(wrapper.get('[data-testid=user-name]').text()).toBe('admin')
    expect(wrapper.get('h1').text()).toBe('Proxy Hosts')
  })

  it('signs out with a JSON content type and lands on the login page', async () => {
    const { calls } = mockApi({
      ...signedIn(),
      'POST /api/auth/logout': () => json(204),
    })
    const { wrapper, router } = await mountApp('/hosts')
    const button = wrapper.findAll('button').find((b) => b.text().includes('Sign out'))!
    await button.trigger('click')
    await settle()
    const logout = calls.find((c) => c.path === '/api/auth/logout')
    expect(logout?.contentType).toBe('application/json')
    // The login page is a lazy chunk, so wait for the navigation to finish.
    await vi.waitFor(() => expect(router.currentRoute.value.name).toBe('login'))
  })

  it('switches theme from the sidebar and remembers it', async () => {
    mockApi(signedIn())
    const { wrapper } = await mountApp('/hosts')
    await wrapper.get('[aria-label="Dark theme"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem(THEME_KEY)).toBe('dark')
    expect(wrapper.get('[aria-label="Dark theme"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('[aria-label="Light theme"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
