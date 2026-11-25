import { existsSync } from 'node:fs'
import { defineConfig } from '@playwright/test'

// In CI the job installs Playwright's own Chromium. Where a browser is already
// installed (the sandbox), point at it instead.
const sandboxChromium = '/opt/pw-browsers/chromium'
const executablePath =
  process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ??
  (existsSync(sandboxChromium) ? sandboxChromium : undefined)

export default defineConfig({
  testDir: 'e2e',
  testMatch: '*.spec.ts',
  // One Panel, one database: the tests share state, so run them in order.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: process.env.HPM_E2E_URL,
    headless: true,
    viewport: { width: 1280, height: 800 },
    launchOptions: { executablePath, args: ['--no-sandbox'] },
  },
})
