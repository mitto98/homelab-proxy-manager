import createClient, { type Middleware } from 'openapi-fetch'
import type { components, paths } from './schema'

export type User = components['schemas']['User']
export type Credentials = components['schemas']['Credentials']
export type Status = components['schemas']['Status']
export type Host = components['schemas']['Host']
export type HostInput = components['schemas']['HostInput']

/** An error answer from the API (`{error, field?}`), or a failure to get one. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    /** The request field a validation error is about, when the server names one. */
    readonly field?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

// The server answers 415 to any state-changing request without a JSON content
// type, including bodiless ones such as logout. openapi-fetch only sets the
// header when there is a body, so set it for every non-GET request here.
const jsonContentType: Middleware = {
  onRequest({ request }) {
    if (
      request.method !== 'GET' &&
      request.method !== 'HEAD' &&
      !request.headers.has('Content-Type')
    ) {
      request.headers.set('Content-Type', 'application/json')
    }
    return request
  },
}

export function createApi(fetchImpl: typeof fetch = (...args) => globalThis.fetch(...args)) {
  const client = createClient<paths>({
    // Absolute because Request (in Node, under test) rejects relative URLs.
    baseUrl: `${globalThis.location.origin}/api`,
    credentials: 'same-origin',
    fetch: fetchImpl,
  })
  client.use(jsonContentType)

  /** Resolves with the data or throws an ApiError. */
  async function run<T>(
    call: Promise<{ data?: T; error?: unknown; response: Response }>,
  ): Promise<T> {
    let result
    try {
      result = await call
    } catch {
      throw new ApiError(0, 'Could not reach the server')
    }
    if (result.error !== undefined) {
      const e = result.error as Partial<components['schemas']['Error']> | undefined
      throw new ApiError(
        result.response.status,
        typeof e?.error === 'string' ? e.error : `Unexpected response (${result.response.status})`,
        e?.field,
      )
    }
    return result.data as T
  }

  return {
    getStatus: () => run(client.GET('/status')),
    /** The signed-in user, or null when there is no valid session. */
    async getMe(): Promise<User | null> {
      try {
        return await run(client.GET('/auth/me'))
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
    setup: (body: Credentials) => run(client.POST('/setup', { body })),
    login: (body: Credentials) => run(client.POST('/auth/login', { body })),
    logout: () => run(client.POST('/auth/logout')),
    listHosts: () => run(client.GET('/hosts')),
    createHost: (body: HostInput) => run(client.POST('/hosts', { body })),
    updateHost: (id: number, body: HostInput) =>
      run(client.PUT('/hosts/{id}', { params: { path: { id } }, body })),
    deleteHost: (id: number) => run(client.DELETE('/hosts/{id}', { params: { path: { id } } })),
  }
}

export type Api = ReturnType<typeof createApi>

export const api = createApi()
