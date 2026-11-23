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
