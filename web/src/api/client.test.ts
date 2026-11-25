import { describe, expect, it } from 'vitest'
import { ApiError, createApi } from './client'
import { json, mockApi } from '@/test/helpers'

describe('api client', () => {
  it('sends the JSON content type on a POST with no body', async () => {
    const { calls } = mockApi({ 'POST /api/auth/logout': () => json(204) })
    await createApi().logout()
    expect(calls).toHaveLength(1)
    expect(calls[0]?.contentType).toBe('application/json')
  })

  it('sends credentials as a JSON body', async () => {
    const { calls } = mockApi({
      'POST /api/auth/login': () => json(200, { id: 1, username: 'a', role: 'admin' }),
    })
    const user = await createApi().login({ username: 'a', password: 'password1' })
    expect(user.username).toBe('a')
    expect(calls[0]?.contentType).toBe('application/json')
    expect(calls[0]?.body).toEqual({ username: 'a', password: 'password1' })
  })

  it('does not force a content type on GET', async () => {
    const { calls } = mockApi({
      'GET /api/status': () => json(200, { setupRequired: true, version: 'x' }),
    })
    await createApi().getStatus()
    expect(calls[0]?.contentType).toBeNull()
  })

  it('maps {error, field} to an ApiError', async () => {
    mockApi({
      'POST /api/setup': () => json(400, { error: 'password too short', field: 'password' }),
    })
    const err = await createApi()
      .setup({ username: 'a', password: 'x' })
      .catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 400, message: 'password too short', field: 'password' })
  })

  it('maps an error without a JSON body to a generic ApiError', async () => {
    mockApi({ 'GET /api/status': () => new Response('Bad Gateway', { status: 502 }) })
    const err = await createApi()
      .getStatus()
      .catch((e: unknown) => e)
    expect(err).toMatchObject({ status: 502 })
  })

  it('maps a network failure to status 0', async () => {
    const { fetchMock } = mockApi({})
    fetchMock.mockRejectedValue(new TypeError('failed to fetch'))
    const err = await createApi()
      .getStatus()
      .catch((e: unknown) => e)
    expect(err).toMatchObject({ status: 0 })
  })

  it('getMe answers null on 401 and throws on other errors', async () => {
    mockApi({ 'GET /api/auth/me': () => json(401, { error: 'unauthorized' }) })
    expect(await createApi().getMe()).toBeNull()
    mockApi({ 'GET /api/auth/me': () => json(500, { error: 'internal error' }) })
    await expect(createApi().getMe()).rejects.toMatchObject({ status: 500 })
  })
})
