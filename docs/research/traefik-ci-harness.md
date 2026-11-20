# Traefik CI harness and version pin

Researched 2026-11-20. Primary sources only. Items marked UNVERIFIED could not be confirmed. Answers issue #5 ("Pinned Traefik version and CI integration harness"); builds on `docs/research/traefik-distribution.md` and `docs/research/traefik-distro-packages.md`.

## Summary

- **Pin: Traefik v3.7.19** (latest stable v3 on 2026-11-20). One value, kept in two places that must agree: the Docker image tag in `packaging/Dockerfile` and a `TRAEFIK_VERSION` workflow env var.
- **CI obtains the binary from the official release asset** `traefik_v3.7.19_linux_<arch>.tar.gz`, verified against the combined `traefik_v3.7.19_checksums.txt` (sha256). Not a GitHub Actions service container: the binary route is simpler for the loopback and file-watch assumptions in `docs/architecture.md` (sections 7.2 and 7.4).
- **Docker image pin**: `COPY --from=traefik:v3.7.19@sha256:a6c0194d...` so the tag and digest are both fixed.
- **File provider watch and loopback API work as the spec assumes**, with two details to encode: the `traefik` entrypoint must be declared with an explicit `127.0.0.1:` address (the default is `:8080`, all interfaces), and the temp file written before rename must not end in `.yml`, `.yaml` or `.toml` if it sits in the watched directory.
- **No `.github/workflows` exists yet** (VERIFIED, `ls .github` fails in the repo). The architecture mentions CI only as a plan (sections 3, 10, 11).
- The downloaded binary was checksum-verified but **not executed** in this session (see Access notes), so behaviour claims below rest on the docs and Go source at the tag, not on a live run. UNVERIFIED empirically.

## Access notes

- Reachable and used: GitHub release asset downloads (`github.com/traefik/traefik/releases/download/...`), `git ls-remote` and a shallow clone of `traefik/traefik` at tag `v3.7.19` (docs source and Go source read from that clone), Docker Hub API (`hub.docker.com/v2`), `docs.github.com` via `firecrawl_scrape`.
- Not tried or not reachable: `doc.traefik.io` (earlier notes record it as proxy-blocked; I did not retry it, so the Traefik docs below are the repo copies of `docs/content/` at v3.7.19, the source the site is built from, not the rendered site). The GitHub REST API and `github.com/.../releases` HTML pages (403 per earlier notes), so the release publication timestamp stays UNVERIFIED.
- Running the downloaded `traefik` binary was refused by the sandbox's permission check in this session, so no live run was made. Checksum verification (no execution) did succeed.

## 1. The pin

| Claim | Status | Source |
| --- | --- | --- |
| Highest stable v3 tag is `v3.7.19`; the only newer tags are `v3.8.0-rc.1`, `-rc.2`, `-rc.3` | VERIFIED | `git ls-remote --tags https://github.com/traefik/traefik` on 2026-11-20 |
| Docker Hub `library/traefik` tags `v3.7.19` and `latest` share index digest `sha256:a6c0194dda77bcb804824580f36fa7edc56e315b1acfe933366c8b52844a78a4`, last updated 2026-11-19T02:10Z | VERIFIED | https://hub.docker.com/v2/repositories/library/traefik/tags/v3.7.19 and `.../tags/latest` |
| v3.7.19 is what `docs/research/traefik-distribution.md` verified docs against | VERIFIED | that note, section 4 |
| Release timestamp of v3.7.19 on GitHub | UNVERIFIED | releases page and API blocked; tag commit date 2026-11-12 is in the earlier note |

Recommendation:

- Pin **v3.7.19**, an exact patch. Do not track `v3` or `latest` in CI, and do not pin the v3.8 release candidates.
- The Panel's declared supported range (architecture section 3, "Supported Traefik: v3.x") stays wider than the CI pin. CI proves one version; the range is a promise the project does not test. Consider a scheduled (weekly) job that runs the same integration suite against the newest stable v3 and `latest`, informational only, so range drift is noticed. This is a suggestion, not something sourced.
- Bump the pin by hand in one pull request that changes the Dockerfile line and the workflow variable together; a dependency bot is optional.

## 2. Getting the binary in GitHub Actions

### Option A (recommended): official release asset plus checksum

