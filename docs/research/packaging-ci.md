# Packaging, signing and package CI research

Researched 2026-11-22. Primary sources only (GoReleaser v2.20.3, nfpm v2.49.0, cosign v3.1.4 and v2.6.1, systemd v255, modernc.org/sqlite v1.62.0, SQLite docs, runner-image inventories and image-publisher pages). Items marked UNVERIFIED could not be confirmed. Feeds issue #12 and `docs/architecture.md` section 10.

## Summary

- **Pin GoReleaser v2.20.3** (tag commit `a6edafaa`, 2026-11-20; vendors nfpm v2.49.0, needs Go 1.27.4). Run it with `goreleaser/goreleaser-action@v7` (latest tag v7.3.0) and `version: "v2.20.3"`.
- **The whole pipeline ran live** on a throwaway fixture: `before.hooks` frontend step, linux amd64 and arm64 static binaries with the tag in `-X main.version`, `.deb` and `.rpm` for both arches, `checksums.txt`. Not run: Docker image build (no Docker daemon in the sandbox) and keyless signing (sigstore hosts are blocked here).
- **"Three units" is really four.** A `.path` unit whose `Unit=` is the Traefik service does **not** restart a running Traefik (a start on an active unit is a no-op; VERIFIED live). The path unit must activate a small `Type=oneshot` service that runs `systemctl try-restart homelab-proxy-manager-traefik.service`.
- **systemd forbids a variable as the program in `ExecStart=`.** "Traefik binary path set in an environment file" (section 10) therefore needs `ExecStart=/bin/sh -c 'exec "$TRAEFIK_BIN" ...'` or a wrapper script. Source: systemd.service(5), "the first argument ... may not be a variable".
- **Do not ship `/var/lib/homelab-proxy-manager` in the package.** dpkg removed the (empty) package-owned directory on `dpkg -r`; with `StateDirectory=` systemd creates it with the right owner and nothing in the package owns it. VERIFIED live. Also, with the user created in `postinstall`, an nfpm `dir` entry owned by that user silently became `root:root` on a fresh `.deb` install (VERIFIED live).
- **Config files**: `type: config|noreplace` works for both formats. rpm keeps the local file and writes `.rpmnew`; dpkg keeps it only with `--force-confold`, otherwise a non-interactive upgrade with a modified conffile and a changed shipped default **fails** ("end of file on stdin at conffile prompt"). VERIFIED live.
- **Prereleases**: `dockers_v2` has no `skip_push`. Use template tags that render empty for prereleases (empty tags are dropped). Rendered live: `v1.2.3-rc.1` gives `1.2.3-rc.1` and nothing else; `v1.2.3` gives `1.2.3`, `1.2`, `latest`. The legacy `dockers.skip_push: auto` skips the **whole** push for a prerelease, so it is the wrong tool if you want rc images published.
- **Cosign v3 changed the CLI**: `sign-blob --bundle <file>` is required and always writes the new bundle format (`application/vnd.dev.sigstore.bundle.v0.3+json`, VERIFIED live with a key). In v2.6.x the new format is opt-in with `--new-bundle-format`.
- **systemd works inside a plain pid+mount namespace on this sandbox** (Ubuntu 24.04, systemd 255), so the whole install/path-unit/upgrade/remove recipe was run for real, but on an **Ubuntu 24.04 host with `dpkg` only**. Not run: Debian 12, Ubuntu 22.04, Fedora, Rocky/Alma 9, arm64, any Docker or Podman container, any GitHub-hosted runner, rpm under systemd. See section 3.
- **`ubuntu-latest` is mid-migration to Ubuntu 26.04** (runner-images announcement, November 2026). Pin `ubuntu-24.04` and `ubuntu-24.04-arm`.
- **SQLite backup**: both `VACUUM INTO` and the driver's backup API are safe while the Panel runs in WAL mode. `Backup.Step(-1)` in one call or `VACUUM INTO` work; incremental stepping against a live writer never finished in the test (section 4). Prefer `VACUUM INTO`.

## Access notes

- Reachable and used: GitHub release tags and raw files at the pinned tags (goreleaser v2.20.3, nfpm v2.49.0, cosign v3.1.4 and v2.6.1, systemd v255, actions/runner-images main), Go module proxy, Docker Hub pages (through the Firecrawl scraper), `www.sqlite.org` (through the Firecrawl scraper only; direct `curl` got 403), Ubuntu archive (installed `rpm`, `lintian`, `rpmlint`).
- Not reachable: `goreleaser.com`, `docs.sigstore.dev` (docs were read from the tagged repos instead), `tuf-repo-cdn.sigstore.dev` (403, so cosign cannot fetch its trusted root or signing config; keyless sign and `verify-blob` with a real bundle were **not run**), GitHub REST API and releases HTML pages, the Docker daemon (none in the sandbox).
- The SQLite page quotes below came back through Firecrawl's "directQuote" extraction of a cached copy (2026-11-22), not through a raw fetch.
- **Sandbox accident, worth knowing**: booting systemd as PID 1 in a private PID namespace that can still see the host `/tmp` ran `systemd-tmpfiles` and emptied `/tmp`, including my scratch directory. Second run mounted a private tmpfs over `/tmp` first. Not an issue for containers, only for this experiment style.
- Tools actually executed here: GoReleaser v2.20.3 and nfpm v2.49.0 (built with `go install`), cosign v3.1.4 (key mode only), dpkg 1.22.6, rpm 4.18.2, lintian 2.117.0, rpmlint 2.5.0, systemd 255.4, modernc.org/sqlite v1.62.0 (bundles SQLite 3.54.1).

## 1. GoReleaser and nfpm

### 1.1 Versions

| Claim | Status | Source |
| --- | --- | --- |
| GoReleaser v2.20.3 is the latest stable tag (2026-11-20); newer tags are `v2.21.0-<sha>-nightly` | VERIFIED | `git ls-remote --tags https://github.com/goreleaser/goreleaser` ; `git log` of the tag |
| v2.20.3 vendors nfpm `v2.49.0` and requires Go 1.27.4 | VERIFIED | https://github.com/goreleaser/goreleaser/blob/v2.20.3/go.mod lines 3 and 37 |
| `goreleaser/goreleaser-action` latest is v7.3.0; the docs example uses `actions/checkout@v7`, `actions/setup-go@v7`, `fetch-depth: 0`, `version: "~> v2"`, `args: release --clean` | VERIFIED | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/ci/actions.md ; tags of the action repo |
| The action does not install Docker, cosign, QEMU or log in to registries | VERIFIED | same actions.md, "The action does not install, configure or authenticate into dependencies" |
| Other action tags seen: `sigstore/cosign-installer` v4.1.3 (default `cosign-release: v3.1.4`), `docker/login-action` v4.8.0, `docker/setup-buildx-action` v4.4.1, `docker/setup-qemu-action` v4.4.0, `actions/setup-go` v7.1.0 | VERIFIED (tags exist); `cosign-installer` default read from its `action.yml` at v4.1.3 | `git ls-remote --tags` of each repo; https://github.com/sigstore/cosign-installer/blob/v4.1.3/action.yml |

