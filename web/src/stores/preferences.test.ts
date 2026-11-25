import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { THEME_KEY, usePreferencesStore } from './preferences'
import { setSystemDark } from '@/test/media'

const isDark = () => document.documentElement.classList.contains('dark')
const fresh = () => {
  setActivePinia(createPinia())
  return usePreferencesStore()
}

describe('theme preference', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('follows the system setting by default', async () => {
    const prefs = fresh()
    expect(prefs.mode).toBe('system')
    await Promise.resolve()
    expect(isDark()).toBe(false)
    setSystemDark(true)
    await Promise.resolve()
    expect(isDark()).toBe(true)
  })

  it('lets a manual choice override the system and persists it', async () => {
    const prefs = fresh()
    setSystemDark(true)
    prefs.setMode('light')
    await Promise.resolve()
    expect(isDark()).toBe(false)
    expect(localStorage.getItem(THEME_KEY)).toBe('light')
    prefs.setMode('dark')
    await Promise.resolve()
    expect(isDark()).toBe(true)
    expect(localStorage.getItem(THEME_KEY)).toBe('dark')
  })

  it('removes the stored value when going back to system', () => {
    const prefs = fresh()
    prefs.setMode('dark')
    prefs.setMode('system')
    expect(localStorage.getItem(THEME_KEY)).toBeNull()
  })

  it('restores the stored choice on the next load', async () => {
    localStorage.setItem(THEME_KEY, 'dark')
    const prefs = fresh()
    expect(prefs.mode).toBe('dark')
    await Promise.resolve()
    expect(isDark()).toBe(true)
  })

  it('works when localStorage throws', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const prefs = fresh()
    expect(prefs.mode).toBe('system')
    prefs.setMode('dark')
    await Promise.resolve()
    expect(prefs.mode).toBe('dark')
    expect(isDark()).toBe(true)
  })
})
