# Testing real ACME issuance in CI with Pebble

Researched 2026-11-20. Primary sources only (Traefik v3.7.19, lego v5.6.0 and Pebble v2.10.3 source and docs at the pinned tags). Items marked UNVERIFIED could not be confirmed. Feeds issue #8; builds on `docs/research/traefik-ci-harness.md` (branch `research/traefik-ci-harness`), which pins Traefik v3.7.19 and downloads the checksum-verified release binary.

## Summary

- **It works with plain processes, no Docker, no root, no hosts-file edit.** On 2026-11-20 I ran the official `traefik` v3.7.19 binary, a Pebble v2.10.3 build and `pebble-challtestsrv` as ordinary processes on loopback and high ports. Traefik obtained a certificate from Pebble over **HTTP-01** and another over **TLS-ALPN-01**, wrote both into `acme.json`, and served the HTTP-01 certificate on its TLS entrypoint. VERIFIED by live run (section 7).
- **`PEBBLE_VA_HTTPPORT` and `PEBBLE_VA_TLSPORT` do not exist.** The validation ports are the `httpPort` and `tlsPort` keys of Pebble's JSON config (defaults 5002 and 5001). VERIFIED (grep of the whole v2.10.3 tree finds neither name). The task brief's env var names are wrong; do not use them.
- **Pebble connects to `<domain>:<httpPort>`, not port 80.** So Traefik's `httpChallenge.entryPoint` must listen on Pebble's `httpPort`, and for TLS-ALPN-01 some TLS entrypoint must listen on `tlsPort`. Traefik has no setting for a different challenge port; the entrypoint address is the only lever.
- **Trust the Pebble CA with Traefik's own `acme.caCertificates` key** (present in v3.7.19, simpler and more explicit than env vars). `LEGO_CA_CERTIFICATES` works as a fallback only when `caCertificates` and `caServerName` are both unset.
- **Name resolution**: with no `-dnsserver`, Pebble uses Go's system resolver (so `/etc/hosts` works, but not wildcards). With `-dnsserver` pointing at `pebble-challtestsrv`, every name resolves to one configurable IP, which is what CI wants. Recommended.
- **Binaries**: Pebble publishes tarballs for `pebble` and `pebble-challtestsrv` as GitHub release assets, but **no checksums file and no signatures**. Pin sha256 values in the workflow. `go install` also works. Pebble's config and the minica root cert are not in the tarballs; take them from the tag's source tree.
- **acme.json** top-level key is the resolver name; certificates are base64 PEM bundles; `notAfter` is read by decoding the leaf. Not a public contract (section 5).
- The Pebble binary used in the live run was **built from the v2.10.3 source** with `go build` (reports version `dev`), not the release tarball. The release tarballs were downloaded and listed but not executed. See Access notes.

## Access notes

- Reachable and used: `git clone` of `letsencrypt/pebble` at `v2.10.3`, `traefik/traefik` at `v3.7.19`, `go-acme/lego` at `v5.6.0`; GitHub release asset downloads for Pebble and Traefik.
- Not reachable: the GitHub releases HTML page (403 from the proxy) and the GitHub REST API for `letsencrypt/pebble` (403, repository not enabled for this session). So the release's publication time, and any GitHub-computed asset digests, are UNVERIFIED. `doc.traefik.io` was not used; Traefik docs below are the repo copies at `v3.7.19`.
- Running binaries was **not** refused here: Pebble, challtestsrv and Traefik all ran. The sandbox did reject some long compound shell commands, which I split.
- Latest Pebble tag is `v2.10.3` (`git ls-remote --tags`, 2026-11-20); Traefik v3.7.19 vendors lego **v5.6.0** (`go.mod` line 26 at the tag).

## 1. Pointing Traefik v3.7.19 at Pebble

