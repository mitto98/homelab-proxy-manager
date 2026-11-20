# Traefik distribution research

Researched 2026-11-19. Primary sources only. Items marked UNVERIFIED could not be confirmed.

## Access notes (blocked sources)

Through the agent proxy:

- Blocked (proxy 403 / no tunnel): doc.traefik.io, tracker.debian.org, packages.debian.org, deb.debian.org, ftp.debian.org, sources.debian.org, packages.fedoraproject.org, src.fedoraproject.org, dl.fedoraproject.org (EPEL), kojipkgs/bodhi, repology.org.
- GitHub REST API and github.com/.../releases HTML pages: 403. Release asset downloads (github.com/traefik/traefik/releases/download/...), raw.githubusercontent.com, and a shallow git clone of traefik/traefik all worked.
- Reachable: Docker Hub API (hub.docker.com/v2), docker-library/official-images, packages.ubuntu.com, archive.ubuntu.com.
- The Traefik docs were read from the docs source in the tagged repo (`docs/content/` at v3.7.19). This is the same content doc.traefik.io is built from, but it is not the rendered site.

## 1. Official apt/deb/rpm packages and distro availability

- Traefik Labs official deb/rpm: none found. The release workflow only uploads `.tar.gz`/`.zip` archives, a checksums file and a source tarball (see section 2). Neither the release workflow nor `.goreleaser.yml.tmpl` has an nfpm/deb/rpm step.
  - https://github.com/traefik/traefik/blob/v3.7.19/.github/workflows/release.yaml
  - https://github.com/traefik/traefik/blob/v3.7.19/.goreleaser.yml.tmpl
- The install docs at v3.7.19 (`docs/content/getting-started/`) only cover Docker and Kubernetes/Helm; there is no apt/rpm instruction. grep for `apt-get`, `.deb`, `.rpm`, `dnf install` across docs/content returned nothing relevant.
- Probing apt.traefik.io and pkgs.traefik.io was blocked by the proxy. Treat the absence of a Traefik-Labs repo as "none documented", not as proven nonexistence of such a host.
- A `traefik_v3.7.19_linux_amd64.deb` release asset returns 404.
- Ubuntu: no `traefik` package in any suite. packages.ubuntu.com name search "traefik" (all suites) finds only two library packages, `golang-github-traefik-paerser-dev` and `golang-github-traefik-yaegi-dev`. The same two source packages appear in a source-name search.
  - https://packages.ubuntu.com/search?keywords=traefik&searchon=names&suite=all&section=all
  - https://packages.ubuntu.com/search?keywords=traefik&searchon=sourcenames&suite=all&section=all
- Debian: UNVERIFIED (tracker.debian.org, packages.debian.org and mirrors blocked). The Debian archive contains packages for those Go libraries via the Ubuntu import, but I could not confirm whether a `traefik` source package exists in Debian.
- Fedora: UNVERIFIED (src.fedoraproject.org and packages.fedoraproject.org blocked).
- RHEL/EPEL: UNVERIFIED (dl.fedoraproject.org blocked).
- Implication: the project will have to build its own deb/rpm, or bundle or download the upstream release tarball. Docker is the only upstream-supported packaged distribution found.

## 2. GitHub release assets

Verified by downloading `traefik_v3.7.19_checksums.txt` from
https://github.com/traefik/traefik/releases/download/v3.7.19/traefik_v3.7.19_checksums.txt

The checksums file lists these archives:

- linux: `traefik_v3.7.19_linux_{386,amd64,armv6,armv7,arm64,ppc64le,riscv64,s390x}.tar.gz`
- darwin: `amd64` and `arm64` (`.tar.gz`)
- freebsd: `386` and `amd64` (`.tar.gz`)
- openbsd: `386`, `amd64` and `riscv64` (`.tar.gz`)
- windows: `386`, `amd64` and `arm64` (`.zip`)
- Naming template: `traefik_v{{Version}}_{{Os}}_{{Arch}}[v{{Arm}}]` with `.tar.gz` (`.zip` on Windows). Source: `archives.name_template` in https://github.com/traefik/traefik/blob/v3.7.19/.goreleaser.yml.tmpl
- Each archive contains the `traefik` binary plus `LICENSE.md` and `CHANGELOG.md` (same file; `archives.files`). The binary is static (`CGO_ENABLED=0`, `-trimpath`).
- Checksums: one combined file `traefik_v3.7.19_checksums.txt`, per `checksum.name_template` and the "Publish Release" step in https://github.com/traefik/traefik/blob/v3.7.19/.github/workflows/release.yaml. The hash algorithm is UNVERIFIED because I only extracted filenames. goreleaser's default is sha256.
- Source tarball: `traefik-v3.7.19.src.tar.gz` (HTTP 200 on download; created in release.yaml).
- No `.deb` or `.rpm` assets (see section 1).
- No signature files (`.sig` / `.asc`) are produced by the release workflow. I did not see a signing step.
- Note: the release is created with `--latest=false` in release.yaml, so GitHub's "Latest" badge may not point at it. Do not rely on `/releases/latest` to find the newest version; use tags or the Docker Hub tag list.

## 3. Official `traefik` Docker image

The Docker Hub official image (`library/traefik`) is built from https://github.com/traefik/traefik-library-image, not from the Dockerfile in the main repo. Mapping: https://raw.githubusercontent.com/docker-library/official-images/master/library/traefik lists `GitRepo: https://github.com/traefik/traefik-library-image.git`.

For tag `v3.7.19` / `3.7` / `3` / `latest` (codename `langres`), directory `v3.7/alpine`, commit `991f7219c159647ae8011a7cee88ac500ef9aeeb`:

