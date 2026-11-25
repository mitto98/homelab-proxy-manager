import { afterEach, beforeEach, vi } from 'vitest'
import { installMatchMedia } from './media'

beforeEach(() => {
  installMatchMedia()
  localStorage.clear()
  document.documentElement.classList.remove('dark')
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
