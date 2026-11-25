# Map: Traefik-backed proxy manager

## Destination

A locked tech stack and an architecture spec, ready to hand off for building. No code is written in this effort.

**Reached:** see `docs/architecture.md`.

## Notes

- Product: an Nginx Proxy Manager equivalent whose engine is Traefik, storage is SQLite.
- Distribution: Docker image, plus apt (.deb) and rpm packages.
- Vocabulary lives in `GLOSSARY.md`.

## Decisions so far

- Language: **Go**. Single static binary, UI embedded, linux amd64 and arm64.
- v1 scope: Proxy Hosts, Redirection Hosts, Certificates, Access Lists, Users. Streams, 404 hosts and the audit log are deferred.
- Config mechanism: the Panel writes YAML into Traefik's file provider directory. Routing survives a Panel crash.
- Certificates: Traefik's built-in ACME issues them. The Panel stores only uploaded custom certificates.
- Process model: a real supervisor runs both processes. s6-overlay in Docker, systemd units for apt/rpm. The Panel restarts Traefik through a `TraefikController` interface (`s6-svc` in Docker, a systemd `.path` unit on packages). Static-config changes need a Traefik restart.
- Traefik is an external dependency, never bundled in the Panel binary.
  - Docker: multi-stage build. Copy `/usr/local/bin/traefik` from a pinned official `traefik` image into a base we choose (Alpine plus s6-overlay).
  - apt/rpm: the package requires a `traefik` binary on the PATH and fails with clear install instructions if missing. No distro or official Traefik package exists. See `docs/research/traefik-distribution.md`.
