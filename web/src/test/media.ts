// jsdom has no matchMedia. This stub reports the system colour scheme and lets
// a test flip it.
const listeners = new Set<(e: { matches: boolean }) => void>()
let systemDark = false

export function setSystemDark(value: boolean) {
  systemDark = value
  listeners.forEach((l) => l({ matches: value }))
}

export function installMatchMedia() {
  systemDark = false
  listeners.clear()
  window.matchMedia = ((query: string) => ({
    media: query,
    get matches() {
      return systemDark
    },
    addEventListener: (_: string, l: (e: { matches: boolean }) => void) => listeners.add(l),
    removeEventListener: (_: string, l: (e: { matches: boolean }) => void) => listeners.delete(l),
  })) as unknown as typeof window.matchMedia
}
