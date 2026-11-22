# Testing DNS-01 and wildcard certificates in CI with Pebble and challtestsrv

Researched 2026-11-22. Primary sources only (Traefik v3.7.19, lego v5.6.0 and Pebble v2.10.3 source and docs at the pinned tags), plus live runs. Items marked UNVERIFIED could not be confirmed. Feeds issue #9 and answers the open question in `docs/architecture.md` section 7.5 ("exact `tls.domains` shape for wildcard hosts"). Builds on `docs/research/pebble-acme-ci.md` (branch `research/pebble-acme-ci`), which proved Traefik, Pebble and `pebble-challtestsrv` run as plain loopback processes for HTTP-01 and TLS-ALPN-01.

## Summary

- **A full DNS-01 wildcard issuance runs in CI with plain processes, no real DNS provider, no Docker, no root.** On 2026-11-22 Traefik v3.7.19 (release binary), Pebble v2.10.3 and `pebble-challtestsrv` v2.10.3 (both **release tarballs**, not source builds) issued one certificate covering `*.example.test` plus `example.test` over DNS-01. VERIFIED by live run (section 6).
- **Simplest provider: lego `exec`.** `dnsChallenge.provider: exec` with env `EXEC_PATH=<script>`; the script gets `present <fqdn> <value>` / `cleanup <fqdn> <value>` and does one `curl` to challtestsrv's management API. `httpreq` also works (needs a small HTTP shim) and was run live. lego `manual` is unusable (reads Enter from stdin, and fails the zone lookup against challtestsrv). VERIFIED live for all three.
- **challtestsrv's endpoint is `/set-txt`, not `/add-txt`.** There is no `add-txt` route in v2.10.3. `/set-txt` **appends** a value (several values per host accumulate), `/clear-txt` removes all values for the host. The host needs a trailing dot. VERIFIED (source and live).
- **Traefik's propagation check must be adjusted.** With defaults, lego's authoritative-nameserver check sends an `SOA` query that challtestsrv answers `NOTIMP`, so the check never passes and Traefik waits out its 60 s timeout. Two settings work: `resolvers: ["127.0.0.1:8053"]` plus `propagation.disableANSChecks: true` (real TXT check against the mock, issued in about 6 s), or `propagation.disableChecks: true` with `delayBeforeChecks: 1s` (no DNS from Traefik at all, no `resolvers` needed). Both VERIFIED live.
- **Set `EXEC_SEQUENCE_INTERVAL=1` with `exec`.** The `exec` provider is "sequential" and waits `EXEC_SEQUENCE_INTERVAL` (default 60 s) between the apex and wildcard authorizations. VERIFIED live: log line `sequence: wait. interval=60000`. `httpreq` is not sequential and has no such wait.
- **Pebble issues the wildcard.** Wildcard authorizations get only DNS challenges; the TXT name is `_acme-challenge.example.test` for both `*.example.test` and `example.test`. VERIFIED.
- **Answer to the section 7.5 question.** Router-level YAML that worked, producing one `acme.json` entry `{domain: {main: "*.example.test", sans: ["example.test"]}}`:

  ```yaml
  http:
    routers:
      wild-host:
        rule: "Host(`*.example.test`)"
        entryPoints: [websecure]
        service: svc
        tls:
          certResolver: dns
          domains:
            - main: "*.example.test"
              sans: ["example.test"]
  ```

  `tls.domains` is a key of the **router's `tls` block**, not a top-level `tls:` section. Without it Traefik derives `main: "*.example.test"` from the `Host` rule and the apex is **not** covered (VERIFIED live). One entry with the apex as a SAN gives one certificate and one issuance, so the architecture's "apex as a separate domain" is not required.
- **`Host(`*.example.test`)` is valid in Traefik v3.7.19** and matches exactly one label: `foo.example.test` matched; `example.test` (404) and `a.b.example.test` did not. VERIFIED live (section 5).
- **Release tarball binaries are not executable.** The Pebble and challtestsrv `.tar.gz` entries have mode `0644`; `chmod +x` after extracting. VERIFIED. The earlier note had only listed them.

## Access notes