| Claim | Status | Source |
| --- | --- | --- |
| `certificatesResolvers.<n>.acme.caServer` sets the ACME directory URL; default is Let's Encrypt production | VERIFIED | https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/tls/certificate-resolvers/acme.md (option table) and `pkg/provider/acme/provider.go` (`CAServer`, line 47; `SetDefaults` line 72) |
| Pebble's directory is `https://<host>:14000/dir` (ACME port 14000, management port 15000 in the shipped config) | VERIFIED | https://github.com/letsencrypt/pebble/blob/v2.10.3/test/config/pebble-config.json and README "Usage" |
| Traefik v3.7.19 has native YAML keys `acme.caCertificates` (list of PEM file paths), `acme.caSystemCertPool` (bool) and `acme.caServerName` | VERIFIED | acme.md option table (`opt-acme-caCertificates`, `-caSystemCertPool`, `-caServerName`); struct tags in `pkg/provider/acme/provider.go` lines 61-63 |
| Precedence: if `caCertificates` is non-empty **or** `caServerName` is set, those keys build the cert pool and the `LEGO_*` env vars are **not read** | VERIFIED | `createClientTLSConfig`, `pkg/provider/acme/provider.go` lines 413-443 |
| Otherwise Traefik falls back to `LEGO_CA_CERTIFICATES` (path list split on `os.PathListSeparator`, i.e. `:` on Linux), `LEGO_CA_SYSTEM_CERT_POOL` (bool) and `LEGO_CA_SERVER_NAME`; with none of them set the system pool is used | VERIFIED (source) | same function, lines 430-445. The env path was **not** exercised in the live run (I used `caCertificates`) |
| lego defines the same three env var names | VERIFIED | https://github.com/go-acme/lego/blob/v5.6.0/lego/client_config.go lines 24-34 |
| `LEGO_CA_SYSTEM_CERT_POOL=true` means "also trust the system pool"; without it only the listed files are trusted | VERIFIED (source) | `lego.CreateCertPool(paths, useSystemCertPool)` call in `createClientTLSConfig`; semantics of `CreateCertPool` not read in detail |
| The Pebble ACME API cert (`test/certs/localhost/cert.pem`) has SANs `localhost`, `pebble`, `127.0.0.1`, expires 2027-10-03, and chains to `test/certs/pebble.minica.pem` (CN `minica root ca 5345e6`, valid to 2125) | VERIFIED | `openssl x509` on the files in the v2.10.3 tree. Use `https://localhost:14000/dir` as `caServer` so the name matches |
| Pebble's README says to trust `pebble.minica.pem` only per client, never system-wide (its private key is public in the repo) | VERIFIED | https://github.com/letsencrypt/pebble/blob/v2.10.3/README.md, section on the test CA ("IMPORTANT: Do not add ... system-wide trust") |
| `httpChallenge.entryPoint` names a Traefik entrypoint; Traefik adds an internal router `acme-http` for `/.well-known/acme-challenge/` on that entrypoint | VERIFIED | `pkg/provider/traefik/internal.go` lines 108-134; `pkg/server/service/internalhandler.go` line 54 |
| `tlsChallenge: {}` enables TLS-ALPN-01; no entrypoint is named. Traefik answers `acme-tls/1` ALPN handshakes on any TCP entrypoint that has TLS routers | VERIFIED (source) | `pkg/server/router/tcp/router.go` line 195 (`slices.Contains(hello.protos, tlsalpn01.ACMETLS1Protocol)`); live run succeeded on a `websecure` entrypoint bound to Pebble's `tlsPort` |
| A resolver with none of dns/http/tls challenge configured fails with "ACME challenge not specified" | VERIFIED | `provider.go` line ~337 |
| Docs say HTTP-01 needs port 80 and TLS-ALPN-01 needs port 443 ("reachable by Let's Encrypt") | VERIFIED | acme.md lines 155 and 183. That is a Let's Encrypt constraint, not a Traefik one; Pebble validates on its configured ports |

Minimal static config used in the live run (resolver names `http` and `alpn`):

```yaml
entryPoints:
  web:       {address: "127.0.0.1:5002"}   # Pebble httpPort
  websecure: {address: "127.0.0.1:5001"}   # Pebble tlsPort
certificatesResolvers:
  http:
    acme:
      email: ci@example.test
      caServer: https://localhost:14000/dir
      storage: /path/acme.json
      keyType: EC256
      caCertificates: [/path/pebble.minica.pem]
      httpChallenge: {entryPoint: web}
  alpn:
    acme:
      email: ci@example.test
      caServer: https://localhost:14000/dir
      storage: /path/acme-alpn.json
      keyType: EC256
      caCertificates: [/path/pebble.minica.pem]
      tlsChallenge: {}
```

Notes:

- Default `keyType` is `RSA4096` (`SetDefaults`); the account key is always RSA 4096 (`NewAccount` in `account.go`), generated at first use. Setting `keyType: EC256` speeds the certificate key, not the account key. UNVERIFIED how much time this saves on a runner.
- The storage file must be mode `0600` or Traefik refuses it ("permissions ... are too open"); a missing file is created as `0600`. `pkg/provider/acme/local_store_unix.go`. VERIFIED.
- Traefik's `email` is required in the docs table; Pebble does not check it.

