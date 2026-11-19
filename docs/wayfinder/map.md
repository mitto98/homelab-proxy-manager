# Map: Traefik-backed proxy manager

## Destination

A locked tech stack and an architecture spec, ready to hand off for building. No code is written in this effort.

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
  - Certificates split in two: `acme_resolvers` (name, email, challenge type, encrypted DNS credentials) and `custom_certificates` (PEM chain, encrypted key, domains, expiry). A host references at most one. Issued ACME certificates stay in Traefik's `acme.json` on a persistent volume.
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
  - Forms use TanStack Form for Vue (`@tanstack/vue-form`) with Zod schemas, not vee-validate. shadcn-vue still supplies the visual components. The server remains the final authority on validation.
  - Light and dark themes, following the system setting by default with a manual toggle.
  - Vitest with Vue Test Utils for unit and component tests, a few Playwright end-to-end tests against the built binary, ESLint with Prettier. CI blocks merging on type-check, lint and tests.
- Frontend is resolved.

## Not yet specified

- Wildcard domains: how a host with `*.example.com` becomes a router rule and `tls.domains` (DNS challenge only).
- How DNS-provider credentials reach Traefik's environment, and what that means for supervisor restarts.
- Backup and restore. In Docker one `/data` volume is a complete backup. On packages a backup needs both the SQLite file and `acme.json`, otherwise Traefik reissues certificates and may hit Let's Encrypt rate limits.
- Release signing: which key or keyless method signs the checksums file.
- Debian, Fedora and EPEL package availability for Traefik, which was not verifiable from the sandbox.

## Out of scope

- Writing the implementation.
- Streams (TCP/UDP), 404 hosts, audit log in v1.
