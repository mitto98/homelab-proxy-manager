import { VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import App from '@/App.vue'
import { api } from '@/api/client'
import { createQueryClient } from '@/query'
import { createAppRouter } from '@/router'

export interface Recorded {
  method: string
  path: string
  contentType: string | null
  body: unknown
}

type Handler = (req: Recorded) => Response
export const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
  })

/**
 * Replaces fetch with a fake API. Routes are keyed "METHOD /api/path"; a
 * request with no route fails the test loudly with a 500.
 */
export function mockApi(routes: Record<string, Handler>) {
  const calls: Recorded[] = []
  const fetchMock = vi.fn(async (input: Request) => {
    const url = new URL(input.url, 'http://localhost')
    const text = await input.text()
    const rec: Recorded = {
      method: input.method,
      path: url.pathname,
      contentType: input.headers.get('content-type'),
      body: text ? JSON.parse(text) : undefined,
    }
    calls.push(rec)
    const handler = routes[`${rec.method} ${rec.path}`]
    return handler ? handler(rec) : json(500, { error: `no mock for ${rec.method} ${rec.path}` })
  })
  vi.stubGlobal('fetch', fetchMock)
  return { calls, fetchMock }
}

export const admin = { id: 1, username: 'admin', role: 'admin' as const }

/** Mounts the whole app on a memory router at `path`. */
export async function mountApp(path: string) {
  const pinia = createPinia()
  setActivePinia(pinia) // the guard runs on the first push, before the app installs pinia
  const queryClient = createQueryClient()
  const router = createAppRouter(createMemoryHistory(), queryClient, api)
  await router.push(path)
  const wrapper = mount(App, {
    global: { plugins: [pinia, [VueQueryPlugin, { queryClient }], router] },
    attachTo: document.body,
  })
  await router.isReady()
  await flushPromises()
  return { wrapper, router, pinia, queryClient }
}

/** vee-validate validates and TanStack Query mutates asynchronously; let both finish. */
export async function settle() {
  await new Promise((r) => setTimeout(r, 30))
  await flushPromises()
}