- Version compatibility: the Panel declares a supported Traefik v3.x range and warns at startup when outside it.
- Authentication: local users, password login, cookie sessions, first-run admin setup. OIDC and API tokens deferred.
- API and UI: JSON REST API under `/api`, embedded SPA via `embed.FS`.
- Data model (round 1):
  - One `hosts` table with a `kind` column (`proxy` or `redirect`); domains in a child `host_domains` table with a unique constraint across all hosts.
  - One upstream per Proxy Host, stored as columns. Multiple upstreams deferred.
  - Host options limited to what Traefik supports natively: force HTTPS redirect, HSTS with include-subdomains, upstream `http`/`https` with skip-TLS-verify, enabled switch. No free-form advanced config in v1.
  - Certificates split in two: `acme_resolvers` (name, email, challenge type, staging flag; DNS credentials live in `settings`, see ACME and DNS) and `custom_certificates` (PEM chain, encrypted key, domains, expiry). A host references at most one. Issued ACME certificates stay in Traefik's `acme.json` on a persistent volume.
  - Private keys and DNS credentials are encrypted at rest with AES-GCM, using a key generated at first run into a 0600 file in the data directory.
  - Access Lists combine basic-auth credentials and IP allow/deny rules, with AND semantics only in v1 (Traefik middlewares chain; NPM's "satisfy any" is not supported).
  - Two roles: `admin` and `viewer` (read-only), as a `role` column on `users`.
  - The database is the only source of truth. Generated Traefik YAML is never stored, is rebuilt after every change, written atomically, and regenerated in full at startup.
  - Integer auto-increment keys, `created_at`/`updated_at`, hard deletes. Embedded `.sql` migrations. Pure-Go SQLite driver `modernc.org/sqlite`, so no CGO.
- Data model (round 2):
  - Sessions are rows in a `sessions` table (hashed random token, user id, expiry); the cookie holds only the token, so sessions can be revoked.
  - Access Lists have child tables `access_list_users` (username, password hash) and `access_list_rules` (CIDR, allow-only; see config generation below). Hosts link through a nullable `access_list_id`; a list in use cannot be deleted.
  - A single-row typed `settings` table holds instance options: default ACME email, default resolver, Traefik version warning, Panel listen port, and Traefik entrypoint ports.
- Data model is resolved.
- Config generation (verified against Traefik v3.7.19 docs; Traefik has no deny-list middleware, so IP rules are allow-only and map to `ipAllowList`):
  - All dynamic config goes into a single file, written atomically (temp file then rename) into Traefik's watched directory.
  - The Panel also generates the static `traefik.yml` from settings and ACME resolvers. It restarts Traefik through the `TraefikController` only when the static file's content hash changes.
  - "Force HTTPS" is per host: an HTTPS-entrypoint router plus an HTTP-entrypoint router that redirects (`redirectScheme`) or forwards, per the host's toggle.
  - HSTS uses the `headers` middleware; skip-TLS-verify uses a `serversTransport`; basic auth uses bcrypt hashes.
  - Traefik's API is enabled on a loopback-only port so the Panel can show per-host active or failed status and the error.
  - The Panel listens on its own port (default 81), not routed through Traefik by default, so it stays reachable if Traefik fails.
- Packaging:
  - Everything is named `homelab-proxy-manager`: binary, packages, system user, `/etc/homelab-proxy-manager/`, `/var/lib/homelab-proxy-manager/`.
  - GoReleaser on GitHub Actions for each `vX.Y.Z` tag builds the linux amd64 and arm64 binaries, nfpm `.deb` and `.rpm` packages, and the multi-arch Docker image. The version comes from the tag and is embedded in the binary.
  - v1 distribution: packages as GitHub Release assets with a signed checksums file, Docker image on GitHub Container Registry. Hosted apt/yum repositories are deferred.
  - Package services: a dedicated system user; `homelab-proxy-manager.service`, `homelab-proxy-manager-traefik.service` and a path unit that restarts Traefik when the generated static config changes. The Traefik unit has `CAP_NET_BIND_SERVICE`. The Traefik path defaults to `/usr/local/bin/traefik`, configurable in an environment file. No `Depends:` on a Traefik package; the unit fails with a clear message if the binary is missing and the post-install script prints install instructions.
  - Supported targets: Debian 12+, Ubuntu 22.04+, Fedora (current), RHEL-family 9+, all with systemd, on amd64 and arm64.
  - Upgrades: config files keep local edits, migrations run at Panel start (never in package scripts), removal keeps `/var/lib/homelab-proxy-manager` including the database, secret key and `acme.json`.
  - Docker image: one `/data` volume holds the database, secret key, generated config and `acme.json`; ports 80, 443, 81; tags `latest`, `X.Y.Z`, `X.Y`; ships a compose example.
- Frontend (round 1):
  - Vue 3 with TypeScript and Vite, using shadcn-vue (the Vue port of shadcn/ui, on Tailwind) for components.
  - The SPA lives in `web/` in the same repo. In development the Vite dev server proxies `/api` to the Go backend; the production build goes to `web/dist` and is embedded with `embed.FS`.
  - `web/dist` is not committed. CI builds it in the GoReleaser pre-build step, and a committed placeholder file keeps `go build` working from a fresh clone.
  - The API is described by an OpenAPI 3 spec in the repo. Go server types are generated with `oapi-codegen` and TypeScript client types with `openapi-typescript`.
  - English only in v1, with no i18n library.
- Frontend (round 2):
  - Vue Router for pages; Pinia only for the signed-in user and UI preferences, never for server data.
  - Server data through TanStack Query for Vue (`@tanstack/vue-query`) over the typed client; per-host Traefik status is polled on an interval.
  - Tables use TanStack Table for Vue (`@tanstack/vue-table`), through shadcn-vue's Data Table component, which is built on it.
  - Forms use vee-validate with Zod schemas, the integration shadcn-vue ships. The server remains the final authority on validation.
  - Light and dark themes, following the system setting by default with a manual toggle.
  - Vitest with Vue Test Utils for unit and component tests, a few Playwright end-to-end tests against the built binary, ESLint with Prettier. CI blocks merging on type-check, lint and tests.
- Frontend is resolved.
- ACME and DNS (verified against Traefik v3.7.19 docs):
  - Traefik supports one DNS challenge provider per instance, so the provider and its encrypted credentials live in the single-row `settings` table and resolvers only choose the DNS challenge. Domains at other providers use CNAME delegation, documented for users. This amends the data-model decision.
  - Credentials reach Traefik as environment variables through a 0600 env file in the data directory, derived from the encrypted database copy. The Docker run script and the systemd `EnvironmentFile=` read it, and the path unit restarts Traefik when it changes.
  - A host domain starting with `*.` generates `Host(`*.example.com`)` plus an explicit `tls.domains` entry. It is accepted only when the host's resolver uses the DNS challenge, single level only, and the apex domain is added separately. The exact `tls.domains` shape is to be confirmed against a live Traefik during the build.
  - The DNS provider is a name plus key/value environment pairs, validated against lego's provider list, with a few UI presets. Variable names must match `[A-Z0-9_]+` and cannot be `PATH`, `LD_*` or `TRAEFIK_*`.
  - Each resolver has a Let's Encrypt staging toggle.
  - The Panel reads `acme.json` read-only, best effort, to show certificate domains and expiry, and shows "unknown" if parsing fails.
- Distro Traefik packages: none exist in Debian, Ubuntu, Fedora or EPEL (8, 9, 10), so the external-binary decision stands, with no `Depends:`. The post-install message must not suggest `apt install traefik` or `dnf install traefik`. Debian has an open ITP (#983289), unresolved. Caveats in `docs/research/traefik-distro-packages.md`.
- Backup and signing:
  - A backup is exactly `panel.db`, `secret.key` and `acme.json`; everything else is regenerated.
  - The binary has `backup <file>` and `restore <file>` subcommands producing a single `.tar.gz`. SQLite is copied with its online backup mechanism, so backup works while the Panel runs. Restore refuses to run while the Panel is running. The same commands work in Docker through `docker exec`.
  - Backup is CLI only in v1, not in the web UI or API, so the secret key is never behind a web endpoint.
  - Release checksums are signed with cosign keyless, using GitHub's OIDC identity from the Actions run. The verification command goes in the README. GPG is deferred until a hosted repository exists.

## Not yet specified

_Nothing left. The map is complete._

## Out of scope

- Writing the implementation.
- Streams (TCP/UDP), 404 hosts, audit log in v1.