- Dockerfile: https://raw.githubusercontent.com/traefik/traefik-library-image/991f7219c159647ae8011a7cee88ac500ef9aeeb/v3.7/alpine/Dockerfile
- Entrypoint: https://raw.githubusercontent.com/traefik/traefik-library-image/991f7219c159647ae8011a7cee88ac500ef9aeeb/v3.7/alpine/entrypoint.sh

Facts:

- Base: `alpine:3.24`; installs `ca-certificates` and `tzdata`.
- Binary: downloaded from the GitHub release tarball (`traefik_v3.7.19_linux_$arch.tar.gz`) and extracted to `/usr/local/bin/traefik`. This uses the same archives as section 2.
- Architectures built: amd64, arm32v6, arm64v8, ppc64le, riscv64, s390x (official-images file). The Dockerfile also maps `armv7`, but the official-images listing does not include arm32v7.
- `EXPOSE 80`
- `ENTRYPOINT ["/entrypoint.sh"]`, `CMD ["traefik"]`
- `/entrypoint.sh` (copied to `/`):
  - If the first arg starts with `-`, it prepends `traefik`.
  - If `traefik "$1" --help` succeeds, it runs `traefik "$@"` (so `docker run traefik version` works).
  - Otherwise it prints a notice and `exec`s the arguments as a shell command.
- No `USER` instruction, so the container runs as root. No `VOLUME`. No config files are shipped. Config is expected at `/etc/traefik/traefik.yml` (search path from the docs), mounted in.
- Labels: OCI metadata (vendor "Traefik Labs", version `v3.7.19`, etc.).
- Docker Hub tag API confirms `latest`, `3.7.19` and `v3` all share digest `sha256:a6c0194dda77bcb804824580f36fa7edc56e315b1acfe933366c8b52844a78a4`: https://hub.docker.com/v2/repositories/library/traefik/tags/latest
- Multi-arch: manifest includes amd64, arm64 (v8), arm (v6), ppc64le, riscv64 and s390x, plus Windows nanoserver/servercore tags.
- Separate: the main repo's own `Dockerfile` (at v3.7.19, `FROM alpine:3.24`, `COPY ./dist/$TARGETPLATFORM/traefik /`, `EXPOSE 80`, `VOLUME ["/tmp"]`, `ENTRYPOINT ["/traefik"]`) puts the binary at `/traefik` with no entrypoint script. It is not the Docker Hub official image, so do not assume `/traefik` for `traefik:v3`. Source: https://github.com/traefik/traefik/blob/v3.7.19/Dockerfile

## 4. Current stable v3 version

- Latest stable: v3.7.19. Evidence:
  - `git ls-remote --tags https://github.com/traefik/traefik` shows v3.7.19 as the highest stable v3 tag; newer tags are only `v3.8.0-rc.1/2/3` (pre-release).
  - Docker Hub `latest` = `3.7.19` (same digest), while `3.8.0-rc.3` is separate.
- Date: the tag commit "Prepare release v3.7.19" is dated 2026-11-12T11:14:06+01:00 (commit `7dafd6dba371bd1aee70c443c2975c229c969711`). The official GitHub release publication timestamp is UNVERIFIED (API/releases page blocked). Docker Hub images were pushed 2026-11-19T02:09Z (that is the image rebuild time, not the release date).
- v3.8.0 is at release-candidate stage (rc.3). v2.11.63 is the current v2 line.

## 5. Static vs dynamic config reload

Source: Traefik docs, repo copy at v3.7.19.

- Install (static) configuration "defines parameters that require Traefik to restart when changed. This includes entry points, providers, API/dashboard settings, and logging levels." Routing (dynamic) config covers "routers, services, and middlewares" and "can be updated without restarting Traefik." (`docs/content/reference/install-configuration/boot-environment.md`: https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/boot-environment.md)
- Routing config "can change and is seamlessly hot-reloaded, without any request interruption or connection loss." HTTPS certificates belong to the routing config and can be added, updated or removed without restart. (`docs/content/getting-started/configuration-overview.md`: https://github.com/traefik/traefik/blob/v3.7.19/docs/content/getting-started/configuration-overview.md)
- Static config can be given by exactly one of: config file, CLI args, env vars (mutually exclusive). The file is searched as `traefik.yml|yaml|toml` in `/etc/traefik/`, `$XDG_CONFIG_HOME/`, `$HOME/.config/`, `.`; override with `--configFile`. (same page)
- File provider: `providers.file.directory` (recommended) or `providers.file.filename` (mutually exclusive); `providers.file.watch` defaults to `true` and works with both options. Watching relies on fsnotify, which can miss changes when mounts or symlinks break the link between file systems; the docs recommend setting `directory` to the parent directory or bind-mounting it. (`docs/content/reference/install-configuration/providers/others/file.md`: https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/providers/others/file.md)
- `providersThrottleDuration` (default 2s): minimum wait after a config reload before the next provider event is applied; only the most recent event within the window is used. (same file)
- `certificatesResolvers` is install (static) configuration, so changing it requires a restart. The docs do not state this explicitly on the resolver pages that I read; it is inferred from the boot-environment page ("providers" etc. require restart) and the fact that resolvers are defined only in static config. Treat as likely but UNVERIFIED as a direct quote. One explicit data point: ACME docs note that for new (sub)domains needing Let's Encrypt authentication, "the default Traefik certificate will be used until Traefik is restarted" (`docs/content/reference/install-configuration/tls/certificate-resolvers/acme.md`, line ~332).
- No documented SIGHUP-based reload for static config was found in `docs/content/reference/install-configuration` (grep for SIGHUP and reload only hit provider throttling and an unrelated entrypoints note at `entrypoints.md:540` about reloading config without downtime).
- Implication for the panel: write dynamic config as files in a watched directory (no restart needed). For entrypoints/certificatesResolvers changes, restart or recreate the traefik process/container.
