import { VueQueryPlugin } from '@tanstack/vue-query'
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import CredentialsForm from './CredentialsForm.vue'
import { useAuthStore } from '@/stores/auth'
import { admin, json, mockApi, settle } from '@/test/helpers'

function mountForm(mode: 'setup' | 'login') {
  const pinia = createPinia()
  const wrapper = mount(CredentialsForm, {
    props: { mode },
    global: { plugins: [pinia, VueQueryPlugin] },
  })
  const fill = async (username: string, password: string, confirm?: string) => {
    const inputs = wrapper.findAll('input')
    await inputs[0]!.setValue(username)
    await inputs[1]!.setValue(password)
    if (confirm !== undefined) await inputs[2]!.setValue(confirm)
  }
  const submit = async () => {
    await wrapper.find('form').trigger('submit')
    await settle()
  }
  return { wrapper, pinia, fill, submit }
}

describe('login form', () => {
  it('shows validation messages and sends nothing', async () => {
    const { calls } = mockApi({})
    const { wrapper, fill, submit } = mountForm('login')
    await fill('   ', 'short')
    await submit()
    expect(wrapper.text()).toContain('Enter a username')
    expect(wrapper.text()).toContain('At least 8 characters')
    expect(calls).toHaveLength(0)
  })

  it('has no confirmation field', () => {
    mockApi({})
    expect(mountForm('login').wrapper.findAll('input')).toHaveLength(2)
  })

  it('signs in with the trimmed username, stores the user and emits done', async () => {
    const { calls } = mockApi({ 'POST /api/auth/login': () => json(200, admin) })
    const { wrapper, pinia, fill, submit } = mountForm('login')
    await fill('  admin  ', 'password1')
    await submit()
    expect(calls[0]?.body).toEqual({ username: 'admin', password: 'password1' })
    expect(useAuthStore(pinia).user).toEqual(admin)
    expect(wrapper.emitted('done')).toHaveLength(1)
  })

  it('shows "invalid username or password" on 401', async () => {
    mockApi({ 'POST /api/auth/login': () => json(401, { error: 'invalid username or password' }) })
    const { wrapper, fill, submit } = mountForm('login')
    await fill('admin', 'wrongpass1')
    await submit()
    expect(wrapper.get('[role=alert]').text()).toBe('invalid username or password')
    expect(wrapper.emitted('done')).toBeUndefined()
  })

  it('shows a generic message for an unexpected failure', async () => {
    mockApi({ 'POST /api/auth/login': () => json(500, { error: 'internal error' }) })
    const { wrapper, fill, submit } = mountForm('login')
    await fill('admin', 'password1')
    await submit()
    expect(wrapper.get('[role=alert]').text()).toBe('Something went wrong. Try again.')
  })
})

describe('setup form', () => {
  it('requires matching passwords', async () => {
    const { calls } = mockApi({})
    const { wrapper, fill, submit } = mountForm('setup')
    await fill('admin', 'password1', 'password2')
    await submit()
    expect(wrapper.text()).toContain('Passwords do not match')
    expect(calls).toHaveLength(0)
  })

  it('rejects a password over 72 characters and a username over 64', async () => {
    mockApi({})
    const { wrapper, fill, submit } = mountForm('setup')
    await fill('a'.repeat(65), 'p'.repeat(73), 'p'.repeat(73))
    await submit()
    expect(wrapper.text()).toContain('At most 64 characters')
    expect(wrapper.text()).toContain('At most 72 characters')
  })

  it('creates the admin and does not send the confirmation', async () => {
    const { calls } = mockApi({ 'POST /api/setup': () => json(201, admin) })
    const { wrapper, pinia, fill, submit } = mountForm('setup')
    await fill('admin', 'password1', 'password1')
    await submit()
    expect(calls[0]?.body).toEqual({ username: 'admin', password: 'password1' })
    expect(calls[0]?.contentType).toBe('application/json')
    expect(useAuthStore(pinia).user).toEqual(admin)
    expect(wrapper.emitted('done')).toHaveLength(1)
  })

  it('shows a server validation error on the field it names', async () => {
    mockApi({
      'POST /api/setup': () =>
        json(400, { error: 'password must be 8 to 72 bytes', field: 'password' }),
    })
    const { wrapper, fill, submit } = mountForm('setup')
    await fill('admin', 'password1', 'password1')
    await submit()
    expect(wrapper.text()).toContain('password must be 8 to 72 bytes')
    expect(wrapper.emitted('done')).toBeUndefined()
  })

  it('shows the server message when setup was already done (409)', async () => {
    mockApi({ 'POST /api/setup': () => json(409, { error: 'setup already completed' }) })
    const { wrapper, fill, submit } = mountForm('setup')
    await fill('admin', 'password1', 'password1')
    await submit()
    expect(wrapper.get('[role=alert]').text()).toBe('setup already completed')
  })
})
