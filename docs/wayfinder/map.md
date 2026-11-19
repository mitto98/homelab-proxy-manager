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

## Not yet specified

- Data model, remaining: sessions storage, instance settings, Access List item shape, and how each host links to its Access List.
- Frontend framework and build tooling.
- Access List semantics (basic auth versus IP rules) and how they map to Traefik middlewares.
- Backup and restore. A full backup must include the SQLite file and the `acme.json` volume, otherwise Traefik reissues certificates and may hit Let's Encrypt rate limits.
- Package build pipeline (nfpm or similar) and release process.
- Debian, Fedora and EPEL package availability for Traefik, which was not verifiable from the sandbox.

## Out of scope

- Writing the implementation.
- Streams (TCP/UDP), 404 hosts, audit log in v1.
