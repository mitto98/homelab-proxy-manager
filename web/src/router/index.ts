import type { QueryClient } from '@tanstack/vue-query'
import { createRouter, type RouterHistory } from 'vue-router'
import type { Api } from '@/api/client'
import { fetchStatus } from '@/query'
import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    /** Reachable without a session (login and setup). */
    guest?: boolean
    title?: string
  }
}

/** Only follow redirects to a path inside this app. */
export function safeRedirect(value: unknown): string {
  return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') ? value : '/'
}

export function createAppRouter(history: RouterHistory, qc: QueryClient, api: Api) {
  const router = createRouter({
    history,
    routes: [
      {
        path: '/setup',
        name: 'setup',
        component: () => import('@/pages/SetupPage.vue'),
        meta: { guest: true },
      },
      {
        path: '/login',
        name: 'login',
        component: () => import('@/pages/LoginPage.vue'),
        meta: { guest: true },
      },
      {
        path: '/',
        component: () => import('@/layouts/AppShell.vue'),
        children: [
          { path: '', redirect: '/hosts' },
          {
            path: 'hosts',
            name: 'hosts',
            component: () => import('@/pages/HostsPage.vue'),
            meta: { title: 'Proxy Hosts' },
          },
        ],
      },
      // The polished 404 page is slice [6.4]; until then unknown paths go home.
      { path: '/:pathMatch(.*)*', redirect: '/' },
    ],
  })

  router.beforeEach(async (to) => {
    const status = await fetchStatus(qc, api)

    // While no user exists the only reachable page is setup.
    if (status.setupRequired) return to.name === 'setup' ? true : { name: 'setup' }
    if (to.name === 'setup') return { path: '/' }

    const auth = useAuthStore()
    if (!auth.loaded) auth.setUser(await api.getMe())

    if (!auth.user) {
      return to.meta.guest ? true : { name: 'login', query: { redirect: to.fullPath } }
    }
    return to.meta.guest ? { path: safeRedirect(to.query.redirect) } : true
  })

  return router
}
