# Architecture: homelab-proxy-manager

A web panel in the spirit of Nginx Proxy Manager whose proxy engine is Traefik and whose storage is SQLite. It ships as a Docker image and as `.deb` and `.rpm` packages.

This document is the destination of the wayfinder map in `docs/wayfinder/map.md`. Every decision here was made there, in order, with the reasoning kept in the conversation history that produced it. Terms are defined in `GLOSSARY.md`. Facts about Traefik were checked against the Traefik v3.7.19 documentation (see `docs/research/traefik-distribution.md`).

## 1. Goals and non-goals

**v1 goals**
- Manage Proxy Hosts, Redirection Hosts, TLS certificates (Let's Encrypt and uploaded), Access Lists and Users from a web UI.
- Keep routing alive when the Panel is down.
- One small artifact per channel: a Docker image, a `.deb`, an `.rpm`.

**Non-goals for v1**
- TCP/UDP streams, 404 hosts, an audit log.
- Multiple upstreams or load balancing per host.
- Free-form "advanced config" per host.
- Access List "satisfy any" (login or IP) semantics. Traefik chains middlewares, so both checks apply together.
- OIDC login, API tokens, per-object permissions, translations.
- Hosted apt/yum repositories.

## 2. System overview

```
                 browser
                    |
             :81    v
        +------------------------+        writes          +-----------------------+
        |   homelab-proxy-manager| ---------------------> |  /data (or /var/lib)  |
        |   (Go, single binary)  |   dynamic.yml           |  panel.db  (SQLite)   |
        |  REST API + embedded   |   traefik.yml           |  secret.key           |
        |  Vue SPA               |   traefik.env (0600)    |  dynamic/panel.yml    |
        +-----------+------------+   certs/                |  traefik.yml          |
                    |  reads                               |  traefik.env          |
                    |  Traefik API (loopback)              |  acme.json            |
                    v                                      +-----------+-----------+
        +------------------------+   file provider watch               |
        |        Traefik         | <-----------------------------------+
        |  :80  :443             |   env file at process start
        |  API on 127.0.0.1 only |
        +------------------------+
```

- The Panel and Traefik are **separate processes under a supervisor**: s6-overlay in the Docker image, systemd on packages. Neither is PID 1 for the other, and a Panel crash does not stop routing.
- **SQLite is the only source of truth.** All Traefik files are derived, disposable, and regenerated in full at Panel startup.
- Traefik is an **external dependency** and is never bundled in the Panel binary.

## 3. Technology

| Area | Choice |
|---|---|
| Backend language | Go 1.26 (required by the current `modernc.org/sqlite`) |
| SQLite driver | `modernc.org/sqlite` (pure Go, no CGO, cross-compiles to arm64) |
| Migrations | Embedded `.sql` files, applied in order at Panel start |
| API | JSON REST under `/api`, described by an OpenAPI 3 spec in the repo |
| Go types from spec | `oapi-codegen` |
| Frontend | Vue 3, TypeScript, Vite |
| UI components | shadcn-vue on Tailwind, light and dark themes |
| Routing and state | Vue Router; Pinia only for the signed-in user and UI preferences |
| Server data | TanStack Query for Vue over a client typed by `openapi-typescript` |
| Tables | TanStack Table for Vue via shadcn-vue's Data Table |
| Forms | vee-validate with Zod; the server is the final validator |
| Frontend tests | Vitest and Vue Test Utils; a few Playwright end-to-end tests on the built binary |
| Lint and format | ESLint and Prettier; CI blocks on type-check, lint and tests |
| Language in UI | English only in v1 |
| Build and release | GoReleaser on GitHub Actions per `vX.Y.Z` tag, with nfpm for packages |
| Supported Traefik | v3.x; the Panel warns at startup when the installed version is outside the declared range |

## 4. Repository layout

```
/
├── cmd/homelab-proxy-manager/    main package
├── internal/                     Go packages (see section 6)
├── api/openapi.yaml              API contract, source of generated code
├── migrations/                   embedded .sql migrations
├── web/                          Vue SPA; web/dist is built in CI, never committed
├── packaging/                    nfpm config, systemd units, s6 service scripts, Dockerfile
├── docs/                         this file, research notes, wayfinder map
├── GLOSSARY.md
└── .goreleaser.yaml
```

A placeholder file is committed in `web/dist` so `go build` works from a fresh clone. In development the Vite dev server proxies `/api` to the Go backend.

## 5. Data model

Integer auto-increment keys, `created_at` and `updated_at` on every table, hard deletes, foreign keys enforced. Column lists are the intended shape; exact types and indexes are an implementation detail.

**users**: `id`, `username`, `password_hash`, `role` (`admin` or `viewer`).
The first user is created by the first-run setup flow and is an admin. (Whether the login name is a username or an email address is left to implementation.)

**sessions**: `id`, `token_hash`, `user_id`, `expires_at`. The cookie holds only the random token; the database holds its hash, so sessions can be revoked on logout, password change or user deletion.

**hosts**
- `id`, `kind` (`proxy` or `redirect`), `enabled`
- Proxy fields: `upstream_scheme` (`http` or `https`), `upstream_host`, `upstream_port`, `upstream_skip_tls_verify`
- Redirect fields: `redirect_target`, `redirect_status_code`, `redirect_preserve_path`
- Options: `force_https`, `hsts_enabled`, `hsts_include_subdomains`
- The upstream columns are nullable so Redirection Hosts can share the table; a `CHECK` makes `upstream_scheme`, `upstream_host` and `upstream_port` mandatory when `kind` is `proxy`. Domains keep the order the user gave them (insertion order of `host_domains`).
- References, all nullable: `acme_resolver_id`, `custom_certificate_id` (a check constraint allows at most one of them), `access_list_id`

**host_domains**: `id`, `host_id`, `domain` with a unique constraint across all hosts, so a domain can never be both proxied and redirected or belong to two hosts. A domain may start with `*.` (see section 7.5).

**acme_resolvers**: `id`, `name`, `email`, `challenge` (`http`, `tls` or `dns`), `use_staging`.

**custom_certificates**: `id`, `name`, `domains`, `certificate_pem`, `encrypted_private_key`, `not_after`.

**access_lists**: `id`, `name`.
**access_list_users**: `id`, `access_list_id`, `username`, `password_hash` (bcrypt).
**access_list_rules**: `id`, `access_list_id`, `cidr`. Allow-only, because Traefik has no deny-list middleware.
An Access List that any host uses cannot be deleted.

**settings**: one row (`id` is always 1, enforced by a `CHECK`), with typed columns, each stage adding its own by migration. Today: `http_port` (default 80), `https_port` (default 443) and `api_port` (default 8899), the loopback port of Traefik's API, all checked to 1..65535. 8899 and not Traefik's own 8080 because upstreams in a homelab often use 8080. Planned: default ACME email, default resolver, Panel listen port, DNS provider name and encrypted DNS credentials (see 7.6).

**Secrets at rest.** Custom certificate keys and DNS credentials are encrypted with AES-GCM. The key is generated at first run into `secret.key` (mode 0600) in the data directory. Losing that file makes those secrets unrecoverable, so backups must include it.

## 6. Backend components

```
internal/
├── api/            HTTP handlers generated against the OpenAPI spec; auth middleware; embedded SPA
├── auth/           password hashing, sessions, first-run setup
├── store/          SQLite access, migrations, transactions
├── secrets/        AES-GCM encryption using secret.key
├── model/          domain types and validation rules
├── render/         turns the stored model into Traefik files (pure functions)
├── apply/          the pipeline in 7.1; owns the single writer lock
├── traefik/        TraefikController interface, Traefik API client, acme.json reader
└── version/        embedded version and supported Traefik range
```

`render` is a pure function from model to bytes, so it can be tested with golden files and no Traefik running. `apply` is the only code that touches the generated files.

### TraefikController

The implementation is chosen at startup by the environment variable `HPM_TRAEFIK_CONTROLLER`. The default and only value today is `none`; an unknown value stops the Panel at startup with a clear message, so a typo cannot silently turn restarts off. Slice 2.8 adds `s6`.

```go
type TraefikController interface {
    // Called when the static config or env file content changed.
    Restart(ctx context.Context) error
}
```

- Docker implementation: calls `s6-svc -r` on the Traefik service.
- `none` (the default; also used for development and tests) and systemd: do nothing, because a systemd `.path` unit watches `traefik.yml` and `traefik.env` and restarts the Traefik unit itself. The Panel needs no privileges.

## 7. Traefik integration

### 7.1 Apply pipeline

Every change to hosts, certificates, resolvers, Access Lists or settings goes through one pipeline, serialized by a mutex:

1. Open a database transaction and apply the change.
2. Render dynamic config, static config and env file from the transaction's view of the data.
3. Validate the rendered model (unique routers, every reference resolves, wildcard rules in 7.5). On failure, roll back and return the error to the caller.
4. Commit.
5. Write each file to a temporary name, then rename it into place.
6. If the static config or env file content hash changed, call `TraefikController.Restart`.

At Panel startup (`Applier.Start`) the pipeline renders everything once from the database, so a restored backup or hand-deleted or corrupted file is repaired automatically. A failure there is fatal: the Panel does not serve a state it cannot publish. Start restarts Traefik too if the static config on disk differs.

**Failure semantics.** Until commit everything is atomic: a failing change, rendering or validation rolls back, stores nothing and touches no file (validation matters because one invalid file in `dynamic/` freezes all Traefik reloads). After commit the database is the truth and files are derived. A failure to write them is returned as an error although the change is stored, and the next apply or the next Panel start rewrites every file whose content differs. A failed `Restart` does not fail the request: it is logged and reported in the apply result, and the new static config is already on disk. Files are only rewritten when their content differs, so a change that does not alter a file does not wake Traefik's watcher.

The `traefik.yml` content comparison is against the file currently on disk; a missing file counts as changed.

### 7.2 Files

| File | Purpose | Reload |
|---|---|---|
| `dynamic/panel.yml` | Routers, services, middlewares, serversTransport, custom-certificate TLS entries. A single file. | Hot, by Traefik's file provider watch; no restart |
| `traefik.yml` | Static config: entrypoints, file provider, certificate resolvers, API | Restart on content change |
| `traefik.env` | DNS provider credentials, mode 0600 | Restart on content change |
| `acme.json` | Written by Traefik, holds issued certificates | Panel reads it, never writes |

One dynamic file means every reload sees a consistent snapshot; writing one file per host would expose half-regenerated states.

**Data directory layout** (`HPM_DATA_DIR`): `traefik.yml` at its root and `dynamic/panel.yml`. Directories are mode 0750 and generated files 0640. Only `panel.yml` is owned by the Panel; other files in `dynamic/` are never touched or deleted. Files are written to a temporary name in the same directory that ends in `.tmp` (never `.yml`, `.yaml` or `.toml`, which Traefik would load half-written) and renamed into place.

### 7.3 Dynamic config shape

Names are derived from IDs, such as `host-12`. A Proxy Host with HTTPS, forced redirect, HSTS and an Access List renders roughly as:

```yaml
http:
  routers:
    host-12-https:
      rule: Host(`app.example.com`) || Host(`www.example.com`)
      entryPoints: [websecure]
      service: host-12
      middlewares: [access-list-3-ip, access-list-3-auth, host-12-hsts]
      tls:
        certResolver: resolver-1
    host-12-http:
      rule: Host(`app.example.com`) || Host(`www.example.com`)
      entryPoints: [web]
      service: host-12
      middlewares: [host-12-redirect-https]   # omitted, with access middlewares added, when force_https is off
  middlewares:
    host-12-redirect-https:
      redirectScheme: { scheme: https, permanent: true }
    host-12-hsts:
      headers: { stsSeconds: 31536000, stsIncludeSubdomains: false, forceSTSHeader: true }
    access-list-3-ip:
      ipAllowList: { sourceRange: ["192.168.0.0/16"] }
    access-list-3-auth:
      basicAuth: { users: ["alice:$2y$..."] }
  services:
    host-12:
      loadBalancer:
        servers: [{ url: "https://10.0.0.5:8443" }]
        serversTransport: skip-tls-verify   # only when upstream_skip_tls_verify
  serversTransports:
    skip-tls-verify: { insecureSkipVerify: true }
```

Rules:
- With no enabled host the file is `{}`. Checked against Traefik 3.7.19: `http: {}` is rejected ("http cannot be a standalone element") and would freeze all reloads.
- Disabled hosts are left out of the file entirely.
- A host without TLS gets only a `web` router.
- An Access List renders an `ipAllowList` (only if it has rules) and a `basicAuth` middleware (only if it has users). Both must pass.
- Redirection Hosts use a redirect middleware and Traefik's internal no-op service; the exact middleware (`redirectRegex` to preserve the path) is chosen at build time.
- Custom certificates are referenced through the dynamic file's TLS section. Whether the PEM can be inline or must be written to `certs/<id>.pem` (mode 0600) is confirmed at build time; writing files is the safe default.

### 7.4 Static config

Generated from `settings` and `acme_resolvers`:
- Entrypoints `web` and `websecure` on the configured ports, plus the `traefik` entrypoint for the API, declared as `127.0.0.1:<api_port>`. Without that explicit address Traefik would serve its unauthenticated API on all interfaces. `api.insecure` is true and `api.dashboard` false.
- `providers.file.directory` pointing at `dynamic/` with `watch: true`.
- One `certificatesResolvers` entry per `acme_resolvers` row, named `resolver-<id>`, with `storage` set to `acme.json`, the challenge type, the email, and the staging CA when `use_staging` is set.

### 7.5 Wildcard domains

A host domain such as `*.example.com` renders as `Host(`*.example.com`)` plus an explicit `tls.domains` entry. The Panel validates that:
- the host's resolver uses the DNS challenge,
- the wildcard is a single leading label (`*.*.example.com` is rejected),
- the apex `example.com` is added as a separate domain if wanted, because the wildcard does not match it.

The exact `tls.domains` shape is confirmed against a live Traefik during the build.

### 7.6 DNS challenge credentials

Traefik supports one DNS challenge provider per instance, so the provider and its credentials are instance settings, not per-resolver. Users with domains at another provider use CNAME delegation, which the user docs explain.

- The provider is a name plus key/value environment pairs, validated against lego's provider list. The UI offers a few presets such as Cloudflare.
- Variable names must match `[A-Z0-9_]+` and may not be `PATH`, `LD_*` or `TRAEFIK_*`.
- The Panel writes `traefik.env` from the encrypted database copy. The Docker run script and the systemd `EnvironmentFile=` read it, and a change restarts Traefik.

### 7.7 Status and certificates in the UI

- Traefik's API listens on loopback only. The Panel polls it to show each host as active, or failed with Traefik's error.
- The Panel reads `acme.json` read-only, best effort, to show each issued certificate's domains and expiry. If parsing fails it shows "unknown". The file format is not a public contract, so this feature must never block anything else.
- At startup the Panel checks the Traefik version and logs a warning when it is outside the supported range.

## 8. HTTP API and authentication

All routes are under `/api` and defined in `api/openapi.yaml`.

| Area | Routes |
|---|---|
| Setup and auth | `POST /setup` (only while no user exists), `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` |
| Hosts | `GET/POST /hosts`, `GET/PUT/DELETE /hosts/{id}`, `GET /hosts/{id}/status` |
| TLS | `/acme-resolvers`, `/custom-certificates` (CRUD), `GET /certificates/issued` (from `acme.json`) |
| Access Lists | `/access-lists` (CRUD, with their users and rules) |
| Users | `/users` (CRUD, admin only) |
| Settings | `GET/PUT /settings` |
| System | `GET /status` (Traefik reachable, version, warnings) |

Errors share one JSON shape, `{"error": "message"}`, with an optional `field` naming the request field a validation error is about (for example `domains`). A domain already used by another host is a `409` that names the domain.

Authentication is local users with passwords, cookie sessions and a first-run admin setup. Roles are `admin` (everything) and `viewer` (read-only). Authorization is enforced in one middleware, not per handler.

The Panel listens on its own port, default 81, and is not routed through Traefik by default. A user who wants a domain and TLS for the Panel creates an ordinary Proxy Host pointing at it. If Traefik fails, the Panel is still reachable.

## 9. Frontend

The Vue SPA lives in `web/`, is served by the Go binary from `embed.FS`, and talks to `/api` only. Server data lives in TanStack Query's cache. Pinia holds only the signed-in user and UI preferences. The theme follows the system setting with a manual toggle. The typed client is generated from the OpenAPI spec, so a mismatch between backend and frontend is a compile error. The sidebar shell (variant A of the prototype) lists only pages that exist, from one data array. The router guard sends users to `/setup` while no user exists and to `/login` when signed out. Tailwind is v4. See `docs/development.md` for the scripts, the dev server and the end-to-end test.

## 10. Packaging and release

**Naming.** Binary, packages, system user, `/etc/homelab-proxy-manager/` and `/var/lib/homelab-proxy-manager/` all use `homelab-proxy-manager`.

**Pipeline.** GoReleaser on GitHub Actions for each `vX.Y.Z` tag builds linux amd64 and arm64 binaries, nfpm `.deb` and `.rpm` packages and the multi-arch Docker image. The version is embedded in the binary from the tag.

**Distribution in v1.** Packages are GitHub Release assets with a signed checksums file. The Docker image goes to GitHub Container Registry with tags `latest`, `X.Y.Z` and `X.Y`. Hosted package repositories are deferred.

**Targets.** Debian 12+, Ubuntu 22.04+, Fedora (current) and RHEL-family 9+, all with systemd, on amd64 and arm64.

**Docker image**
- Multi-stage build: build the Panel, copy `/usr/local/bin/traefik` from a pinned official `traefik` image, and assemble a final Alpine image with s6-overlay. The Traefik version is one pinned line in the Dockerfile.
- One `/data` volume holds the database, `secret.key`, generated config and `acme.json`, so a copy of that volume is a complete backup.
- Ports 80, 443 and 81. A compose example ships with the project.

**Packages**
- A dedicated system user; units `homelab-proxy-manager.service`, `homelab-proxy-manager-traefik.service` and a path unit that restarts Traefik when `traefik.yml` or `traefik.env` change.
- The Traefik unit has `CAP_NET_BIND_SERVICE` so it can bind 80 and 443 without root.
- No distro or official Traefik package exists, so the package declares no `Depends:` on one. The Traefik binary path defaults to `/usr/local/bin/traefik` and is set in an environment file. The Traefik unit fails with a clear message if the binary is missing, and the post-install script prints install instructions.
- Config files keep local edits on upgrade. Migrations run when the Panel starts, never in package scripts. Removing the package keeps `/var/lib/homelab-proxy-manager`.

### Backup and signing

- A backup is exactly `panel.db`, `secret.key` and `acme.json`; everything else is regenerated. The binary provides `backup <file>` and `restore <file>` subcommands that produce and consume one `.tar.gz`. SQLite is copied with its online backup mechanism so backup works while the Panel runs, and restore refuses to run while it is running. In Docker the same commands run through `docker exec`.
- Backup is CLI only in v1. It is not in the web UI or API, so the secret key is never behind a web endpoint.
- Release checksums are signed with cosign in keyless mode using GitHub's OIDC identity from the Actions run, and the README documents the verification command. GPG signing is deferred until a hosted package repository exists.

## 11. Testing strategy

- **Renderer**: golden-file tests over a table of host configurations. No Traefik needed.
- **Store and apply**: tests on a temporary SQLite database, including rollback on validation failure.
- **Integration**: in CI, start a pinned Traefik with generated config, then assert the routes load through its API and requests are routed. This is where the open points flagged "confirm at build time" get settled.
- **API**: handler tests against the OpenAPI spec.
- **Frontend**: Vitest for components; Playwright end-to-end tests against the built binary.

## 12. Suggested build order

1. Go skeleton, SQLite store, migrations, OpenAPI spec, first-run setup, login and sessions.
2. Proxy Hosts with HTTP routing, the renderer, the apply pipeline and Traefik supervision in Docker.
3. HTTPS: ACME resolvers with HTTP/TLS challenges, force HTTPS, HSTS, certificate expiry display.
4. Custom certificates, secrets encryption, and wildcard hosts with the DNS challenge.
5. Access Lists, Redirection Hosts and users with roles.
6. Status polling and the full UI polish.
7. `.deb` and `.rpm` packages, systemd units, release pipeline and signing.

Stages 1 and 2 give a working end-to-end slice early. The Docker path comes first because it is the only environment that exercises the supervisor design.

## 13. Open points

These were deliberately left for later and none blocks starting the build:


- **Traefik in Debian, Fedora and EPEL repositories**: settled. None ship a Traefik package, so the external-binary decision stands with no `Depends:` (see `docs/research/traefik-distro-packages.md`).
- **Confirm at build time**: the `tls.domains` shape for wildcard hosts, whether custom-certificate PEMs can be inline, and the redirect middleware for Redirection Hosts.
