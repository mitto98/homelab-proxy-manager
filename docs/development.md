# Development

## The API contract

`api/openapi.yaml` is the single source of truth for the HTTP API. Everything
derived from it is generated and committed:

| Output | Generator | Purpose |
|---|---|---|
| `internal/api/api.gen.go` | [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen) (pinned as a `tool` in `go.mod`) | Go types, `/api` routing, the strict server interface, and the embedded spec |
| `web/src/api/schema.d.ts` | [`openapi-typescript`](https://github.com/openapi-ts/openapi-typescript) (pinned in `web/package.json`) | Types for the SPA's API client |

**Changing the API starts in `api/openapi.yaml`.** Then regenerate and commit
the result:

```bash
scripts/generate.sh
```

CI runs the same script and fails if it leaves a diff, so a hand-edited or stale
generated file cannot be merged. Never edit `api.gen.go` or `schema.d.ts`.

Handlers live in `internal/api/handlers.go` and implement the generated
`StrictServerInterface`; a contract change that a handler does not follow is a
compile error.

### Conventions

- **Authentication** is declared in the contract: an operation with `security`
  requires a valid session cookie. The server reads this from the embedded spec
  and enforces it in one middleware (`requireSession` in `internal/api/api.go`);
  handlers never check sessions themselves.
- **Errors** have one shape, `{"error": "message"}`, on every status.
- **State-changing requests** must send `Content-Type: application/json`, even
  without a body (for example logout). The server answers `415` otherwise.
- Request bodies are limited to 1 MiB.

## The frontend (`web/`)

Vue 3, TypeScript and Vite, with Tailwind 4 and shadcn-vue components. Versions
are pinned exactly in `web/package.json`; `web/package-lock.json` is committed.
Run everything from `web/` after `npm ci`.

| Script | What it does |
|---|---|
| `npm run dev` | Vite dev server with hot reload; `/api` is proxied to the Panel |
| `npm run build` | Type-checks, then builds into `web/dist` (embedded by `web/embed.go`) |
| `npm run type-check` | `vue-tsc --noEmit` |
| `npm run lint` / `npm run format` / `npm run format:check` | ESLint (flat config), Prettier |
| `npm test` | Vitest and Vue Test Utils (jsdom); the network is mocked at `fetch` |
| `npm run test:e2e` | Playwright against the real binary (see below) |
| `npm run generate:api` | Regenerates `src/api/schema.d.ts`; use `scripts/generate.sh` |

### Developing against the Go backend

Start the Panel on the port the dev server expects, then the dev server:

```bash
HPM_DATA_DIR=./data HPM_LISTEN=127.0.0.1:81 go run ./cmd/homelab-proxy-manager
cd web && npm run dev          # http://localhost:5173, /api proxied to 127.0.0.1:81
```

Set `HPM_DEV_BACKEND=http://127.0.0.1:8081` to proxy elsewhere. The session
cookie is `SameSite=Lax` and same-origin through the proxy, so signing in works
in dev.

### Structure and conventions

- `src/api/client.ts` is the only place that talks to `/api`: an `openapi-fetch`
  client typed from `schema.d.ts`. It sends `credentials: 'same-origin'`, adds
  `Content-Type: application/json` to every non-GET request (logout has no body
  and the server answers 415 without it) and turns `{error, field}` into an
  `ApiError`.
- Server data goes through TanStack Query. Pinia holds only the signed-in user
  (`stores/auth.ts`) and UI preferences (`stores/preferences.ts`, the theme).
- `src/nav.ts` is the sidebar's list of pages; a new page adds a route in
  `src/router/index.ts` and an entry there.
- `src/router/index.ts` has the guard: setup required goes to `/setup`, a
  signed-out visitor goes to `/login?redirect=...`, and a signed-in user is
  redirected away from `/login` and `/setup`.
- The theme follows `prefers-color-scheme` with a system/light/dark override
  kept in `localStorage` (key `hpm.theme`; storage errors are ignored). The
  inline script in `index.html` applies it before first paint.
- shadcn-vue components live in `src/components/ui` as copied source. They are
  excluded from Prettier and a few lint rules to stay close to upstream. Add
  more with `npx shadcn-vue@latest add <name>` (the registry host was not
  reachable when these slices were written, so they were copied from the
  `unovue/shadcn-vue` repository by hand, `registry/new-york-v4/ui`, with the
  `cn` and `@/registry/...` imports rewritten to `@/lib/utils` and
  `@/components/ui/...`; `components.json` is the CLI config). Components in use:
  button, card, form, input, label, and since slice 2.7 table, badge, sheet,
  alert-dialog, switch, native-select and tags-input. Sheet and AlertDialog need
  `tw-animate-css`, imported in `src/style.css`.

### The Proxy Hosts page

`src/pages/HostsPage.vue` lists hosts (`useQuery(['hosts'])`) in
`components/hosts/HostsTable.vue` and opens `HostSheet.vue`, a right-hand Sheet,
to add or edit one.

- **Table.** TanStack Table for Vue, v9: a table declares the features it uses
  (`tableFeatures`), here only global filtering. The search box filters on a
  Domains column whose text is all the host's domains. Status is `active` or
  `disabled` from `enabled`; Traefik status arrives in stage 6.
- **Server data.** `hosts/queries.ts` holds the query and the create, update and
  delete mutations; each invalidates `['hosts']`, so the table refetches. The form
  works on its own copy of the host and never writes to the cache.
- **The form.** `HostForm.vue` uses vee-validate with the Zod schema in
  `hosts/schema.ts`, which mirrors only the simple rules (at least one domain, a
  non-empty host, a port from 1 to 65535). The server is the final validator.
  Save is disabled until `meta.dirty` (vee-validate compares with the initial
  values, so reverting clears it) and while a request runs. Cancel closes and
  discards. Esc, the overlay and the close button go through `HostSheet.vue`,
  which asks "Discard your changes?" when the form is dirty. The form is created
  when the sheet opens, so closing always discards its state.
- **Server errors.** A `400` carries the request field it is about and is shown
  on that field (`setFieldError`; `domains` under the chip input). A `409` (domain
  used by another host) shows on the domains field. Any other 4xx shows its
  message, and anything else a generic "Something went wrong" message, in an alert
  inside the form. The form stays open and keeps its input in every case.
- **Sections.** Each block of the slide-over is a `components/FormSection.vue`
  (heading, short description, bordered card). A later slice adds its block by
  putting another `FormSection` in `HostForm.vue` (the comment marks the spot:
  SSL in [3.2], Access in [5.3]) with its fields added to the Zod schema, the form
  values in `hosts/schema.ts` and the summary in `hosts/summary.ts`.
- **What will happen.** `hosts/summary.ts` (`describeHost`) is a pure function from
  the form values to plain-language lines; it is unit tested and rendered in its
  own section.
- **Chips.** `DomainsInput.vue` wraps the reka-ui TagsInput: Enter, comma, space,
  blur or paste add a domain, trimmed and lower-cased, without duplicates. The chip
  is rendered as a `span` because reka's default nests a button in a button.
- Unit tests render the sheet into `document.body` (it uses portals) and mock
  `fetch`. The form keeps a field's value when its input is unmounted
  (`keepValuesOnUnmount`): the skip-TLS-verify switch is only rendered for
  `https`, and dropping its value would make the schema reject the host.

### `web/dist` and the placeholder

`go build` needs at least one file under `web/dist` (`//go:embed all:dist`), so
`web/dist/placeholder.txt` is tracked and everything else in `dist` is ignored
by `.gitignore`. Vite is configured with `emptyOutDir: false` and
`scripts/clean-dist.mjs` empties `dist` before each build except for the
placeholder. A build therefore never changes `git status`. Do not delete the
placeholder and do not commit anything else under `web/dist`.

### The end-to-end smoke test

`npm run test:e2e` (`web/e2e/run.mjs`) builds the frontend, builds the Go binary
so it embeds the real `web/dist`, starts it with a fresh temporary
`HPM_DATA_DIR` on a free port (no Traefik controller), runs
the specs in `e2e/` and removes the temporary directory. The specs share one
database and run in name order: `01-smoke.spec.ts` covers first-run setup, the
shell, a deep-link reload through the SPA fallback, sign out, sign in and the
theme override; `02-hosts.spec.ts` signs in and creates, edits, searches and
deletes Proxy Hosts, including a `409` for a taken domain and the discard
confirmation. Add a spec with the next number prefix.

- Browser: `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` selects an installed Chromium
  (default `/opt/pw-browsers/chromium` when that exists). Otherwise install one
  with `npx playwright install --with-deps chromium`, as CI does.
- Chromium's socket path must be short: if it aborts with "Socket path too
  long", point `TMPDIR` at a short directory.

## Integration tests against a real Traefik

`internal/traefiktest` runs a pinned Traefik (version and tarball hashes in
`internal/traefiktest/pin.go`) against a directory of config, and tests wait for
its API to list the routers they expect. Its package comment records what Traefik
does with invalid files, the API with the dashboard off, and the `@file` names.

- The first run downloads the release tarball, checks it against the committed
  sha256 and caches the binary under `os.UserCacheDir()` (override with
  `HPM_TRAEFIK_CACHE`). Linux only.
- If the binary cannot be obtained, tests that need it skip with the reason. CI
  sets `HPM_REQUIRE_TRAEFIK=1` so they fail instead.
- To bump Traefik, change `Version` and both hashes in `pin.go` (take them from
  the release's `traefik_v<version>_checksums.txt`, and compare with a second
  download) in one pull request, together with the Docker image tag.

## Traefik configuration and the apply pipeline

Every change to hosts goes through `internal/apply`, which renders
`<data>/traefik.yml` and `<data>/dynamic/panel.yml` (see `docs/architecture.md`
section 7) with the pure functions in `internal/render`.

- `HPM_DATA_DIR` (default `./data`) holds `panel.db` and the generated files.
- `HPM_TRAEFIK_CONTROLLER` selects how Traefik is restarted when `traefik.yml`
  changes. Only `none` exists so far (the default); the Panel runs without Traefik
  installed.
- To try the output with a real Traefik, run the Panel and point Traefik at the
  file: `traefik --configFile=data/traefik.yml`.
- Renderer golden files live in `internal/render/testdata`. After an intended
  output change run `go test ./internal/render -update` and review the diff.
- `internal/api/traefik_test.go` creates hosts through the API and sends requests
  through a real Traefik (skipped without the binary, failing under
  `HPM_REQUIRE_TRAEFIK=1`).