## 2. How Pebble validates HTTP-01, and running without Docker

| Claim | Status | Source |
| --- | --- | --- |
| Pebble fetches `http://<identifier>:<httpPort>/.well-known/acme-challenge/<token>` | VERIFIED | `fetchHTTP`, https://github.com/letsencrypt/pebble/blob/v2.10.3/va/va.go (builds the URL with `net.JoinHostPort(identifier, httpPort)`). Live log: `Attempting to validate w/ HTTP: http://http01.test.example:5002/.well-known/acme-challenge/...` |
| `httpPort` / `tlsPort` come from the JSON config (`pebble.httpPort`, `pebble.tlsPort`); defaults in the shipped config are 5002 and 5001, "unprivileged high ports" so Pebble needs no root | VERIFIED | `cmd/pebble/main.go` (config struct), `test/config/pebble-config.json`, README "Default validation ports" |
| There is **no** `PEBBLE_VA_HTTPPORT` or `PEBBLE_VA_TLSPORT` env var | VERIFIED | grep over the v2.10.3 tree excluding `vendor/`: no match. The only `PEBBLE_VA_*` vars are `NOSLEEP`, `SLEEPTIME`, `ALWAYS_VALID` (`va/va.go` lines 52-75) |
| The target IP is resolved by `resolveIP`: Go's `net.DefaultResolver.LookupHost` when no `-dnsserver` is given (so `/etc/hosts` and system DNS apply); otherwise it asks the `-dnsserver` address for AAAA then A | VERIFIED | `va/va.go` lines 862-900 |
| With `-dnsserver`, an IP-literal identifier is returned untouched | VERIFIED | same function (`net.ParseIP` branch) |
| Only the **first** resolved address is dialled | VERIFIED | `fetchHTTP` `DialContext` uses `addrs[0]`; TLS-ALPN path also `addrs[0]` |
| `pebble-challtestsrv` is a DNS mock plus challenge responders; flags: `-dnsserver` (default `:8053`), `-management` (`:8055`), `-defaultIPv4` (`127.0.0.1`), `-defaultIPv6` (`::1`), `-http01` (`:5002`), `-https01` (`:5003`), `-tlsalpn01` (`:5001`), `-doh` (`:8443`); an empty string disables a listener | VERIFIED | `pebble-challtestsrv -help` from the v2.10.3 build; README of `cmd/pebble-challtestsrv` |
| Pebble's `-dnsserver` accepts `host:port` (e.g. `127.0.0.1:8053`) | VERIFIED | `cmd/pebble/main.go` flag help; README lines ~193-205 |
| Pebble and challtestsrv run as plain processes on a Linux host with no Docker | VERIFIED (live) | Both ran here as ordinary processes bound to `127.0.0.1`. The shipped `docker-compose.yml` is a convenience, not a requirement. UNVERIFIED on an actual GitHub-hosted runner (not available to me), but nothing used needs more than loopback TCP/UDP above port 1024 |

Recommended CI shape (what the live run did):

1. `pebble-challtestsrv -http01 "" -https01 "" -tlsalpn01 "" -doh "" -dnsserver 127.0.0.1:8053 -management 127.0.0.1:8055 -defaultIPv4 127.0.0.1 -defaultIPv6 ""`. Disabling its three challenge listeners frees ports 5001 and 5002 for Traefik; we only want its DNS mock. `-defaultIPv6 ""` stops AAAA answers pointing at `::1` (the upstream compose file does the same).
2. `PEBBLE_VA_NOSLEEP=1 pebble -config pebble.json -dnsserver 127.0.0.1:8053`, with a config copied from `test/config/pebble-config.json` but `listenAddress`/`managementListenAddress` set to `127.0.0.1:...` and absolute paths to the cert and key. Without `PEBBLE_VA_NOSLEEP=1` Pebble sleeps a random time before each validation (README "Validation Authority Sleep"); with `-dnsserver` unset it uses the system resolver.
3. Any hostnames under any suffix then resolve to `127.0.0.1` (live run: `http01.test.example`, `alpn01.test.example`). No hosts-file edit and no `sudo`.
4. Wait for readiness by polling `https://localhost:14000/dir` with `--cacert pebble.minica.pem` (what I did) before starting Traefik. A fixed `sleep` is racy; UNVERIFIED how long the runner needs.

