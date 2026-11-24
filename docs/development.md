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
