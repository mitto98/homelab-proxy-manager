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

## Not yet specified

- Data model: tables, relations, migrations.
- Frontend framework and build tooling.
- Access List semantics (basic auth versus IP rules) and how they map to Traefik middlewares.
- Backup and restore of the SQLite database.
- Package build pipeline (nfpm or similar) and release process.
- Debian, Fedora and EPEL package availability for Traefik, which was not verifiable from the sandbox.

## Out of scope

- Writing the implementation.
- Streams (TCP/UDP), 404 hosts, audit log in v1.