- Asset name: `traefik_v3.7.19_linux_amd64.tar.gz` and `..._linux_arm64.tar.gz`. VERIFIED, both appear in the checksums file. Naming template `traefik_v{{Version}}_{{Os}}_{{Arch}}` in https://github.com/traefik/traefik/blob/v3.7.19/.goreleaser.yml.tmpl (from the earlier note).
- Checksums: one combined file `traefik_v3.7.19_checksums.txt` at https://github.com/traefik/traefik/releases/download/v3.7.19/traefik_v3.7.19_checksums.txt . VERIFIED to be sha256 (64 hex digit lines) and the downloaded `linux_amd64` archive passed `sha256sum -c` against it on 2026-11-20. Expected values:
  - `97534a68335a087137d0f2adecefe37c8216ac10114dc7e5b510f2a8a12f89be  traefik_v3.7.19_linux_amd64.tar.gz`
  - `6d796c41b8d3c8c31bb2faf1bfff1b5fcf603676ad9a773b29ffa7efa678881a  traefik_v3.7.19_linux_arm64.tar.gz`
- Trust caveat, VERIFIED: the checksums file ships beside the archive in the same release, so it detects corruption but not a compromised release. The release workflow has no signing step (grep for `sign`, `cosign`, `attest` in `release.yaml` and `.goreleaser.yml.tmpl` finds nothing; only the checksum steps at `release.yaml` lines 80 and 128). To make that check stronger, commit the expected sha256 values into the workflow (or a `packaging/traefik.sha256` file) at pin time, so a changed upstream asset fails CI. Sources: https://github.com/traefik/traefik/blob/v3.7.19/.github/workflows/release.yaml and https://github.com/traefik/traefik/blob/v3.7.19/.goreleaser.yml.tmpl
- The binary is static (`CGO_ENABLED=0`), per the earlier note, so it runs on any runner image without extra libraries.
- Runner labels (VERIFIED): `ubuntu-latest` (x64, also `ubuntu-24.04`) and `ubuntu-24.04-arm` (arm64), free on public repositories. https://docs.github.com/en/actions/reference/runners/github-hosted-runners . The Linux runners have passwordless `sudo` on the same page, but nothing here needs it: run Traefik as the runner user on high ports.
- Runner network allowlists (VERIFIED, same page) list `github.com` and `release-assets.githubusercontent.com` among needed endpoints, so downloading release assets is an expected operation on hosted runners.

Sketch (not committed; there is no workflow yet):

```yaml
env:
  TRAEFIK_VERSION: v3.7.19
steps:
  - run: |
      curl -fsSL -o traefik.tgz "https://github.com/traefik/traefik/releases/download/${TRAEFIK_VERSION}/traefik_${TRAEFIK_VERSION}_linux_amd64.tar.gz"
      echo "97534a68335a087137d0f2adecefe37c8216ac10114dc7e5b510f2a8a12f89be  traefik.tgz" | sha256sum -c -
      tar -xzf traefik.tgz traefik
```

Cache the tarball with `actions/cache` keyed on the version if download time matters (UNVERIFIED need).

### Option B: official Docker image

- `docker.io/library/traefik:v3.7.19` is built from the same release tarball, with the binary at `/usr/local/bin/traefik`, root user, `ENTRYPOINT ["/entrypoint.sh"]`. VERIFIED in `docs/research/traefik-distribution.md` section 3.
- GitHub Actions service containers can pass `command` and `entrypoint` and map `ports`, but jobs on the runner reach them only through mapped ports. https://docs.github.com/en/actions/tutorials/use-containerized-services/use-docker-service-containers . VERIFIED.
- That page documents no way to mount workspace files into a service container, so config would need a `docker run -v` step instead of a `services:` block. I did not find a documented bind-mount option for `services` on that page (UNVERIFIED that none exists).
- A container also makes "loopback-only API" harder to test: the API would have to listen on `0.0.0.0` inside the container and be published to `127.0.0.1` on the host with `-p 127.0.0.1:8080:8080`. That tests the port mapping, not the Panel's real `127.0.0.1` entrypoint config.
- Use the image only where the image itself is under test (the Dockerfile `COPY --from=` step), which the release job exercises anyway.

Decision: Option A for the integration job, Option B's pinned reference in the Dockerfile. Both use the same version string, so a test that reads `/api/version` can assert it equals `TRAEFIK_VERSION` and catch the two drifting.

## 3. Running Traefik against generated config in CI

Harness shape (all from architecture 7.2 and 7.4, checked against the Traefik source):

1. Test builds a temp data dir, calls `render` (and `apply`) to write `traefik.yml` and `dynamic/panel.yml`.
2. Test starts `traefik --configFile=<tmp>/traefik.yml` as a child process, with entrypoints on free high ports, and the `traefik` entrypoint on `127.0.0.1:<port>`.
3. Test polls the API until routers appear, then asserts and sends requests to the `web` entrypoint with a `Host` header.
4. Test overwrites `dynamic/panel.yml` (temp file plus rename) and polls again to assert hot reload.
5. Teardown kills the process.

