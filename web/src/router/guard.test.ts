import { describe, expect, it } from 'vitest'
import { safeRedirect } from '.'
import { admin, json, mockApi, mountApp, settle } from '@/test/helpers'

const status = (setupRequired: boolean) => () => json(200, { setupRequired, version: 'test' })
const me = (signedIn: boolean) => () =>
  signedIn ? json(200, admin) : json(401, { error: 'unauthorized' })

describe('router guard', () => {
  it('sends everything to /setup while setup is required', async () => {
    mockApi({ 'GET /api/status': status(true) })
    const { router, wrapper } = await mountApp('/hosts')
    expect(router.currentRoute.value.name).toBe('setup')
    expect(wrapper.text()).toContain('Create the first administrator')
  })

  it('sends a signed-out visitor to /login and remembers the intended route', async () => {
    mockApi({ 'GET /api/status': status(false), 'GET /api/auth/me': me(false) })
    const { router } = await mountApp('/hosts')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/hosts')
  })

  it('lets a signed-in user into the app', async () => {
    mockApi({ 'GET /api/status': status(false), 'GET /api/auth/me': me(true) })
    const { router, wrapper } = await mountApp('/hosts')
    expect(router.currentRoute.value.name).toBe('hosts')
    expect(wrapper.text()).toContain('Proxy Hosts')
  })

  it('redirects a signed-in user away from /login and /setup', async () => {
    mockApi({ 'GET /api/status': status(false), 'GET /api/auth/me': me(true) })
    const { router } = await mountApp('/login')
    expect(router.currentRoute.value.name).toBe('hosts')
    await router.push('/setup')
    expect(router.currentRoute.value.name).toBe('hosts')
  })

  it('does not show /setup once setup is done, even when signed out', async () => {
    mockApi({ 'GET /api/status': status(false), 'GET /api/auth/me': me(false) })
    const { router } = await mountApp('/setup')
    expect(router.currentRoute.value.name).toBe('login')
  })

  it('returns to the intended route after signing in', async () => {
    mockApi({
      'GET /api/status': status(false),
      'GET /api/auth/me': me(false),
      'POST /api/auth/login': () => json(200, admin),
    })
    const { router, wrapper } = await mountApp('/hosts')
    await wrapper.find('input[type=text]').setValue('admin')
    await wrapper.find('input[type=password]').setValue('password1')
    await wrapper.find('form').trigger('submit')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/hosts')
  })

  it('only follows redirects inside the app', () => {
    expect(safeRedirect('/hosts')).toBe('/hosts')
    expect(safeRedirect('//evil.example')).toBe('/')
    expect(safeRedirect('https://evil.example')).toBe('/')
    expect(safeRedirect(undefined)).toBe('/')
  })
})