Pebble defaults worth knowing (all from `pebble` startup log and README): it rejects 5% of good nonces and reuses authorizations 50% of the time; lego retries on `badNonce`, and both behaviours are intended. Set `PEBBLE_WFE_NONCEREJECT=0` and `PEBBLE_AUTHZREUSE=0` for deterministic tests (README sections on those vars; the env var names are VERIFIED, the effect on Traefik runs UNVERIFIED).

## 3. TLS-ALPN-01

- **Yes, testable the same way, with `tlsPort` as the knob.** Pebble dials `<identifier>:<tlsPort>` with SNI set to the identifier and ALPN `acme-tls/1`, skipping certificate verification, then checks that the leaf has exactly one SAN equal to the identifier and a critical `acmeValidationV1` extension whose value is the SHA-256 of the key authorization. VERIFIED, `validateTLSALPN01` in `va/va.go` lines 412-520.
- There is no `PEBBLE_VA_TLSPORT`; set `"tlsPort"` in the JSON config (see section 2).
- Traefik needs a TCP entrypoint with TLS routers listening on that port; the `acme-tls/1` handshake is intercepted in the TCP router before normal routing (section 1). The ALPN resolver does not need a dedicated entrypoint.
- Live result: certificate for `alpn01.test.example` was issued and stored. VERIFIED. Caveat: the HTTP-01 and TLS-ALPN-01 runs here were in one Traefik process on the same `websecure` entrypoint; the router in the dynamic config for the ALPN domain referenced `certResolver: alpn`.
- Keep challtestsrv's own `-tlsalpn01` and `-http01` listeners disabled when Traefik is the responder: they default to the same ports 5001 and 5002 and would collide.

## 4. Getting pinned Pebble binaries

| Claim | Status | Source |
| --- | --- | --- |
| Latest release tag is `v2.10.3`; recent tags `v2.9.0`, `v2.10.0`, `v2.10.1`, `v2.10.2`, `v2.10.3` | VERIFIED | `git ls-remote --tags https://github.com/letsencrypt/pebble` on 2026-11-20 |
| Each release gets prebuilt binaries as GitHub release assets, produced by the `go-build` and `create-release` jobs: for each app (`pebble`, `pebble-challtestsrv`) and each of darwin/linux/windows x amd64/arm64, a `<app>-<os>-<arch>.tar.gz` and `.zip` | VERIFIED | https://github.com/letsencrypt/pebble/blob/v2.10.3/.github/workflows/release.yml (build matrix; `tar czf ${artifact}.tar.gz`, `zip -r ${artifact}.zip`, `gh release upload`) |
| Download works: `https://github.com/letsencrypt/pebble/releases/download/v2.10.3/pebble-linux-amd64.tar.gz` and `.../pebble-challtestsrv-linux-amd64.tar.gz` | VERIFIED | downloaded 2026-11-20 (HTTP 200) |
| Tarball layout: `pebble-linux-amd64/linux/amd64/pebble` and `pebble-challtestsrv-linux-amd64/linux/amd64/pebble-challtestsrv` | VERIFIED | `tar tzf` |
| sha256 of `pebble-linux-amd64.tar.gz` = `7e33cf3e426b67e9e647c44e8cc91683caa8ab331f7a99e5e27fcee291c4c7e2`; of `pebble-challtestsrv-linux-amd64.tar.gz` = `00b2c5ef525cd8ad56cda898a7cac6169a10b0fd3f2a7456fe0cec92e7b36cb6` | VERIFIED as what the download produced on 2026-11-20 | `sha256sum`. These are **trust-on-first-use** values: nothing upstream publishes them |
| No checksums file and no signature is published: the upload step uploads only the `.tar.gz` and `.zip`; `.../checksums.txt` returns 404 | VERIFIED | release.yml `Upload release files` step; HTTP 404 on `checksums.txt` |
| Release binaries are static (`CGO_ENABLED=0`), built `-trimpath` with `-s -w`, version string `<tag> (<sha>)` via `-ldflags -X main.version` | VERIFIED | release.yml `go-build` env |
| Container images are also published to `ghcr.io/letsencrypt/pebble` and `.../pebble-challtestsrv` | VERIFIED | README "Prebuilt Docker Images"; release.yml `docker-build` |
| `go install` path: README says `git clone` then `go install ./cmd/pebble`; the module path is `github.com/letsencrypt/pebble/v2`, so `go install github.com/letsencrypt/pebble/v2/cmd/pebble@v2.10.3` is the module form | module path VERIFIED (`go.mod` line 1); the `@v2.10.3` one-liner was **not** run (UNVERIFIED) |
| `go.mod` requires Go 1.25.0; the repo has a `vendor/` directory, so a clone builds offline with the standard toolchain | VERIFIED | `go.mod`; `go build ./cmd/pebble ./cmd/pebble-challtestsrv` succeeded here |