Traefik's own integration tests use the same pattern: tests under `integration/` use the file provider and `try.GetRequest` with a retry timeout (for example `integration/file_test.go` requests `http://127.0.0.1:8000/` expecting 404 or 502). VERIFIED by reading https://github.com/traefik/traefik/blob/v3.7.19/integration/file_test.go . So polling with a timeout is the established approach, not sleeping.

### Endpoints to assert against (VERIFIED)

Source: https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/api-dashboard.md

- `GET /api/http/routers` and `/api/http/routers/{name}`, `/api/http/services`, `/api/http/middlewares`, `/api/entrypoints`, `/api/rawdata`, `/api/overview`, `/api/version`. All `GET`.
- Router, service and middleware objects carry `status` and an `error` list. VERIFIED in `pkg/config/runtime/runtime_http.go` (`Status string json:"status"`, `Err []string json:"error"`; RouterInfo also has `using`, the effective entrypoints) at https://github.com/traefik/traefik/blob/v3.7.19/pkg/config/runtime/runtime_http.go . This is what the Panel's `GET /hosts/{id}/status` (architecture 7.7) reads: `enabled` is good, anything else is "failed" with the error text.
- Resource names in the API carry a provider suffix (for example `host-12-https@file`). I did not re-verify the suffix format in this pass; the `provider` field is present in the router JSON (`pkg/api/handler_http.go`). The Panel client should match on the prefix or strip `@file`. UNVERIFIED for the exact string.
- `/ping` is a separate opt-in (`ping: {}`), default entrypoint `traefik`, returns 200 `OK`. VERIFIED: https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/observability/healthcheck.md . Use `/api/version` as the readiness probe instead, so no extra static config is needed.

## 4. Does the loopback-only API work as the spec assumes?

Spec (architecture 7.4): "a loopback-only `traefik` entrypoint for the API". Findings:

- `api.insecure: true` serves the API "directly on the entryPoint named traefik". VERIFIED, api-dashboard.md option table (link above) and `Insecure bool` description "Activate API directly on the entryPoint named traefik" in https://github.com/traefik/traefik/blob/v3.7.19/pkg/config/static/static_config.go
- If no entrypoint named `traefik` is defined, Traefik creates one at **`:8080`**, which is all interfaces. VERIFIED, `static_config.go` lines 308 to 316 (`ep := &EntryPoint{Address: ":8080"}`) and the fallback at lines 321 to 323.
- If the user defines it, the user's definition wins (`if _, ok := c.EntryPoints[DefaultInternalEntryPointName]; !ok`). So the generated static config must declare it, for example:

  ```yaml
  entryPoints:
    traefik:
      address: "127.0.0.1:8080"
  api:
    insecure: true
  ```

  Omitting the entrypoint block silently exposes the API on every interface. This should be a renderer golden-file test and a harness assertion (bind check below).
- The `address` option accepts `[host]:port[/tcp|/udp]`, so a loopback host is valid syntax. VERIFIED: https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/entrypoints.md (`opt-address`).
- Docs warn the API exposes sensitive configuration and should not be publicly reachable. VERIFIED, "Security" section of api-dashboard.md.
- Harness assertion for loopback: query `/api/entrypoints/traefik` and check its `address` is `127.0.0.1:<port>`; additionally, attempt a connection to the runner's non-loopback address on that port and expect refusal. The second check is a suggestion; it is UNVERIFIED that the hosted runner exposes a usable non-loopback address, so make it skip rather than fail if none is found.
- Ping and the dashboard share the same entrypoint when enabled. The dashboard is on by default with `api: {}` (`api.dashboard` default `true`); the Panel does not need it, so set `api.dashboard: false` to cut surface. The option exists in the table above. Whether the API endpoints still work with `dashboard: false` was not tested. UNVERIFIED.

## 5. Does the file provider watch work as the spec assumes?

Spec (architecture 7.2 and 7.4): one `dynamic/panel.yml`, `providers.file.directory` with `watch: true`, files written to a temporary name then renamed.

