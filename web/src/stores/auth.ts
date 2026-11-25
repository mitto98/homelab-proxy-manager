import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { User } from '@/api/client'

/** The signed-in user. Server data other than this lives in TanStack Query. */
export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  // False until the first GET /auth/me has answered, so the router guard knows
  // whether "no user" means "signed out" or "not asked yet".
  const loaded = ref(false)

  function setUser(u: User | null) {
    user.value = u
    loaded.value = true
  }

  return { user, loaded, setUser }
})