Pin actions by commit SHA in the real workflow; tag names above are only for orientation.

### 1.2 Build, hook, version

| Claim | Status | Source |
| --- | --- | --- |
| `before.hooks` (OSS) runs once, before the build, as plain commands; templates allowed | VERIFIED | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/general/hooks.md ; live run executed `sh web/build.sh` once for two targets |
| `builds[].hooks.pre` runs **per target** (so would run the frontend build twice) | VERIFIED | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/builds/hooks.md |
| Pipeline order puts the git dirty check (`git.Pipe`) **before** `before.Pipe`, and nothing re-checks later. So a hook may overwrite a tracked `web/dist/placeholder` without failing the release | VERIFIED | `internal/pipeline/pipeline.go` and `internal/pipe/git/git.go` at v2.20.3; live run: tracked file modified by the hook, release succeeded, `git status` showed ` M` afterwards |
| Default ldflags are `-s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}} -X main.builtBy=goreleaser`; `CGO_ENABLED=0` is the default env for Go builds | VERIFIED (ldflags from doc); CGO set explicitly in the test config | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/builds/builders/go.md line 68 |
| `{{ .Version }}` has the `v` stripped; `.Major`, `.Minor`, `.Patch`, `.Prerelease` (e.g. `rc.1`) exist | VERIFIED | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/general/templates.md ; live run with tag `v1.2.3-rc.1` printed `prerelease=[rc.1]` |
| Live run result: static ELF for arm64, binary reported `version=1.2.3-rc.1 commit=<full sha> ui="<html>built-ui</html>"` (the frontend step output was embedded via `go:embed`) | VERIFIED live | fixture in the scratch dir, not committed |

Config used in the live run (trimmed to what was executed, names changed only in the probe line):

```yaml
version: 2
project_name: homelab-proxy-manager

before:
  hooks:
    - sh -c 'npm ci --prefix web && npm run build --prefix web'   # live run used a stub script instead

builds:
  - id: panel
    main: ./cmd/homelab-proxy-manager
    binary: homelab-proxy-manager
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64]
    flags: [-trimpath]
    ldflags:
      - -s -w -X main.version={{ .Version }} -X main.commit={{ .FullCommit }} -X main.date={{ .CommitDate }}
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: tarball
    ids: [panel]
    formats: [tar.gz]

checksum:
  name_template: checksums.txt

release:
  prerelease: auto
```

Notes:

- Architecture section 4 says a placeholder lives in `web/dist` and the real build is "never committed". Keep the built files ignored and only the placeholder tracked, as in the live run (`.gitignore`: `web/dist/*` then `!web/dist/placeholder.txt`; a broader `dist/` rule earlier in the file silently defeats the `!` line, which bit me once).
- `release.prerelease: auto` marks the GitHub release as a prerelease when the tag has a prerelease part (documented in `.../publish/scm/_index.md`). VERIFIED (doc only, publishing was skipped).
- Prerelease **package** file names use `~`: `homelab-proxy-manager_1.2.3~rc.1_amd64.deb`, `...-1.2.3~rc.1-1.x86_64.rpm` (VERIFIED live; the `~` is the dpkg/rpm convention for sorting below the release, ordering not tested here). Whether GitHub keeps `~` in uploaded asset names so that `sha256sum --check` still matches `checksums.txt` is UNVERIFIED (releases API blocked). Stable tags have no `~`.

### 1.3 nfpm: `nfpms` keys

GoReleaser's key names (from `nfpm.md` at v2.20.3), with what each does for our needs:

| Need | Exact keys | Status |
| --- | --- | --- |
| Formats and names | `nfpms[].formats: [deb, rpm]`, `package_name`, `file_name_template: "{{ .ConventionalFileName }}"`, `ids: [panel]` (v2.8 renamed `builds` to `ids`), `bindir: /usr/bin` | VERIFIED live |
| Units and env file | `contents: [{src, dst}]` with `dst: /usr/lib/systemd/system/<unit>` | VERIFIED live (dpkg unpacked them; systemd 255 loaded them) |
| Local edits survive upgrade | `contents: - src: ...env, dst: /etc/homelab-proxy-manager/homelab-proxy-manager.env, type: config|noreplace, file_info: {mode: 0640}` | VERIFIED live, see 1.4 |
| Scripts | `scripts: {preinstall, postinstall, preremove, postremove}`; rpm extras `rpm.scripts.pretrans/posttrans/verify` | VERIFIED live. Script text is copied **verbatim** into `postinst`/`prerm`/`postrm` and rpm scriptlets, so argument conventions differ per format and are our job |
| Per-format content | `contents[].packager: deb` or `rpm` | VERIFIED (doc) |
| Lintian overrides | `deb.lintian_overrides: [...]` | VERIFIED (doc) |
| rpm install-time dependency | `rpm.requires.post: [shadow-utils]` (nfpm config doc: "Adds `Requires(post): systemd`" example) | VERIFIED (key exists in https://github.com/goreleaser/nfpm/blob/v2.49.0/www/content/docs/configuration.md); whether `shadow-utils` is needed on Fedora/Rocky minimal images is UNVERIFIED |
| Claim ownership of a directory | `contents: - dst: /x, type: dir, file_info: {mode, owner, group}` | VERIFIED live, **but do not use it for the state dir** (1.5) |
| systemd integration | nfpm has **no** built-in systemd helper (no `%systemd_post`, no `deb-systemd-helper`); the `tips.md` page only gives advice | VERIFIED (config reference and tips at v2.49.0). We call `systemctl` ourselves |

nfpm.yaml equivalent used standalone (the GoReleaser `nfpms` block takes the same content keys):

```yaml
contents:
  - src: dist/homelab-proxy-manager
    dst: /usr/bin/homelab-proxy-manager
  - src: packaging/homelab-proxy-manager.service
    dst: /usr/lib/systemd/system/homelab-proxy-manager.service
  - src: packaging/homelab-proxy-manager-traefik.service
    dst: /usr/lib/systemd/system/homelab-proxy-manager-traefik.service
  - src: packaging/homelab-proxy-manager-traefik.path
    dst: /usr/lib/systemd/system/homelab-proxy-manager-traefik.path
  - src: packaging/homelab-proxy-manager-traefik-restart.service
    dst: /usr/lib/systemd/system/homelab-proxy-manager-traefik-restart.service
  - src: packaging/homelab-proxy-manager.env
    dst: /etc/homelab-proxy-manager/homelab-proxy-manager.env
    type: config|noreplace
    file_info: {mode: 0640}
scripts:
  postinstall: packaging/scripts/postinstall.sh
  preremove: packaging/scripts/preremove.sh
  postremove: packaging/scripts/postremove.sh
```

### 1.4 What `config|noreplace` really does (live)

nfpm maps `config|noreplace` to `%config(noreplace)` in rpm and to a `conffiles` entry in deb (the deb code treats `config`, `config|noreplace` and `config|missingok` identically). Source: `deb/deb.go` `conffiles()` and `rpm/spec.go` at v2.49.0.

| Scenario | dpkg 1.22.6 | rpm 4.18.2 |
| --- | --- | --- |
| Upgrade, local edit, shipped file **unchanged** | local edit kept silently (VERIFIED) | not run |
| Upgrade, local edit, shipped file **changed**, no flag | **fails non-interactively**: `end of file on stdin at conffile prompt`; package left unconfigured (VERIFIED) | local file kept, new default written as `.rpmnew`, warning printed (VERIFIED) |
| Same with `dpkg --force-confold` | local file kept, new default written as `.dpkg-dist` (VERIFIED) | n/a |
| Remove (not purge) | conffile stays; `dpkg -P` deletes `/etc/homelab-proxy-manager` (VERIFIED) | `rpm -e` renames a modified file to `.rpmsave` (VERIFIED) |

Consequences: in CI use `dpkg --force-confold -i` or `apt-get -o Dpkg::Options::=--force-confold install ./x.deb`. Whether plain `apt-get install ./x.deb` with `DEBIAN_FRONTEND=noninteractive` behaves like `dpkg` here is UNVERIFIED. The `.dpkg-dist` and `.rpmnew` files mean the Panel should never glob `/etc/homelab-proxy-manager/*`.

### 1.5 Dedicated user, state directory, scripts

- A dedicated user in `postinstall` works (`useradd --system --user-group --home-dir /var/lib/homelab-proxy-manager --no-create-home --shell /usr/sbin/nologin`), guarded by `getent passwd`. VERIFIED live on dpkg and rpm.
- `nologin` path: `/usr/sbin/nologin` exists on Debian and Ubuntu; on Fedora and RHEL it is also `/usr/sbin/nologin`. UNVERIFIED on those images.
- **Ownership trap** (VERIFIED live, dpkg 1.22.6): a `dir` entry with `owner: homelab-proxy-manager` is stored by name in the tar; with the user not yet created (because `postinstall` runs after unpack) dpkg unpacked it as `root:root` mode 0750, with no error. The Panel would then fail to write. Creating the user in `preinstall` would avoid it, but is not needed:
- **Use `StateDirectory=homelab-proxy-manager` (and `StateDirectoryMode=0750`) in the Panel unit.** systemd creates `/var/lib/homelab-proxy-manager` owned by `User=` at start (VERIFIED live: `drwxr-x--- homelab-proxy-manager homelab-proxy-manager`). The directive exists since systemd v235 (NEWS), so it works on Debian 12 (252), Ubuntu 22.04 (249), RHEL 9 (252) and current Fedora; those version numbers are from memory and UNVERIFIED, the v235 introduction is VERIFIED.
- **Removal keeps data** only if the package does not own the directory: with a package-owned empty directory `dpkg -r` removed it (VERIFIED live); with data inside it, it stays. With `StateDirectory=` nothing in the package owns it, and remove and purge both left `panel.db`, `traefik.yml` etc. (VERIFIED live). The system user also stays, because `postremove` never calls `userdel` (VERIFIED live).
- First-install `systemctl enable` does not start the service (VERIFIED live: Panel `inactive` after install). Use `enable --now` for the Panel if you want auto-start, or print an instruction.
- Argument conventions in `preremove`: deb `$1` is `remove`, `upgrade`, ...; rpm `$1` is the number of instances left (`0` on final removal). The script above handles both with `case "$1" in remove|0)`. The rpm convention (`$1` = instances remaining) is from memory of the RPM manual and was NOT tested here, because my rpm runs had no systemd and the `case` branch is a no-op without it. Verify with the container recipe in 3.4.

### 1.6 The units (all four run live)

```ini
# homelab-proxy-manager.service
[Service]
Type=simple
User=homelab-proxy-manager
Group=homelab-proxy-manager
EnvironmentFile=/etc/homelab-proxy-manager/homelab-proxy-manager.env
StateDirectory=homelab-proxy-manager
StateDirectoryMode=0750
ExecStart=/usr/bin/homelab-proxy-manager serve
Restart=on-failure
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target

# homelab-proxy-manager-traefik.service
[Unit]
ConditionPathExists=/var/lib/homelab-proxy-manager/traefik.yml
[Service]
User=homelab-proxy-manager
Group=homelab-proxy-manager
EnvironmentFile=/etc/homelab-proxy-manager/homelab-proxy-manager.env      # TRAEFIK_BIN=/usr/local/bin/traefik
EnvironmentFile=-/var/lib/homelab-proxy-manager/traefik.env
ExecStartPre=/bin/sh -c 'test -x "$TRAEFIK_BIN" || { echo "Traefik not found at $TRAEFIK_BIN" >&2; exit 1; }'
ExecStart=/bin/sh -c 'exec "$TRAEFIK_BIN" --configFile=/var/lib/homelab-proxy-manager/traefik.yml'
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
Restart=on-failure
[Install]
WantedBy=multi-user.target

# homelab-proxy-manager-traefik.path
[Path]
PathChanged=/var/lib/homelab-proxy-manager/traefik.yml
PathChanged=/var/lib/homelab-proxy-manager/traefik.env
Unit=homelab-proxy-manager-traefik-restart.service
[Install]
WantedBy=multi-user.target

# homelab-proxy-manager-traefik-restart.service
[Service]
Type=oneshot
ExecStart=/usr/bin/systemctl try-restart homelab-proxy-manager-traefik.service
```

| Claim | Status | Source |
| --- | --- | --- |
| A variable cannot be the program in `ExecStart=`; `$VAR`/`${VAR}` substitution works only in arguments | VERIFIED | https://github.com/systemd/systemd/blob/v255/man/systemd.service.xml ("the first argument ... may not be a variable") |
| `Unit=` of a path unit names the unit to **activate**; it defaults to the same-named `.service` | VERIFIED | https://github.com/systemd/systemd/blob/v255/man/systemd.path.xml |
| `Unit=homelab-proxy-manager-traefik.service` directly does **not** restart a running Traefik (MainPID unchanged after the watched file changed) | VERIFIED live | section 3 run |
| Via the oneshot `try-restart` service: restart happens, and is a no-op while Traefik is stopped | VERIFIED live | section 3 run |
| `PathChanged=` fires on in-place writes, on atomic `mv tmp file` replacement, and when the file is created for the first time; it did not fire for an unrelated file | VERIFIED live | section 3 run |
| `AmbientCapabilities=CAP_NET_BIND_SERVICE` gives the non-root Traefik process the capability: `CapAmb: 0000000000000400` (bit 10) in `/proc/<pid>/status` | VERIFIED live | section 3 run |
| `AmbientCapabilities=` exists since systemd v229 | VERIFIED | https://github.com/systemd/systemd/blob/v255/NEWS |
| `PathChanged` rate-limits via `TriggerLimitIntervalSec=2s`, `TriggerLimitBurst=200` by default | VERIFIED (doc) | systemd.path.xml v255. A Panel that writes both files within one apply causes two restarts unless the apply writes `traefik.env` first and the restart is debounced; UNVERIFIED how the Panel writes them |
| `ConditionPathExists=` keeps Traefik inactive (no failure) until `traefik.yml` exists | VERIFIED live | section 3 run |
| Enabling the path unit in `postinstall` and `systemctl start` it | VERIFIED live | postinstall output in section 3 |

### 1.7 Docker image to GHCR

| Claim | Status | Source |
| --- | --- | --- |
| `dockers_v2` (since v2.12) uses `docker buildx`, builds and **pushes** the multi-platform manifest in the **publish** phase, so `--skip=publish` also skips the image | VERIFIED (doc, source) | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/package/dockers_v2.md |
| Keys: `dockers_v2[].images`, `tags`, `platforms` (default linux/amd64, linux/arm64), `labels`, `annotations`, `extra_files`, `build_args`, `sbom` (default `true`), `disable` (templated bool), `dockerfile` | VERIFIED | same page; `internal/pipe/docker/v2/docker.go` defaults |
| The name `dockers_v2` is "provisional" and becomes `dockers` in GoReleaser v3; the old `dockers` + `docker_manifests` remain in v2 | VERIFIED | warning box in dockers_v2.md |
| The build context holds prebuilt binaries at `linux/amd64/<bin>` and `linux/arm64/<bin>`; the Dockerfile should `COPY $TARGETPLATFORM/<bin>` and the docs warn against building inside the Dockerfile | VERIFIED | dockers_v2.md "The Docker build context". **This conflicts with architecture section 10** ("Multi-stage build: build the Panel ..."); the Traefik copy step (`COPY --from=traefik:v3.7.19 ...`) and the s6 layers can stay |
| Empty rendered tags are dropped; at least one tag must remain | VERIFIED | `tpl.Slice(d.Tags, tmpl.NonEmpty())` and `"no tags provided"` in `internal/pipe/docker/v2/docker.go` |
| Multi-arch from an amd64 runner needs QEMU/binfmt if the Dockerfile has `RUN` steps for arm64 (apk installs, s6 extraction) | VERIFIED (doc) | dockers_v2.md "Setting up a builder": `docker run --privileged --rm tonistiigi/binfmt --install all`; in Actions use `docker/setup-qemu-action` |
| Needed workflow permissions: `contents: write`, `packages: write` (GHCR via `docker/login-action` with `GITHUB_TOKEN`), `id-token: write` (cosign) | VERIFIED (doc comments in actions.md list `packages: write` and `id-token: write`) | actions.md; cosign-installer README |

Tags that satisfy "latest, X.Y.Z, X.Y, and prereleases move nothing but their own tag":

```yaml
dockers_v2:
  - id: image
    images: ["ghcr.io/mitto98/homelab-proxy-manager"]
    tags:
      - "{{ .Version }}"                                                   # 1.2.3 or 1.2.3-rc.1
      - "{{ if not .Prerelease }}{{ .Major }}.{{ .Minor }}{{ end }}"       # 1.2, empty for rc
      - "{{ if not .Prerelease }}latest{{ end }}"                          # empty for rc
    platforms: [linux/amd64, linux/arm64]
    dockerfile: packaging/Dockerfile
    labels:
      org.opencontainers.image.source: "{{ .GitURL }}"
      org.opencontainers.image.version: "{{ .Version }}"
```

Live check of the three expressions inside a `before.hook` (templates are rendered by the same engine as `tags`):

| Tag | Rendered |
| --- | --- |
| `v1.2.3-rc.1` | `TAGS: 1.2.3-rc.1 \|  \|  \| prerelease=[rc.1]` (only the first tag survives) |
| `v1.2.3` | `TAGS: 1.2.3 \| 1.2 \| latest \| prerelease=[]` |

The image build itself was **not run** (no Docker daemon).

`skip_push: auto` and prerelease handling:

- Legacy `dockers[].skip_push` and `docker_manifests[].skip_push` accept `true`, `false`, or `auto`. `auto` skips the push when `ctx.Semver.Prerelease != ""`, i.e. for `v1.2.3-rc.1`, and it skips the **entire** push, not only the moving tags. VERIFIED in `internal/pipe/docker/docker.go` (lines 380-389) and `manifest.go` at v2.20.3; doc text in `docker.md` ("If set to auto, the release will not be pushed ... prerelease").
- `dockers_v2` has **no** `skip_push` key (not in the doc, not read by the v2 pipe). To not publish rc images at all use `disable: '{{ if .Prerelease }}true{{ end }}'` (the field is rendered with `tpl.Bool`; VERIFIED in source, not executed). To publish rc images but not move the floating tags use the template tags above (executed).
- GitHub Release prerelease flag is separate: `release.prerelease: auto`.

## 2. Cosign keyless signing

### 2.1 GoReleaser `signs:` for the checksums file

Copied from the GoReleaser docs at v2.20.3 (`customization/sign/sign.md`, "Signing with cosign"):

```yaml
checksum:
  name_template: checksums.txt

signs:
  - cmd: cosign
    signature: "${artifact}.sigstore.json"
    args:
      - "sign-blob"
      - "--bundle=${signature}"
      - "${artifact}"
      - "--yes"
    artifacts: checksum
```

| Claim | Status | Source |
| --- | --- | --- |
| Signing the checksums file only is "generally enough"; `artifacts: checksum` | VERIFIED | sign.md |
| The signature file must exist after the command (`signature` template), GoReleaser warns "the signer did not write ..." otherwise (an error in v3) | VERIFIED | sign.md "Limitations" |
| `sign.Pipe` runs after `checksums.Pipe` in the pipeline | VERIFIED | `internal/pipeline/pipeline.go` |
| `goreleaser release --skip=sign` skips it (used in the live run so the build did not call cosign) | VERIFIED live | run output "skipping announce, publish, sign, and validate" |
| Running this block for real in Actions | **not run** (needs the OIDC token and sigstore network) | |

### 2.2 Workflow `permissions` and steps

```yaml
name: release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write    # create the release, upload assets
  packages: write    # push to ghcr.io
  id-token: write    # keyless cosign: request the GitHub OIDC token
jobs:
  release:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v7
        with: {fetch-depth: 0}
      - uses: actions/setup-go@v7
        with: {go-version-file: go.mod}
      - uses: actions/setup-node@<pin>        # frontend build
      - uses: sigstore/cosign-installer@v4.1.3   # installs cosign v3.1.4 by default
      - uses: docker/setup-qemu-action@v4
      - uses: docker/setup-buildx-action@v4
      - uses: docker/login-action@v4
        with: {registry: ghcr.io, username: "${{ github.actor }}", password: "${{ secrets.GITHUB_TOKEN }}"}
      - uses: goreleaser/goreleaser-action@v7
        with: {distribution: goreleaser, version: "v2.20.3", args: release --clean}
        env: {GITHUB_TOKEN: "${{ secrets.GITHUB_TOKEN }}"}
```

| Claim | Status | Source |
| --- | --- | --- |
| Keyless signing from Actions needs `id-token: write` | VERIFIED | https://github.com/sigstore/cosign-installer/blob/v4.1.3/README.md (example `permissions: id-token: write # needed for signing the images with GitHub OIDC Token`) |
| `sign-blob` with no `--key` and ambient credentials: `--oidc-provider` options include `github-actions`; if unset all providers are tried | VERIFIED | https://github.com/sigstore/cosign/blob/v3.1.4/doc/cosign_sign-blob.md |
| The Fulcio certificate SAN for GitHub Actions is the `job_workflow_ref`, which identifies the workflow file and ref | VERIFIED | https://github.com/sigstore/fulcio/blob/main/docs/oidc.md "Case study: GitHub Actions" |
| Resulting identity: `https://github.com/mitto98/homelab-proxy-manager/.github/workflows/release.yml@refs/tags/v1.2.3`, issuer `https://token.actions.githubusercontent.com` | VERIFIED as the GoReleaser-documented pattern (sign.md); not observed from a real run |

### 2.3 Bundle: cosign v3 vs v2

| | cosign v2.6.1 | cosign v3.1.4 |
| --- | --- | --- |
| `sign-blob --bundle <file>` | optional | **required** ("must specify --bundle with --new-bundle-format"; CHANGELOG v3.0.1: "moved from optional to required") |
| Format written | legacy by default; `--new-bundle-format` default `false` | new Sigstore bundle only; `--new-bundle-format` default `true` and deprecated |
| `--output-signature`, `--output-certificate` | accepted | deprecated, point to `--bundle` |
| `verify-blob --bundle` | needs `--new-bundle-format` for the new format (default `false`) | new format default |

Sources: `doc/cosign_sign-blob.md` and `cmd/cosign/cli/options/signblob.go` at https://github.com/sigstore/cosign/tree/v2.6.1 and `tree/v3.1.4`; `CHANGELOG.md` at v3.1.4. VERIFIED.

Live (key mode, offline, because sigstore hosts are blocked): `cosign sign-blob --key cosign.key --bundle checksums.txt.sigstore.json --use-signing-config=false --tlog-upload=false --yes checksums.txt` produced `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json", ... "messageSignature": ...}`, and `cosign verify-blob --key cosign.pub --bundle ... --insecure-ignore-tlog checksums.txt` printed `Verified OK`. Without `--bundle`, v3.1.4 refused to sign. The default (keyless) path fetches `https://tuf-repo-cdn.sigstore.dev/.../root.json` for the signing config and fails here with 403; that is a sandbox limit, not a cosign bug.

### 2.4 What the user runs

From the GoReleaser docs (same sign.md), with our repository substituted:

```sh
cosign verify-blob \
  --certificate-identity 'https://github.com/mitto98/homelab-proxy-manager/.github/workflows/release.yml@refs/tags/v1.2.3' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.sigstore.json checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

| Claim | Status | Source |
| --- | --- | --- |
| Both `--certificate-identity` (or `-regexp`) and `--certificate-oidc-issuer` (or `-regexp`) are required for keyless `verify-blob` | VERIFIED | https://github.com/sigstore/cosign/blob/v3.1.4/doc/cosign_verify-blob.md (option text) |
| Offline-friendly variant: add `--trusted-root trusted_root.json` | VERIFIED (flag exists) | same page |
| With cosign v2 (6.x) the user must add `--new-bundle-format` | VERIFIED | v2.6.1 `cosign_verify-blob.md` |
| A version-agnostic regexp is `--certificate-identity-regexp '^https://github.com/mitto98/homelab-proxy-manager/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$'` | VERIFIED (flag), regexp untested | |
| End-to-end run of this command against a real release | **not run** (TUF host blocked, no release yet) | |

### 2.5 Signing the image (`docker_signs`)

```yaml
docker_signs:
  - cmd: cosign
    artifacts: ""          # default: images built by dockers_v2
    args:
      - "sign"
      - "${artifact}@${digest}"
      - "--yes"
```

| Claim | Status | Source |
| --- | --- | --- |
| `docker_signs` exists; default `cmd: cosign`, default `args: ["sign", "--key=cosign.key", "${artifact}@${digest}", "--yes"]`, so keyless **must override `args`** (drop `--key`) | VERIFIED | https://github.com/goreleaser/goreleaser/blob/v2.20.3/www/content/customization/sign/docker_sign.md ; `internal/pipe/sign/sign_docker.go` `Default()` |
| `artifacts: ''` (default) signs images built by `dockers_v2`; `images`, `manifests`, `all` are the other values | VERIFIED | docker_sign.md ; `sign_docker.go` `Publish()` switch |
| It runs at the end, in the publish phase, because cosign changes the image in the registry; `${digest}` is the digest recorded by the build | VERIFIED | docker_sign.md note and footnotes ; `docker/v2/docker.go` stores `ExtraDigest` |
| One artifact exists per `image:tag`, so each tag gets its own `cosign sign` call on the same digest | VERIFIED (source: `ctx.Artifacts.Add` per image in `da.images`); signature storage effects UNVERIFIED | `internal/pipe/docker/v2/docker.go` lines 267-276 |
| Verify | VERIFIED command shape (`cosign verify` flags in `doc/cosign_verify.md` v3.1.4); not run | below |

```sh
cosign verify ghcr.io/mitto98/homelab-proxy-manager:1.2.3 \
  --certificate-identity 'https://github.com/mitto98/homelab-proxy-manager/.github/workflows/release.yml@refs/tags/v1.2.3' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
```

Tell users to verify by digest when they need rigour; a tag can move.

## 3. Testing the packages in CI

### 3.1 What the runners are

| Claim | Status | Source |
| --- | --- | --- |
| `ubuntu-latest` and `ubuntu-24.04` are Ubuntu 24.04.5, systemd 255.4, Docker 28.0.4 client and server, Podman 4.9.3, Buildah 1.33.7, rpm 4.18.2 preinstalled | VERIFIED | https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md (image 20261116.376.1) |
| `ubuntu-24.04-arm` (arm64) has the same Docker 28.0.4, Podman 4.9.3, rpm 4.18.2, systemd 255.4 | VERIFIED | https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Arm64-Readme.md (image 20261116.188.1) |
| README table lists `ubuntu-latest` = Ubuntu 24.04, plus new `ubuntu-26.04` and `ubuntu-26.04-arm` labels; announcement banner: "`ubuntu-latest` label will use Ubuntu 26.04 in November 2026"; Ubuntu 22.04 runner images are being deprecated (unsupported from April 17th) | VERIFIED | https://github.com/actions/runner-images/blob/main/README.md and the Ubuntu readmes' banner |
| Whether the Arm64 runner is free for private repos | not checked; public repos only is the usual case (UNVERIFIED) | |

The runners are Azure VMs with full systemd as PID 1 on the host, so **Ubuntu packages could be tested on the runner itself**, but you need Debian 12, Fedora and RHEL-9 userlands, which means containers.

### 3.2 systemd in a container: what is documented

| Claim | Status | Source |
| --- | --- | --- |
| systemd's container contract: pre-mount `/proc`, `/sys` (read-only), writable cgroup subtree, tmpfs `/run`; set `container=` ; stop with `SIGRTMIN+3` | VERIFIED | https://github.com/systemd/systemd/blob/v255/docs/CONTAINER_INTERFACE.md |
| Podman implements this: with `--systemd=always` (or when the command is `/sbin/init`) it mounts tmpfs on `/run`, `/run/lock`, `/tmp`, `/var/lib/journal`, mounts `/sys/fs/cgroup` writable on cgroup v2, and sets stop signal `SIGRTMIN+3` | VERIFIED | https://github.com/containers/podman/blob/v4.9.3/docs/source/markdown/options/systemd.md (podman 4.9.3 is the version on the runners) |
| Docker: `--cgroupns` accepts `host` or `private`; the default depends on the daemon (`default-cgroupns-mode`) | VERIFIED | https://github.com/docker/cli/blob/v28.0.4/docs/reference/commandline/container_run.md ; moby API 1.48 `CgroupnsMode` |
| jrei images (`jrei/systemd-debian` tags 10-13, `latest`=12; `jrei/systemd-ubuntu` tags 20.04, 22.04, 24.04, 26.04) document: `docker run -d --tmpfs /tmp --tmpfs /run --tmpfs /run/lock -v /sys/fs/cgroup:/sys/fs/cgroup:ro ...`, "or if it doesn't work" `--privileged -v /sys/fs/cgroup:/sys/fs/cgroup:ro` | VERIFIED (Docker Hub pages, updated 13 days before 2026-11-22) | https://hub.docker.com/r/jrei/systemd-debian , https://hub.docker.com/r/jrei/systemd-ubuntu . Community images, not distro-official. Architectures not stated on the page (UNVERIFIED for arm64; other tags of the same maintainer are widely multi-arch but I did not check) |
| `almalinux/9-init`: official AlmaLinux "init image ... Systemd is enabled as init system", archs amd64, arm64, ppc64le, s390x, 386 | VERIFIED | https://hub.docker.com/r/almalinux/9-init (AlmaLinux 9.9) |
| Rocky: `rockylinux/rockylinux` publishes `-ubi-init` tags (page shows `8-ubi-init`); a `9-ubi-init` tag was not seen | partly VERIFIED, `9-ubi-init` UNVERIFIED | https://hub.docker.com/r/rockylinux/rockylinux |
| Fedora: official `fedora` image (latest = 45, also 43, 44, 46/rawhide; amd64 and arm64v8) has **no documented init variant**; I found no first-party `fedora-init` image | VERIFIED for the page; absence of an init image is "not found" only | https://hub.docker.com/_/fedora . Plan: `fedora:latest` plus `dnf -y install systemd`, run `/usr/lib/systemd/systemd` as the container command (UNVERIFIED) |
| Actual `docker run` on a GitHub-hosted runner reliably brings up systemd | **UNVERIFIED**: no Docker daemon here and no runner access. The jrei pages themselves list `--privileged` as the fallback when the tmpfs flags do not work |

### 3.3 What I ran: systemd 255 as PID 1 in a namespace, no Docker

On this sandbox (Ubuntu 24.04, kernel 6.18, root, cgroup v1 layout) I started a real systemd as PID 1 with
`unshare --pid --fork --mount --mount-proc --propagation private` plus a script that mounts tmpfs on `/run` and `/tmp` and execs `/lib/systemd/systemd --system --unit=multi-user.target` with `container=docker`, then drove it with `nsenter -t <pid> -m -p`. `systemctl is-system-running` reported `running`. The filesystem is the real root, so `dpkg -i` modified the sandbox itself (cleaned afterwards).

Results for the packaged fixture (stub Panel and stub Traefik, real units and scripts):

| Step | Observed |
| --- | --- |
| `dpkg -i 1.0.0` | user created; `postinst` ran `daemon-reload`, enabled `homelab-proxy-manager.service` and `-traefik.path`; Panel was `inactive` until started |
| Start Panel | runs as `homelab-proxy-manager`; `/var/lib/homelab-proxy-manager` created by `StateDirectory=` with the right owner; `STATE_DIRECTORY` env set |
| Start Traefik before `traefik.yml` exists | stays inactive (`ConditionPathExists`) |
| Create `traefik.yml` as the service user | path unit fired, restart service ran, `try-restart` no-op (Traefik inactive) |
| Start Traefik, then (1) append to `traefik.yml`, (2) atomic `mv` of `traefik.env`, (3) atomic `mv` of `traefik.yml`, (4) touch an unrelated file | MainPID changed in (1), (2), (3) (436, 449, 462, 475); unchanged in (4). `starts.log` had 4 lines |
| `Unit=homelab-proxy-manager-traefik.service` on a throwaway path unit, then change its file | MainPID unchanged: no restart |
| `CapAmb` of Traefik | `0000000000000400` |
| Upgrade to 1.1.0 (same shipped env) with a local edit | edit kept, services still active |
| Upgrade to 1.2.0 (changed shipped env), `--force-confold` | edit kept, new default as `.dpkg-dist` |
| `dpkg -r` | all units stopped and disabled, unit files gone, `/etc/...env` and `/var/lib/...` kept, user kept |
| `dpkg -P` | `/etc/homelab-proxy-manager` removed, `/var/lib/homelab-proxy-manager` kept |

Not covered: Debian 12, Ubuntu 22.04 (systemd 249), Fedora, Rocky/Alma 9 userlands; arm64; rpm install under systemd (rpm was run on the same host but without systemd, in a separate `--dbpath`); SELinux contexts on RHEL/Fedora (the `StateDirectory` and `/usr/local/bin/traefik` execution under `systemd_unit_t` confinement could differ; UNVERIFIED); a real container.

### 3.4 Recommendation and minimal recipe

Ranking for this project (my judgement, from the sources above):

1. **Distro init images in Docker or Podman with `--privileged`**, on the two runner types. Maintained and first-party for Alma 9; community-maintained for Debian and Ubuntu (jrei); build-your-own one-liner for Fedora and Rocky. Least moving parts, but the flag set must be tried on a real runner once (UNVERIFIED).
2. **Podman `--systemd=always`** is the best documented systemd mode (tmpfs, cgroup v2 writable, stop signal) and ships on both runners (4.9.3); run rootful via `sudo podman`.
3. **systemd-nspawn** needs `systemd-container` plus a debootstrap/`dnf --installroot` rootfs per distro; more setup than a pulled image. Not recommended unless containers fail.
4. A full VM per distro is overkill; GitHub-hosted runners cannot boot other distros as the host.
5. My pid-namespace trick works only for the host distro (Ubuntu 24.04 here) and is shown for completeness.

Workflow shape (UNVERIFIED on runners, the script body is what I ran):

```yaml
test-packages:
  needs: snapshot            # goreleaser release --snapshot --clean --skip=publish,sign,docker
  strategy:
    fail-fast: false
    matrix:
      runner: [ubuntu-24.04, ubuntu-24.04-arm]
      target:
        - {name: debian12,   image: "jrei/systemd-debian:12",    pkg: deb}
        - {name: ubuntu2204, image: "jrei/systemd-ubuntu:22.04", pkg: deb}
        - {name: alma9,      image: "almalinux/9-init",          pkg: rpm}
        - {name: fedora,     image: "fedora:latest",             pkg: rpm, init: /usr/lib/systemd/systemd}
        - {name: rocky9,     image: "rockylinux/rockylinux:9-ubi-init", pkg: rpm}   # tag UNVERIFIED
  runs-on: ${{ matrix.runner }}
  steps:
    - uses: actions/download-artifact@<pin>
    - run: |
        docker run -d --name t --privileged --cgroupns=host \
          -v /sys/fs/cgroup:/sys/fs/cgroup:rw -v "$PWD/dist:/dist:ro" -v "$PWD/ci:/ci:ro" \
          --tmpfs /tmp --tmpfs /run --tmpfs /run/lock \
          ${{ matrix.target.image }} ${{ matrix.target.init }}
        docker exec t sh -c 'until [ "$(systemctl is-system-running 2>/dev/null)" = running ] || [ "$(systemctl is-system-running 2>/dev/null)" = degraded ]; do sleep 1; done'
        docker exec t sh /ci/test-package.sh ${{ matrix.target.pkg }}
```

Fedora's official image has no init, so it needs `dnf -y install systemd` first (an extra `docker exec`), and Debian/Ubuntu need `apt-get install -y ./x.deb` to pull `ca-certificates` etc. if any.

`ci/test-package.sh` body (the commands below are what I ran, in `dpkg` form; the rpm commands I ran were the `rpm -i/-U/-e` forms on the same host without systemd):

```sh
set -eu
# install v1 (deb)  : apt-get install -y /dist/homelab-proxy-manager_1.0.0_*.deb    (rpm: dnf install -y /dist/*.rpm)
getent passwd homelab-proxy-manager
systemctl start homelab-proxy-manager.service
systemctl is-active homelab-proxy-manager.service
# fake Traefik so ExecStartPre passes
install -m755 /ci/fake-traefik /usr/local/bin/traefik
runuser -u homelab-proxy-manager -- sh -c 'echo "entryPoints: {}" > /var/lib/homelab-proxy-manager/traefik.yml'
systemctl start homelab-proxy-manager-traefik.service
p1=$(systemctl show -p MainPID --value homelab-proxy-manager-traefik.service)
runuser -u homelab-proxy-manager -- sh -c 'echo "# edit" >> /var/lib/homelab-proxy-manager/traefik.yml'
sleep 2
p2=$(systemctl show -p MainPID --value homelab-proxy-manager-traefik.service)
[ "$p1" != "$p2" ]                                        # path unit restarted it
grep -q '^CapAmb:.*0000000000000400' /proc/"$p2"/status    # CAP_NET_BIND_SERVICE
# upgrade keeps a local edit
echo LOCAL_EDIT=1 >> /etc/homelab-proxy-manager/homelab-proxy-manager.env
# deb: dpkg --force-confold -i /dist/homelab-proxy-manager_1.1.0_*.deb ; rpm: rpm -U /dist/*1.1.0*.rpm
grep -q LOCAL_EDIT=1 /etc/homelab-proxy-manager/homelab-proxy-manager.env
# remove keeps state
# deb: dpkg -r homelab-proxy-manager ; rpm: rpm -e homelab-proxy-manager
test -f /var/lib/homelab-proxy-manager/traefik.yml
! systemctl is-active --quiet homelab-proxy-manager.service
```

The upgrade-keeps-edit test only proves the policy if the shipped default also **changes** between the two test versions; build a second snapshot version whose env file differs (as done here for 1.2.0), otherwise both formats trivially keep the file.

For the arm64 matrix leg, packages are built once on amd64 (pure Go, no CGO); arm64 is exercised by installing them on `ubuntu-24.04-arm`. GoReleaser snapshot builds both arches (VERIFIED live: `linux_arm64_v8.0` ELF and `aarch64` rpm).

### 3.5 Lint

Run live against the fixture packages:

- `lintian --pedantic *.deb` (lintian 2.117.0): `E: extended-description-is-empty`, `E: no-changelog`, `E: no-copyright-file`, `W: maintainer-script-calls-systemctl` (x3), `W: no-manual-page`, `W: non-standard-file-perm 0640 != 0644` (our `0640` env file). Missing in my fixture only: the real binary would add `statically-linked-binary`.
- `rpmlint` 2.5.0 (run with `/usr/bin/python3.12 /usr/bin/rpmlint`; the default `python3` here is 3.13 and lacks the rpm bindings): `E: non-readable ...env 640`, `E: no-signature`, `E: no-group-tag`, `E: no-changelogname-tag`, `E: no-binary` (arch-independent tooling warning for a stub), `W: no-url-tag`, `W: invalid-license MIT`, `W: dangerous-command-in-%preun rpm`, ...

Verdict: lintian is worth a non-blocking CI step (fix the description, add a `changelog` and `copyright` file via `contents` with `type: doc`/`license`, add `deb.lintian_overrides` for `statically-linked-binary` and `maintainer-script-calls-systemctl`, accept the 0640 warning), blocking only on `E:` after one pass. rpmlint is mostly noise against nfpm output (it is calibrated for Fedora spec-built packages with the Fedora config, e.g. signature and group-tag errors), so run it as informational or skip it; the install/upgrade/remove recipe in 3.4 catches the real failures. nfpm's own docs recommend lintian for deb (`tips.md`, "Debian packages"). Neither lint replaces the container test.

## 4. SQLite online backup with modernc.org/sqlite

| Claim | Status | Source |
| --- | --- | --- |
| Latest module is v1.62.0, requires Go 1.26.0, bundles SQLite 3.54.1 | VERIFIED | `go list -m`, `SELECT sqlite_version()` live; matches architecture section 3 ("Go 1.26 required") |
| The driver exposes the backup API: unexported `(*conn).NewBackup(dstURI string) (*Backup, error)` and `NewRestore(srcURI)`; exported `*Backup` with `Step(n int32) (bool, error)`, `Remaining()`, `PageCount()`, `Commit()` (returns the dest `driver.Conn`), `Finish()` | VERIFIED | module source of `modernc.org/sqlite@v1.62.0` from the Go module proxy (upstream repo is gitlab.com/cznic/sqlite): `backup.go`, `conn.go` lines 1423-1445; `CHANGELOG.md` line 291 |
| It is reached through `database/sql` with `conn.Raw(func(driverConn any) error { bk, err := driverConn.(interface{ NewBackup(string) (*sqlite.Backup, error) }).NewBackup(dst) ... })` | VERIFIED live (compiled and ran) | scratch Go program (not committed) |
| `VACUUM INTO 'file'` creates a new compacted copy; the target "must not previously exist, or else it must be an empty file, or the VACUUM INTO command will fail"; it "is transactional in the sense that the generated output database is a consistent snapshot" | VERIFIED | https://www.sqlite.org/lang_vacuum.html (via Firecrawl directQuote) |
| Backup API: each `sqlite3_backup_step()` takes a shared lock for the duration of that call only; "If the source database is modified by an external process or via a database connection other than the one being used by the backup operation, then the backup will be automatically restarted by the next call to sqlite3_backup_step()" | VERIFIED | https://www.sqlite.org/c3ref/backup_finish.html (via Firecrawl directQuote) |

Live test (WAL database of 41 MB, 30k rows of 1 KiB, pool of 4 connections, a second pool connection inserting one row every ~0.2 ms during each backup, run in this sandbox):

| Method | Result |
| --- | --- |
| `db.Exec("VACUUM INTO ?", path)` | 161 ms; writer kept inserting (136 rows), max writer latency under 1 ms, 0 errors; copy `integrity_check` = `ok`, row count equals a consistent point in time, copy's `journal_mode` = `delete` (a plain single file, no `-wal`) |
| Backup API, `Step(-1)` once | 131 ms; writer unaffected (110 rows, 0 errors); copy integrity `ok`; copy keeps `journal_mode` = `wal`, still a single self-contained file after `Finish()` |
| Backup API, `Step(50)` in a loop with 1 ms sleep, writer on another connection | **never finished**: gave up after 5001 steps, "Remaining" grew back 4990 times (automatic restart, as documented) |

Safety summary:

- Both `VACUUM INTO` and `Step(-1)` are safe while the Panel runs, given WAL mode (readers do not block the writer; confirmed above). In rollback-journal mode the shared lock would block writers for the length of the copy; I did not test that mode, UNVERIFIED. The Panel must set `journal_mode=WAL` (not stated in architecture section 5-6).
- **Incremental stepping is unsafe in practice** under an active writer on another connection. If you must step, run the backup on the same connection the writes go through (documented to update the copy), or use `Step(-1)`.
- Prefer `VACUUM INTO`: public SQL, no reliance on the driver's unexported method, output is compacted and standalone. Costs: needs free disk for a full copy, fails if the file exists (use a temp name then rename inside the tar step), and cannot run inside a transaction (UNVERIFIED, not tested; the docs read for this task do not say).
- The backup is only `panel.db`; `secret.key` and `acme.json` are plain files, copied separately. A restore must put the DB in place with the Panel stopped (as architecture section 10 already says); restoring a WAL-mode copy is fine because it is a complete file.

## 5. Impact on architecture section 10 (for the issue)

| Section 10 says | Finding |
| --- | --- |
| units: panel, traefik, path unit | A fourth, oneshot restart unit is required (1.6). |
| "Traefik binary path ... set in an environment file" | systemd cannot take it as the program; use an `sh -c 'exec "$TRAEFIK_BIN" ...'` wrapper (1.6). |
| "Removing the package keeps `/var/lib/homelab-proxy-manager`" | True only if the package does not own the directory; use `StateDirectory=` (1.5). |
| Docker: "multi-stage build: build the Panel" | `dockers_v2` supplies prebuilt per-platform binaries; do not compile in the Dockerfile (1.7). |
| tags `latest`, `X.Y.Z`, `X.Y` | Template tags that empty out on prereleases; legacy `skip_push: auto` is the wrong tool (1.7). |
| "SQLite is copied with its online backup mechanism" | Either mechanism works under WAL; prefer `VACUUM INTO` or a single `Step(-1)` (4). |
| Testing | Add a package-install CI matrix (3.4); pin `ubuntu-24.04` because `ubuntu-latest` is moving to 26.04. |

## Sources

- GoReleaser v2.20.3: `www/content/customization/{sign/sign.md, sign/docker_sign.md, package/nfpm.md, package/dockers_v2.md, package/docker.md, general/{hooks,templates}.md, builds/{hooks.md,builders/go.md}, ci/actions.md, publish/scm/_index.md}`, `internal/pipeline/pipeline.go`, `internal/pipe/{git,sign,docker,docker/v2}/`, `go.mod` at https://github.com/goreleaser/goreleaser/tree/v2.20.3
- nfpm v2.49.0: `www/content/docs/{configuration,tips}.md`, `deb/deb.go`, `rpm/spec.go`, `files/files.go` at https://github.com/goreleaser/nfpm/tree/v2.49.0
- cosign v3.1.4 and v2.6.1: `doc/cosign_{sign-blob,verify-blob,sign,verify}.md`, `cmd/cosign/cli/options/{signblob,sign,verify}.go`, `CHANGELOG.md` at https://github.com/sigstore/cosign ; https://github.com/sigstore/cosign-installer/tree/v4.1.3 ; https://github.com/sigstore/fulcio/blob/main/docs/oidc.md
- systemd v255: `man/systemd.service.xml`, `man/systemd.path.xml`, `docs/CONTAINER_INTERFACE.md`, `NEWS` at https://github.com/systemd/systemd/tree/v255
- Podman v4.9.3 `docs/source/markdown/options/systemd.md`; docker/cli v28.0.4 `container_run.md`; moby API 1.48
- GitHub runner images: https://github.com/actions/runner-images (README, `Ubuntu2404-Readme.md`, `Ubuntu2404-Arm64-Readme.md`, `Ubuntu2604-Readme.md`)
- Image pages: https://hub.docker.com/r/jrei/systemd-debian , https://hub.docker.com/r/jrei/systemd-ubuntu , https://hub.docker.com/r/almalinux/9-init , https://hub.docker.com/r/rockylinux/rockylinux , https://hub.docker.com/_/fedora
- SQLite: https://www.sqlite.org/lang_vacuum.html , https://www.sqlite.org/c3ref/backup_finish.html ; modernc.org/sqlite v1.62.0 `backup.go`, `conn.go`, `CHANGELOG.md`
