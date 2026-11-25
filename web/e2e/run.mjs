// Runs the Playwright smoke test against the real binary: builds the frontend,
// builds the Go binary (which embeds web/dist), starts it with an empty data
// directory on a free port, runs the tests, and cleans up.
import { spawn, spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const web = fileURLToPath(new URL('..', import.meta.url))
const root = join(web, '..')
const tmp = mkdtempSync(join(tmpdir(), 'hpm-e2e-'))

function run(cmd, args, cwd) {
  const r = spawnSync(cmd, args, { cwd, stdio: 'inherit' })
  if (r.status !== 0) throw new Error(`${cmd} ${args.join(' ')} failed`)
}

function freePort() {
  return new Promise((resolve, reject) => {
    const s = createServer()
    s.once('error', reject)
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address()
      s.close(() => resolve(port))
    })
  })
}

async function waitReady(url) {
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(`${url}/api/status`)).ok) return
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 100))
  }
  throw new Error('the Panel did not start')
}

let panel
let code = 1
try {
  run('npm', ['run', 'build'], web)
  const bin = join(tmp, 'homelab-proxy-manager')
  run('go', ['build', '-o', bin, './cmd/homelab-proxy-manager'], root)

  const port = await freePort()
  const url = `http://127.0.0.1:${port}`
  const env = { ...process.env, HPM_DATA_DIR: join(tmp, 'data'), HPM_LISTEN: `127.0.0.1:${port}` }
  delete env.HPM_TRAEFIK_CONTROLLER
  panel = spawn(bin, [], { env, stdio: ['ignore', 'inherit', 'inherit'] })
  await waitReady(url)

  const args = ['playwright', 'test', ...process.argv.slice(2)]
  if (!process.env.CI) args.push('--trace', 'off')
  code =
    spawnSync('npx', args, {
      cwd: web,
      stdio: 'inherit',
      env: { ...process.env, HPM_E2E_URL: url },
    }).status ?? 1
} catch (e) {
  console.error(e.message)
} finally {
  panel?.kill()
  rmSync(tmp, { recursive: true, force: true })
}
process.exit(code)