- `watch` defaults to `true` and works with both `directory` and `filename`. VERIFIED (the earlier note and `p.Watch = true` in `SetDefaults`, https://github.com/traefik/traefik/blob/v3.7.19/pkg/provider/file/file.go line 44).
- In directory mode the provider adds an fsnotify watch **on the directory itself** and, on any event in it, rebuilds the whole configuration (`watcher.Add(item)` for each watch item, then `callback` for every event when `p.Directory != ""`). VERIFIED, `file.go` `addWatcher`, lines 169 to 215. A rename into the directory is a directory event, so the temp-then-rename write in the apply pipeline triggers a reload. Watching a file path directly would be the fragile case with rename; the spec chose the directory, which is right.
- Only `.toml`, `.yaml` and `.yml` files are loaded from the directory. VERIFIED, `isFileSupported` at `file.go` line 516 to 518. So a temp name like `panel.yml.tmp` is ignored by the loader. Rename-into-place events still wake the watcher but a `.tmp` file is harmless. A temp name ending in `.yml` (for example `panel.tmp.yml`) would be parsed mid-write and must be avoided. Alternative: write the temp file outside `dynamic/` on the same filesystem, so the rename is atomic and the watcher sees one event.
- Docs caveat, VERIFIED: fsnotify "can miss changes when mounts or symlinks break the link between file systems"; the docs recommend setting `directory` to the parent directory or bind-mounting it. https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/providers/others/file.md . For the Docker image this means `dynamic/` should live on the `/data` volume that both processes see directly. The harness cannot reproduce Docker bind-mount edge cases on a plain runner; a separate smoke test inside the built image would. UNVERIFIED either way.
- `providersThrottleDuration` default 2s: after a reload, the next provider event waits and only the latest event in the window is applied. VERIFIED, file.md and `ProvidersThrottleDuration` in `static_config.go`. Consequence for tests: a reload assertion needs a polling timeout of several seconds, not a fixed sleep. Setting `providersThrottleDuration` shorter in the CI static config is possible (it is a static option, per `static_config.go`) but would make CI differ from production; prefer a generous poll (for example 10 seconds).
- Invalid dynamic config: the file provider logs the decode error; whether Traefik keeps the previous good configuration on a parse failure was not verified in this pass. UNVERIFIED. This matters for the apply pipeline's guarantee that only validated config is written; worth one harness test (write a broken file, assert old routes remain).
- `certificatesResolvers`, entrypoints and `api` are static config, so changing them needs a restart, as the earlier note states (the resolver case remains inferred, not directly quoted). The harness's restart path (`TraefikController.Restart`) should be tested by killing and restarting the child process, not by sending a signal, since no SIGHUP reload is documented.

## 6. What the harness can and cannot settle

Can settle in CI on a plain runner with HTTP on high ports:

- Routers, services and middlewares from `render` load with status `enabled` (rule syntax, `ipAllowList`, `basicAuth`, `headers`, `redirectScheme`, `serversTransport`).
- Host-header routing to a stub upstream (a Go `httptest` server in the test), forced-HTTPS redirect responses, basic-auth 401, IP allow-list 403, hot reload of changes.
- Loopback API address, file-watch reload, and `/api/version` equals the pin.
- The "confirm at build time" items in architecture section 13 that do not involve ACME: the custom-certificate PEM inline-or-file question (load a TLS entry with a self-signed cert, check `/api/http/routers` and a TLS handshake with `-k` and SNI) and the redirect middleware for Redirection Hosts. The `tls.domains` wildcard shape can be checked for config acceptance (no router error), but real issuance needs a DNS challenge and cannot run in CI.

Cannot settle: real ACME issuance (needs public DNS or a Pebble test CA, outside this issue), s6 and systemd supervision, and the Docker bind-mount watch case noted above.

## 7. State of CI today

- `.github/` does not exist. `ls .github` returns "No such file or directory" in `/home/user/homelab-proxy-manager`. VERIFIED.
- Architecture section 3 names "GoReleaser on GitHub Actions per `vX.Y.Z` tag" and says "CI blocks on type-check, lint and tests"; section 11 says "in CI, start a pinned Traefik". All are plans.
- Suggested first workflow (`.github/workflows/ci.yml`, for the implementing ticket, not created here): jobs `go` (vet, unit tests, golden files), `integration` (Option A binary plus `go test -tags integration ./internal/...`), later `web`. Gate the integration job behind a `TRAEFIK_BIN` environment variable so `go test ./...` on a developer machine skips it cleanly instead of failing.
- The release workflow (GoReleaser, cosign) is separate and out of scope here.

## 8. Open items for the build

- Run the harness once locally and on a runner to turn the UNVERIFIED rows in sections 4 and 5 into VERIFIED (invalid-config behaviour, `dashboard: false` with API, exact `@file` suffix).
- Decide where the expected sha256 values live (workflow versus `packaging/`).
- Decide whether to add the weekly "newest v3" job from section 1.