- Reachable and used: `git clone` of `letsencrypt/pebble` at `v2.10.3`, `go-acme/lego` at `v5.6.0`, `traefik/traefik` at `v3.7.19`; GitHub release asset downloads. Sha256 of the three tarballs matched the earlier note (Pebble `7e33cf3e...c7e2`, challtestsrv `00b2c5ef...6cb6`, Traefik `97534a68...f89be`).
- Not used: `doc.traefik.io` and `go-acme.github.io/lego`; Traefik docs below are the repo copies at `v3.7.19`. The runner was this sandbox, not a GitHub-hosted runner (UNVERIFIED there).
- Sandbox quirk: `HTTPS_PROXY` is set here, so `curl` to loopback needs `--noproxy '*'`. Not relevant to CI runners without a proxy.
- The sandbox's system resolver is `8.8.8.8`, so any run without `resolvers` and with checks on fails here for the reason in section 3 (`test.` is `NXDOMAIN` publicly). On a runner it fails the same way.

## 1. Which Traefik DNS provider drives challtestsrv

| Claim | Status | Source |
| --- | --- | --- |
| Traefik takes `certificatesResolvers.<n>.acme.dnsChallenge.provider` and resolves it by name through lego (`dns.NewDNSChallengeProviderByName`) | VERIFIED | https://github.com/traefik/traefik/blob/v3.7.19/pkg/provider/acme/provider.go lines 342-355 |
| Provider configuration comes from **environment variables of the Traefik process**, not YAML | VERIFIED (source, live) | same lines; `EXEC_PATH` set in the process environment worked in every run |
| challtestsrv exposes `POST /set-txt` (`{"host":"_acme-challenge.x.","value":"v"}`) and `POST /clear-txt` (`{"host":...}`) on the management address (default `:8055`); host must end in `.` | VERIFIED | https://github.com/letsencrypt/pebble/blob/v2.10.3/cmd/pebble-challtestsrv/main.go lines 129-130; `cmd/pebble-challtestsrv/README.md` "DNS-01" ("a period character is required at the end"). Live log: `Added TXT response for Host "_acme-challenge.example.test."` |
| There is **no** `add-txt` route (the brief's name) | VERIFIED | grep of the v2.10.3 tree: only `set-txt`, `clear-txt`, `add-a` and friends |
| `set-txt` appends: `AddDNSTXTRecord` adds to the host's value list; `DeleteDNSTXTRecord` removes all values of the host | VERIFIED | https://github.com/letsencrypt/pebble/blob/v2.10.3/vendor/github.com/letsencrypt/challtestsrv/dns.go lines 75-92. Live: two `set-txt` calls for the same host both succeeded, Pebble validated both |
| lego `exec`: env `EXEC_PATH` (required), optional `EXEC_MODE=RAW`, `EXEC_PROPAGATION_TIMEOUT`, `EXEC_POLLING_INTERVAL`, `EXEC_SEQUENCE_INTERVAL`. Default mode runs `<program> present <EffectiveFQDN> <value>` and `<program> cleanup <EffectiveFQDN> <value>`; output is logged; a non-zero exit fails the challenge | VERIFIED | https://github.com/go-acme/lego/blob/v5.6.0/providers/dns/exec/exec.go (`NewDNSProvider`, `run`). `EffectiveFQDN` already has the trailing dot and the `_acme-challenge.` prefix; for a wildcard it is for the base name (`_acme-challenge.example.test.`) |
| `exec` is a sequential provider: it solves authorizations one at a time and sleeps `EXEC_SEQUENCE_INTERVAL` (default `dns01.DefaultPropagationTimeout` = 60 s) between them | VERIFIED | `exec.go` `Sequential()`; `challenge/resolver/prober.go` `sequentialSolve`; `dns_challenge.go` line 24. Live: `sequence: wait. interval=60000` without the env var, `interval=1000` with `EXEC_SEQUENCE_INTERVAL=1` |
| lego `httpreq`: env `HTTPREQ_ENDPOINT` (required), optional `HTTPREQ_MODE=RAW`, `HTTPREQ_USERNAME`/`PASSWORD`; POSTs JSON `{"fqdn","value"}` to `<endpoint>/present` and `<endpoint>/cleanup`; any 2xx is success | VERIFIED | https://github.com/go-acme/lego/blob/v5.6.0/providers/dns/httpreq/httpreq.go. Not sequential, so both TXT values were present together and the wildcard and apex were validated in parallel |
| lego `manual` prints the record and blocks reading Enter from stdin; it also calls `FindZoneByFqdn` first | VERIFIED | https://github.com/go-acme/lego/blob/v5.6.0/providers/dns/manual/manual.go `Present`. Live: failed with `manual: could not find zone ... SOA ... code=NOTIMP`; even past that, Traefik has no terminal to answer. Not usable |

**Pick `exec`.** It needs no server, only a ten-line script. Script used (`dns01-exec.sh`):

```bash
#!/bin/bash
# lego exec provider: $1 = present|cleanup, $2 = FQDN (trailing dot), $3 = TXT value
M=http://127.0.0.1:8055
case "$1" in
  present) curl -fsS -d "{\"host\":\"$2\",\"value\":\"$3\"}" $M/set-txt ;;
  cleanup) curl -fsS -d "{\"host\":\"$2\"}" $M/clear-txt ;;
esac
```

Environment for the Traefik process: `EXEC_PATH=/abs/path/dns01-exec.sh` and `EXEC_SEQUENCE_INTERVAL=1`.

`httpreq` alternative: `HTTPREQ_ENDPOINT=http://127.0.0.1:9100` plus a 20-line Python shim that maps `/present` to `set-txt` and `/cleanup` to `clear-txt` (it ran live and issued the same certificate in about 4 s). Worth it only if the harness already has an HTTP helper process; it also lets a test assert on what Traefik sent.

Caveat on `cleanup` with `httpreq`/`exec`: `clear-txt` removes every value for the host. Fine here, because lego cleans up after the solves it started; do not run two Traefik issuances against one challtestsrv concurrently.

## 2. How Pebble does the DNS-01 lookup

| Claim | Status | Source |
| --- | --- | --- |
| Pebble builds `_acme-challenge.<identifier>` and fetches TXT records, accepting if any record equals base64url(SHA-256(key authorization)) | VERIFIED | https://github.com/letsencrypt/pebble/blob/v2.10.3/va/va.go `validateDNS01` lines 336-370 |
| With `-dnsserver host:port`, `getTXTEntry` sends a TXT query to that server; without it, Go's system resolver is used | VERIFIED | `va.go` `getTXTEntry` lines 827-860; startup log `Using custom DNS resolver for ACME challenges: 127.0.0.1:8053` (`va.go` line 140) |
| For a wildcard order, Pebble strips `*.` and validates `_acme-challenge.<base>`; wildcard identifiers get DNS challenges only | VERIFIED | `wfe/wfe.go` lines 1650-1660 (authorization creation), 2314-2318 and 2669-2675 (wildcard stripped, `Wildcard` flag set). Live: Pebble log shows validation tasks for identifier `example.test` twice (one per authorization) |
| So `*.example.test` and `example.test` in one order need **two TXT values at the same name** at different times | VERIFIED | live `challtestsrv` log: two `Added TXT response ... "_acme-challenge.example.test."` with different values |

Pebble flags used: `PEBBLE_VA_NOSLEEP=1 PEBBLE_WFE_NONCEREJECT=0 PEBBLE_AUTHZREUSE=0 pebble -config pebble.json -dnsserver 127.0.0.1:8053`. No `httpPort`/`tlsPort` use here (they only matter for HTTP-01 and TLS-ALPN-01), but keep them valid. challtestsrv: `-http01 "" -https01 "" -tlsalpn01 "" -doh "" -dnsserver 127.0.0.1:8053 -management 127.0.0.1:8055 -defaultIPv4 127.0.0.1 -defaultIPv6 ""`. The Pebble config is the shipped `test/config/pebble-config.json` with loopback addresses and absolute cert paths (as in the earlier note).

## 3. Does Traefik's own propagation check need adjusting? Yes

Traefik passes `dnsChallenge.resolvers` to lego as the recursive nameservers, and maps `propagation.*` to lego options (`provider.go` lines 345-365 at v3.7.19). lego then (a) finds the zone with `SOA` queries, (b) asks the authoritative nameservers and (c) asks the recursive resolvers for the TXT value, before telling the CA to validate.

| Setup | Result | Evidence |
| --- | --- | --- |
| `resolvers: ["127.0.0.1:8053"]`, default propagation | **FAILS.** Loops every 2 s with `authoritative nameservers: could not find zone ... question='_acme-challenge.example.test. IN SOA', code=NOTIMP`, until the 60 s timeout | live run A. challtestsrv's handler answers only CNAME, TXT, A, AAAA and CAA questions; any other type, `SOA` included, gets rcode `NOTIMP` (`vendor/github.com/letsencrypt/challtestsrv/dns.go` lines 460-475; its fixed SOA goes only in the authority section) |
| no `resolvers`, default propagation | **FAILS** here with `could not find the start of authority for ... question='test. IN SOA', code=NXDOMAIN` (the system resolver, `8.8.8.8`, knows no `.test`) | live run D |
| `resolvers: ["127.0.0.1:8053"]` + `propagation: {disableANSChecks: true}` | **PASSES.** Recursive TXT check against challtestsrv, cert in about 6 s | live run B |
| `propagation: {disableChecks: true, delayBeforeChecks: 1s}`, no `resolvers` | **PASSES.** Log: `the active propagation check is disabled, waiting for the propagation instead. wait=1000`. Traefik makes no DNS queries for the zone | live run C |
| lego `manual` with `resolvers` and `disableChecks` | FAILS in `Present` (zone lookup `NOTIMP`) | live run M |

Option docs: `acme.dnsChallenge.resolvers`, `propagation.disableChecks`, `.delayBeforeChecks`, `.disableANSChecks`, `.requireAllRNS` in https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/install-configuration/tls/certificate-resolvers/acme.md lines 97-101.

Recommendation: prefer `resolvers` + `disableANSChecks: true` (B). It still proves that the TXT record is visible over DNS, and does not depend on a sleep. Use `disableChecks` + `delayBeforeChecks` (C) if a runner blocks UDP on loopback. Not tested: `requireAllRNS`, TCP-only resolvers, IPv6 loopback.

## 4. Wildcard issuance and `acme.json`

Live result, resolver named `dns`, one file per run (mode `0600`):

```json
{
  "dns": {
    "Account": { "...": "..." },
    "Certificates": [
      {
        "domain": { "main": "*.example.test", "sans": ["example.test"] },
        "certificate": "<base64 PEM bundle>",
        "key": "<base64 PEM key>",
        "Store": "default"
      }
    ]
  }
}
```

| Claim | Status | Source |
| --- | --- | --- |
| One order for domains `[*.example.test, example.test]` gives **one** certificate entry with `main` = first domain and `sans` = the rest | VERIFIED | live `acme.json`; Traefik log `Obtaining bundled SAN certificate. domains="*.example.test, example.test"` |
| Leaf: empty subject, `X509v3 Subject Alternative Name: critical` with `DNS:*.example.test, DNS:example.test`, issuer `Pebble Intermediate CA <hex>`, 90 days validity | VERIFIED | `openssl x509` on the decoded `certificate` |
| The served certificate for SNI `foo.example.test` and for `example.test` is that same leaf; it verifies against Pebble's root (`https://127.0.0.1:15000/roots/0`) | VERIFIED | `openssl s_client -servername ...` (`Verification: OK`) |
| With **no** `tls.domains`, Traefik derives the domain from the `Host` rule: log `Domains need ACME certificates generation for domains "*.example.test"`, entry `{"main": "*.example.test"}` with **no `sans` key**; the apex is then not covered | VERIFIED | live run N; `ParseDomains` in https://github.com/traefik/traefik/blob/v3.7.19/pkg/muxer/http/mux.go line 179 |
| `tls.domains` is a property of a **router's `tls`** block (`certResolver`, `options`, `domains: [{main, sans}]`) | VERIFIED | https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/routing-configuration/http/routing/router.md (YAML example, option table line 111). The earlier-mentioned top-level `tls:` section of the dynamic file holds certificates and options, not ACME domains; not tested |
| Traefik does not re-issue for a domain set already in `acme.json`: after issuance, log `No ACME certificate generation required for domains` | VERIFIED | live logs, runs B and N |
| `*.*.example.test` is rejected by Traefik: `ACME does not allow '*.*' wildcard domain` | VERIFIED (source), not run | `provider.go` line 1053 |

How the mapping works: Traefik takes the router's `tls.domains` when present (else the `Host` rule) to decide what to request, and stores the result under `domain.main`/`domain.sans`. When a TLS handshake arrives, Traefik's certificate store picks the stored certificate by SNI against the SANs (wildcard-aware), so SNI `foo.example.test` and `example.test` both get this leaf. VERIFIED by behaviour (live handshakes); the selection code was not read.

Implication for `docs/architecture.md` section 7.5: render `tls.domains: [{main: "*.example.com", sans: ["example.com"]}]` when the host also wants the apex, and `[{main: "*.example.com"}]` otherwise (or omit `tls.domains` and rely on the rule). A separate apex entry is not needed and would cost a second issuance. Whether the apex entry can be a *second* `tls.domains` item on the same router: UNVERIFIED.

## 5. `Host(`*.example.test`)` in Traefik v3

| Claim | Status | Source |
| --- | --- | --- |
| In v3 (v3 rule syntax, the default) `Host` supports a single-level wildcard prefix; docs prefer it over `HostRegexp` ("allows attaching a TLS option and is more efficient"). A wildcard matches exactly one label, not the apex, not multiple labels | VERIFIED | https://github.com/traefik/traefik/blob/v3.7.19/docs/content/reference/routing-configuration/http/routing/rules-and-priority.md lines 55-62 |
| Implementation: for a pattern starting with `*`, the request host's first label is replaced by `*` and compared case-insensitively | VERIFIED | https://github.com/traefik/traefik/blob/v3.7.19/pkg/muxer/muxer.go `DomainMatchHostExpression` line 23; `host` in `pkg/muxer/http/matcher.go` line 73 (a bare `*` is a catch-all) |
| `HostRegexp(`^.+\.example\.test$`)` also works and matches any depth | VERIFIED | live run (below) |

Live matching (run B; two routers on `websecure`, `wild-host` with ``Host(`*.example.test`)`` and priority left at default, `wild-regexp` with ``HostRegexp(`^.+\.example\.test$`)`` at priority 1; the backends return different bodies; Traefik access log `RouterName` column):

| Request SNI/Host | Response | Router that matched |
| --- | --- | --- |
| `foo.example.test` | `BACKEND-host` | `wild-host@file` (the `Host` wildcard rule) |
| `example.test` | `404 page not found` | none (`-`) |
| `a.b.example.test` | `BACKEND-regexp` | `wild-regexp@file` |

So the `Host` wildcard matched one label and did **not** match the apex or two labels. The default priority of a rule is its length, so the `Host` router wins over a priority-1 regexp for single-label names. For a CI assertion, request with `curl --resolve foo.example.test:8443:127.0.0.1 -k https://foo.example.test:8443/` and read the access log's router column, or `GET /api/http/routers` on the API entrypoint.

## 6. Live run (2026-11-22, sandbox)

Processes as ordinary children on loopback: `pebble-challtestsrv` v2.10.3 release tarball (DNS `127.0.0.1:8053`, management `127.0.0.1:8055`), `pebble` v2.10.3 release tarball (`Pebble version: v2.10.3 (5caaab38...)`, ACME `127.0.0.1:14000`, `-dnsserver 127.0.0.1:8053`), `traefik` v3.7.19 release tarball (entrypoints `web` 8081, `websecure` 8443, API 8099), and two `python3 -m http.server` backends. Traefik static YAML (the one that worked, variant B; paths shortened):

```yaml
log: {level: DEBUG}
accessLog: {filePath: /work/access.log}
api: {insecure: true}
entryPoints:
  traefik:   {address: "127.0.0.1:8099"}
  web:       {address: "127.0.0.1:8081"}
  websecure: {address: "127.0.0.1:8443"}
providers:
  file: {filename: /work/dynamic.yml}
certificatesResolvers:
  dns:
    acme:
      email: ci@example.test
      caServer: https://localhost:14000/dir
      storage: /work/acme.json            # mode 0600
      keyType: EC256
      caCertificates: [/work/pebble.minica.pem]
      dnsChallenge:
        provider: exec
        resolvers: ["127.0.0.1:8053"]
        propagation: {disableANSChecks: true}
```

Environment: `EXEC_PATH=/work/dns01-exec.sh EXEC_SEQUENCE_INTERVAL=1`. The `traefik` entrypoint must be moved off its default `:8080` if anything else uses it (it collided with a sandbox listener; the API listener is separate from `web`).

Dynamic file: the `wild-host` router from the Summary plus `wild-regexp` and two `loadBalancer` services.

| Run | Config | Outcome |
| --- | --- | --- |
| A | `resolvers` + default propagation | fails, `SOA ... NOTIMP`, retried until timeout |
| B | `resolvers` + `disableANSChecks`, `exec` | **issued** about 6 s after `Obtaining bundled SAN certificate` (apex first, wildcard second, 1 s sequence wait) |
| B (no env) | same without `EXEC_SEQUENCE_INTERVAL` | `sequence: wait interval=60000` (stopped after 12 s; not waited out) |
| C | `disableChecks: true`, `delayBeforeChecks: 1s`, no `resolvers` | **issued** about 7 s |
| D | no `resolvers`, default propagation | fails, `NXDOMAIN` for `test.` |
| H | `httpreq` + shim, B's propagation | **issued** about 4 s; shim saw two `/present` and two `/cleanup` POSTs |
| M | `manual` | fails, `could not find zone ... NOTIMP` |
| N | B without `tls.domains` | **issued** for `{main: "*.example.test"}` only |

Pebble log for B: `Using custom DNS resolver for ACME challenges: 127.0.0.1:8053`, two `Starting 3 validations`, `authz ... set VALID`, `Issued certificate serial ...`. challtestsrv log: `Added TXT response`/`Removed TXT response` pairs for `_acme-challenge.example.test.`, once per authorization. All processes were killed afterwards.

Not tested: a GitHub-hosted runner; `requireAllRNS`; renewal of a wildcard; two `tls.domains` items on one router; CNAME-delegated `_acme-challenge` (lego `GetChallengeInfo` follows CNAMEs on the resolvers; not exercised); `exec` with `EXEC_MODE=RAW`; the top-level `tls.certificates` section.

## 7. Claims from the brief that did not hold

| Brief | Finding |
| --- | --- |
| challtestsrv `add-txt` / `clear-txt` | The routes are `set-txt` and `clear-txt`. `set-txt` appends. |
| "`propagation.disableChecks` / `delayBeforeChecks` / `resolvers` must be adjusted" | Correct in effect, but the minimal change is `resolvers` + `disableANSChecks`; the root cause is `SOA` `NOTIMP`. |
| Pebble release binaries run after extraction | They need `chmod +x` (tar mode `0644`). |

## Sources

- Traefik v3.7.19: `pkg/provider/acme/provider.go`, `pkg/muxer/muxer.go`, `pkg/muxer/http/matcher.go`, `pkg/muxer/http/mux.go`, `pkg/server/aggregator.go`, `docs/content/reference/install-configuration/tls/certificate-resolvers/acme.md`, `docs/content/reference/routing-configuration/http/routing/{router,rules-and-priority}.md` at https://github.com/traefik/traefik/tree/v3.7.19
- lego v5.6.0: `providers/dns/{exec,httpreq,manual}/*.go`, `challenge/dns01/dns_challenge.go`, `challenge/resolver/prober.go` at https://github.com/go-acme/lego/tree/v5.6.0
- Pebble v2.10.3: `va/va.go`, `wfe/wfe.go`, `cmd/pebble-challtestsrv/{main.go,README.md}`, `vendor/github.com/letsencrypt/challtestsrv/dns.go`, `test/config/pebble-config.json` at https://github.com/letsencrypt/pebble/tree/v2.10.3