Recommendation (a suggestion, not sourced):

- Download the release tarballs and verify them against sha256 values committed in the repo. Because upstream publishes no checksums, the first pin is trust-on-first-use over TLS from `github.com`; record the values from two independent fetches if that matters. This mirrors the approach in `docs/research/traefik-ci-harness.md`.
- Also fetch **the same tag's source tree** (a shallow `git clone --branch v2.10.3`, or `curl` the raw files) for `test/certs/pebble.minica.pem`, `test/certs/localhost/{cert,key}.pem` and `test/config/pebble-config.json`: the release tarballs contain only the binary. Commit these three small test fixtures into the repo instead (they are public test material; I did not check the licence terms, UNVERIFIED), which removes the clone and makes the pin one file set.
- The alternative of `go install` needs the Go toolchain on PATH (present on runners that run the Go tests anyway) and compiles locally (build time not measured); the Go module proxy checksum database then gives integrity for free. Either is fine; the tarball route avoids compile time and an extra proxy dependency.

## 5. acme.json structure and reading `notAfter`

Written by `LocalStore` (`pkg/provider/acme/local_store.go`, `store.go`, `provider.go` v3.7.19) with `json.MarshalIndent`; file mode `0600`. The shape below was confirmed by reading the file produced in the live run (key and token lengths replaced by string lengths):

```json
{
  "<resolverName>": {
    "Account": {
      "Email": "ci@example.test",
      "Registration": {
        "body": { "status": "valid", "contact": ["mailto:ci@example.test"], "orders": "https://localhost:14000/list-orderz/..." },
        "uri": "https://localhost:14000/my-account/..."
      },
      "PrivateKey": "<base64 PKCS#1 RSA key>"
    },
    "Certificates": [
      {
        "domain": { "main": "http01.test.example", "sans": ["..."] },
        "certificate": "<base64 of the PEM bundle: leaf then chain>",
        "key": "<base64 of the PEM private key>",
        "Store": "default"
      }
    ]
  }
}
```

| Claim | Status | Source |
| --- | --- | --- |
| Top level is a map keyed by **resolver name** (one file can hold several resolvers; I used one file per resolver in the live run) | VERIFIED | `map[string]*StoredData` in `local_store.go`; live file shows key `http` |
| `StoredData` has `Account` and `Certificates` (capitalised, no json tags) | VERIFIED | `store.go`; `local_store_test.go` expected JSON; live file |
| Each certificate entry has `domain{main,sans}`, `certificate`, `key` (lowercase tags) and `Store` (capitalised, untagged). The embedded `Certificate` struct is flattened into the entry | VERIFIED | `provider.go` lines 81-93 (`CertAndStore` embeds `Certificate`); live file. `sans` is `omitempty` and absent for a single-name cert |
| `certificate` and `key` are `[]byte`, so Go's `encoding/json` stores them as base64 strings; the decoded `certificate` is a PEM bundle | VERIFIED | `Certificate` struct types; live decode through `base64 -d` then `openssl x509` succeeded |
| `notAfter` is not stored as a field: decode `certificate` from base64, parse the first PEM block (the leaf), read `NotAfter` | VERIFIED | live run: `notAfter=Feb 18 18:30:52 2027 GMT` for a cert issued 2026-11-20 18:30:52Z. Traefik itself does the same: `getX509Certificate` then `crt.NotAfter` in `renewCertificates` (`provider.go` line ~912) |
| Pebble's default certificate validity is 7776000 s (90 days); profile `validityPeriod` in the Pebble config changes it | VERIFIED | `test/config/pebble-config.json`; startup log "certificate validity period of 7776000 seconds"; matches the notAfter above |
| Pebble's default profile omits the Common Name: the issued leaf has an empty subject, name only in SAN | VERIFIED (live, `subject=` empty; startup log "promote CN=false") | Assert on SANs, not on CN |
| Issuer in the live run was `CN=Pebble Intermediate CA <hex>`; root and intermediate are regenerated on **every Pebble start** | VERIFIED | startup log; README "CA Root and Intermediate Certificates" (roots at `https://localhost:15000/roots/0`) |

