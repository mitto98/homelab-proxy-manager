import { defineStore } from 'pinia'
import { computed, ref, watchEffect } from 'vue'

export type ThemeMode = 'system' | 'light' | 'dark'

// Keep the key in sync with the inline script in index.html.
export const THEME_KEY = 'hpm.theme'

// localStorage can throw (blocked storage, private windows), so the theme must
// work without it.
function readStored(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY)
    return v === 'light' || v === 'dark' ? v : 'system'
  } catch {
    return 'system'
  }
}

function persist(mode: ThemeMode) {
  try {
    if (mode === 'system') localStorage.removeItem(THEME_KEY)
    else localStorage.setItem(THEME_KEY, mode)
  } catch {
    // Not persisted; the choice still applies for this page view.
  }
}

/** UI preferences. */
export const usePreferencesStore = defineStore('preferences', () => {
  const mode = ref<ThemeMode>(readStored())

  const query =
    typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)')
      : null
  const systemDark = ref(query?.matches ?? false)
  query?.addEventListener('change', (e) => (systemDark.value = e.matches))

  const dark = computed(() => (mode.value === 'system' ? systemDark.value : mode.value === 'dark'))

  function setMode(m: ThemeMode) {
    mode.value = m
    persist(m)
  }

  watchEffect(() => document.documentElement.classList.toggle('dark', dark.value))

  return { mode, dark, setMode }
})
