import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// The Go Panel's default listen address when run with HPM_LISTEN=127.0.0.1:81.
const backend = process.env.HPM_DEV_BACKEND ?? 'http://127.0.0.1:81'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    proxy: { '/api': backend },
  },
  build: {
    outDir: 'dist',
    // The tracked dist/placeholder.txt must survive a build, so Vite does not
    // empty the directory; scripts/clean-dist.mjs does, sparing the placeholder.
    emptyOutDir: false,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['src/test/setup.ts'],
  },
})
