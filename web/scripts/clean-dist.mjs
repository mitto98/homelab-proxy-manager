// Empties web/dist before a build but keeps the tracked placeholder.txt, which
// lets `go build` embed dist from a fresh clone (//go:embed needs a file).
import { mkdirSync, readdirSync, rmSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'
import { join } from 'node:path'

const dist = fileURLToPath(new URL('../dist', import.meta.url))
mkdirSync(dist, { recursive: true })
for (const name of readdirSync(dist)) {
  if (name !== 'placeholder.txt') rmSync(join(dist, name), { recursive: true, force: true })
}