Not a public contract:

- Traefik's docs do not document `acme.json` fields. The only references are source and tests at the tag; a future version may change key capitalisation, flatten differently or move storage. Tests should isolate the read in one helper, and prefer **black-box checks first**: connect to the TLS entrypoint with the SNI and inspect the served leaf (`openssl s_client`, or Go `tls.Dial`), which I did and which showed issuer `Pebble Intermediate CA` and the expected SAN. Use `acme.json` only for `notAfter` and for asserting that the certificate persisted.
- Traefik writes `acme.json` asynchronously from a goroutine (`saveDataChan`, `listenSaveAction`); in the live run the file existed by the time I read it, but poll with a timeout instead of reading once. VERIFIED as asynchronous in source; the latency was not measured.
- Renewal: `renewCertificates` runs once at provider start and then on a ticker (24 h for default durations, renew when within 720 h of expiry; logged in the live run). To test renewal in CI, issue a certificate from a Pebble profile with a short `validityPeriod`, then **restart Traefik**: the start-up pass renews anything inside the window. VERIFIED in source (`Provide` calls `p.renewCertificates` before starting the ticker) but **not exercised live**.

## 6. Claims from the brief that did not hold

| Brief | Finding |
| --- | --- |
| `PEBBLE_VA_HTTPPORT` / `PEBBLE_VA_TLSPORT` | Do not exist in v2.10.3. Use `httpPort` / `tlsPort` in the JSON config. |
| "needs `LEGO_CA_CERTIFICATES`" | Only the fallback. v3.7.19 has `acme.caCertificates`; when set the env vars are ignored. |
| "host resolution via challtestsrv DNS mock or `/etc/hosts`" | Both work (section 2). Note `/etc/hosts` cannot hold wildcards and needs root to edit; the DNS mock needs neither. |

## 7. Live run (2026-11-20, sandbox)

Processes, all as ordinary children on loopback: `pebble-challtestsrv` (DNS only, `127.0.0.1:8053`), `pebble` v2.10.3 built from source (`127.0.0.1:14000`, validating on 5002 and 5001), `traefik` v3.7.19 (release tarball, sha256 `97534a68...f89be` verified, entrypoints `web` on `127.0.0.1:5002`, `websecure` on `127.0.0.1:5001`, `traefik` API on `127.0.0.1:8099`). Dynamic config: two routers on `websecure`, one per resolver, with rules `Host(http01.test.example)` and `Host(alpn01.test.example)`.

Observed:

- Traefik log (lego v5.6.0): "Obtaining bundled SAN certificate ... Validations succeeded; requesting certificates ... Waiting for certificates" for both domains; no ACME errors.
- Pebble log: `Attempting to validate w/ HTTP: http://http01.test.example:5002/...`, a validation task for `alpn01.test.example`, then `Issued certificate serial ...`.
- Two `acme.json` files (mode `0600`) each held one certificate; leaf issuer `Pebble Intermediate CA`, `notAfter` 90 days out.
- `openssl s_client -connect 127.0.0.1:5001 -servername http01.test.example` returned the Pebble-issued leaf with SAN `DNS:http01.test.example`.
- All processes were killed afterwards.

Not tested: the `LEGO_CA_CERTIFICATES` env path, `/etc/hosts` resolution, renewal, the release-tarball Pebble binary, a real GitHub-hosted runner, and wildcard or DNS-01 flows (DNS-01 against challtestsrv's `/set-txt` management API is a separate question).

## Sources

- Traefik v3.7.19: `pkg/provider/acme/provider.go`, `local_store.go`, `local_store_unix.go`, `store.go`, `account.go`, `pkg/server/router/tcp/router.go`, `pkg/provider/traefik/internal.go`, `docs/content/reference/install-configuration/tls/certificate-resolvers/acme.md` at https://github.com/traefik/traefik/tree/v3.7.19
- lego v5.6.0: https://github.com/go-acme/lego/blob/v5.6.0/lego/client_config.go
- Pebble v2.10.3: `README.md`, `va/va.go`, `cmd/pebble/main.go`, `test/config/pebble-config.json`, `test/certs/`, `.github/workflows/release.yml`, `docker-compose.yml`, `cmd/pebble-challtestsrv/` at https://github.com/letsencrypt/pebble/tree/v2.10.3
