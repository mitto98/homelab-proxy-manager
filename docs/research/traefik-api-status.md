# Traefik API as a host status source

Researched 2026-11-22 against **Traefik v3.7.19** (release tarball `traefik_v3.7.19_linux_amd64.tar.gz`, sha256 checked against `traefik_v3.7.19_checksums.txt`: OK). Answers issue #11. Everything marked **live** was run as plain loopback processes in this sandbox (Traefik, plus Pebble v2.10.3 and `pebble-challtestsrv` for ACME), as in `research/traefik-ci-harness` and `research/pebble-acme-ci`. **source** means read in the Traefik tag `v3.7.19` (`https://github.com/traefik/traefik/blob/v3.7.19/<path>`). Anything neither is marked UNVERIFIED.

Static config used: entrypoints `traefik` = `127.0.0.1:18080`, `web` = `127.0.0.1:18081`, `websecure` = `127.0.0.1:18443`; `api: {insecure: true, dashboard: false}`; `ping: {}`; `providers.file.directory` (watch on); resolvers `good` (Pebble) and `down` (CA at `127.0.0.1:1`). Dynamic files were dropped into the watched directory one case at a time.

## Summary

- The Panel can show per-host health from `GET /api/http/routers?per_page=<big>` plus `/api/http/services`. Router `status` is `enabled`, `disabled` (error) or `warning`; the reason is in `error` (list of strings). Traefik never says `failed`; the Panel maps `disabled` to failed.
- **"enabled" is not "working".** Cases where the API says enabled but traffic fails are listed in section 7. The important ones: dead upstream without healthCheck (router enabled, service enabled, server `UP`, clients get 502); dead upstream with healthCheck (router still enabled, server `DOWN`, clients get 503); unknown `certResolver` (router enabled, clients get the self-signed `TRAEFIK DEFAULT CERT`); ACME issuance failure (router enabled, no API trace at all).
- **A malformed file in the watched directory is invisible to the API** and, while it stays there, blocks *every* later change to the directory. On restart it leaves zero file-provider routers. The Panel must validate before writing (it already plans to) and must not rely on the API to notice.
- **Default page size is 100**, so a bare `/api/http/routers` silently truncates. Always pass `per_page`.

## 1. Which endpoints work (`api.insecure: true`, `dashboard: false`, entrypoint `traefik` on loopback)

All verified live, 2026-11-22. Source for the option semantics: `docs/content/reference/install-configuration/api-dashboard.md` and `pkg/config/static/static_config.go` (see the earlier harness note for the `:8080` default if the `traefik` entrypoint is not declared).

| Request on `127.0.0.1:18080` | Result |
| --- | --- |
| `/api/overview` | 200 `application/json` |
| `/api/version` | 200 `application/json; charset=UTF-8` |
| `/api/http/routers` | 200 |
| `/api/http/services` | 200 |
| `/api/http/middlewares` | 200 (`[]` when none defined) |
| `/api/rawdata` | 200 (268 KB with 627 routers) |
| `/api/entrypoints` | 200, shows `"address":"127.0.0.1:18080"` for `traefik` |
| `/api/certificates` | 200 (exists in v3.7.19; `[]` when no certificate is loaded) |
| `/api/http/routers/host-1-https@file` and `.../host-1-https%40file` | 200, single router object |
| `/ping` | 200 body `OK` (only because `ping: {}` is set; it shares the `traefik` entrypoint) |
| `/dashboard/` and `/` | 404 `404 page not found` (dashboard off) |

So `dashboard: false` removes only the UI; the whole JSON API stays. `/ping` is not needed (use `/api/version` as readiness).

Listings include Traefik's own `@internal` resources (`api@internal`, `ping@internal`, `acme-http@internal`, `noop@internal`). Filter on `provider == "file"` or the `@file` suffix.

## 2. Resource names and mapping to Panel host IDs

Verified live. A resource defined as `host-1-https` in the file provider is exposed as **`host-1-https@file`** (`<name>@<provider>`), in the `name` field and in references (`usedBy`, router `service`). Router JSON also carries `"provider":"file"`. The router's own `service` field shows the name as written in the file (`"service":"host-1"`), while `usedBy` on the service uses the qualified router names (`["host-1-http@file","host-1-https@file"]`). Qualified service names appear in error text (`the service "nosuchsvc@file" does not exist`).

Mapping advice:
- Generate deterministic names from the Panel ID, e.g. `host-<id>-http`, `host-<id>-https`, `host-<id>` for the service, `host-<id>-<mw>` for middlewares; parse with a strict regexp on the `@file`-stripped name. Ignore anything that does not match (internal and foreign resources).
- **Names can be rewritten by Traefik**: when two TLS routers on the same entrypoint and host have different `tls.options` (or a TLS-options router has no domain in its rule), Traefik emits a copy named `<entrypoint>-conflicted-<name>@file`, and the original name disappears from the API (verified live: only `websecure-conflicted-warn-tls1@file` was listed, no `warn-tls1@file`). Source: `pkg/server/aggregator.go` lines 187 and 242. The Panel regexp should accept an optional `<entrypoint>-conflicted-` prefix, and a missing expected router should be shown as "not loaded / unknown", not silently "active". The Panel's generated config should not emit per-router `tls.options` that can conflict.
- Prefer one request per poll and index by name in memory, not one request per host.

## 3. What Traefik reports, case by case (all live, v3.7.19)

Router and service objects (`GET /api/http/routers`, `/api/http/services`):

| # | Case | Router `status` / `error` | Service | Client traffic (curl via `web`/`websecure`) |
| --- | --- | --- | --- | --- |
| a | healthy router + service (upstream up) | `enabled`, no `error` field (null in jq) | `enabled`, `"serverStatus":{"http://127.0.0.1:19999":"UP"}` | 200 |
| b | middleware does not exist | `disabled`, `["middleware \"nope@file\" does not exist"]` | enabled | 404 (router not installed) |
| c | service does not exist | `disabled`, `["the service \"nosuchsvc@file\" does not exist"]` | n/a | 404 |
| d | `tls.certResolver` names an undefined resolver | **`enabled`, no error** | enabled | HTTPS answers 200 with certificate `CN=TRAEFIK DEFAULT CERT` (self-signed). Error only in logs, see below |
| e | upstream `127.0.0.1:1` refuses, no healthCheck | `enabled`, no error | `enabled`, `"serverStatus":{"http://127.0.0.1:1":"UP"}` | **502** |
| f | same, `healthCheck: {path: /health, interval: 2s, timeout: 1s}` | `enabled`, no error | `enabled`, `"serverStatus":{"http://127.0.0.1:1":"DOWN"}` (flipped within ~2 s of load) | **503** |
| g | two routers, same rule | both `enabled`, no error, no warning | enabled | one wins; see note |
| h | invalid rule syntax | `disabled`, see below | n/a | 404 |
| i | malformed YAML file | API unchanged, see below | | previous config keeps serving |
| j | ACME issuance fails | `enabled`, no error, see below | | default cert |

Exact JSON observed:

```json
{"name":"case-b@file","status":"disabled","error":["middleware \"nope@file\" does not exist"]}
{"name":"case-c@file","status":"disabled","error":["the service \"nosuchsvc@file\" does not exist"]}
{"name":"case-d@file","status":"enabled","error":null,"using":["websecure"]}
{"name":"case-e@file","status":"enabled","error":null,"using":["web"]}
{"name":"svc-dead@file","status":"enabled","error":null,"serverStatus":{"http://127.0.0.1:1":"UP"},"usedBy":["case-e@file"]}
{"name":"svc-dead-hc@file","status":"enabled","error":null,"serverStatus":{"http://127.0.0.1:1":"DOWN"},"usedBy":["case-f@file"]}
{"name":"case-h@file","status":"disabled","error":["error while parsing rule Host(`h.test.example`) &&& PathPrefix(: parsing rule Host(`h.test.example`) &&& PathPrefix(: 1:39: expected ')', found 'EOF'"]}
{"name":"case-h2@file","status":"disabled","error":["error while parsing rule Foo(`x`): parsing rule Foo(`x`): unsupported function: Foo"]}
{"name":"case-ep@file","status":"disabled","error":["entryPoint \"nosuchentrypoint\" doesn't exist","no valid entryPoint for this router"]}
{"name":"mw-bad-router@file","status":"disabled","error":["cannot parse CIDRs [not-a-cidr]: parsing CIDR trusted IPs <nil>: invalid CIDR address: not-a-cidr"]}
```

Notes (`error` is omitted/null when empty, so treat null as no errors; `serverStatus` is null on routers and on services without servers):

- **Disabled routers are not installed**, so requests fall through to a 404 (checked for b, c, h, middleware-error).
- **Bad middleware config disables both the middleware and every router that uses it.** The middleware object (`/api/http/middlewares`) is `{"ipAllowList":{"sourceRange":["not-a-cidr"]},"error":["cannot parse CIDRs [not-a-cidr]: ..."],"status":"disabled","usedBy":["mw-bad-router@file"],"name":"badallow@file","provider":"file","type":"ipallowlist"}` and the router carries the same error text.
- **(d) unknown certResolver**: log line only: `ERR .../cmd/traefik/traefik.go:398 > Router uses a nonexistent certificate resolver certificateResolver=undefined-resolver routerName=case-d@file`. Source: `cmd/traefik/traefik.go` line 398. The API does not carry it. The Panel should check resolver names against its own static config before writing, and could additionally verify via `/api/certificates` (below).
- **(e) vs (f)**: without a healthCheck Traefik never marks a server DOWN; `serverStatus` shows the configured server as `UP` even though connections are refused. With a healthCheck the server is `DOWN` and the router/service `status` stay `enabled`; the failure reason is only in logs (`WRN ... Health check failed. error="checking HTTP health: HTTP request failed: Get \"http://127.0.0.1:1/health\": dial tcp 127.0.0.1:1: connect: connection refused" serviceName=svc-dead-hc@file`), not in the API. `serverStatus` is a map keyed by server URL with values `UP` or `DOWN`. If every server of a service is DOWN, requests get 503.
- **(g) duplicate rules**: no warning or error. Both routers stay `enabled`. Which one serves is decided by priority (rule length by default), then provider precedence; for equal priority and provider the order is not defined (source: `routes.Less` in `pkg/muxer/http/mux.go` has no further tie-break; routes come from map iteration). Live: the first measurement answered via `dup-a` (502, dead upstream) and every later reload answered via `dup-b` (200), so the winner changed between reloads. The Panel must itself reject duplicate host rules; the API will not tell it.
- **(h)** both a syntax error and an unsupported matcher produce `disabled` with the parser message.

### (i) Malformed YAML in the watched directory (live)

- Writing a new broken file `z-bad.yml`: previous config kept, routers list unchanged (20 `@file` routers before and after; overview totals identical: `{"routers":{"total":23,"warnings":4,"errors":6},...}`), host `ok.test.example` still 200. Log only: `ERR .../pkg/provider/file/file.go:204 > Error occurred during watcher callback error="collecting file configs: .../dynamic/z-bad.yml: yaml: line 4: found unexpected end of stream" providerName=file`.
- Corrupting an existing file (`a-base.yml`, which held the working hosts): same; `host-1-http@file` and `host-1-https@file` stay `enabled` in the API and keep serving from the old config. Nothing in the API reveals the corruption.
- **While the bad file stays in the directory, valid changes to other files are not applied** (the provider rebuilds the whole directory and aborts). Live: added valid `y-new.yml` next to `z-bad.yml`: router absent, `Host: new.test.example` returned 404; after deleting `z-bad.yml` the router appeared (`new-after-bad@file`, `enabled`). Corollary: the API can show "active" for a host whose latest edit was never applied (stale config), the worst misleading case for file-provider setups.
- **After a Traefik restart with a bad file present** the file provider yields nothing: only `acme-http@internal`, `api@internal`, `ping@internal` remained, overview `{"total":3,...}`, `ok.test.example` returned 404. Log: `ERR .../pkg/provider/file/file.go:114 > Error while building configuration (for the first time) error="collecting file configs: ..."`.
- Recommendation: the Panel writes only config it has validated, writes to a temp name that the loader ignores (not `.yml/.yaml/.toml`, as the earlier note says), and after each apply polls until its expected router names appear with the new content (e.g. compare `rule`) instead of trusting presence of old names.

### (j) ACME failure (live, Pebble)

Three router cases using HTTP-01, `/api/http/routers` after issuance attempts:

```json
{"name":"acme-ok@file","status":"enabled","error":null,"tls":{"options":"default","certResolver":"good"}}
{"name":"acme-fail-validation@file","status":"enabled","error":null,"tls":{"options":"default","certResolver":"good"}}
{"name":"acme-ca-down@file","status":"enabled","error":null,"tls":{"options":"default","certResolver":"down"}}
```

- Both failures (domain with no DNS record, so Pebble's validation fails; CA unreachable) leave the router `enabled` with no error. They exist **only in logs**: `ERR .../pkg/provider/acme/provider.go:499 > Unable to obtain ACME certificate for domains error="unable to generate a certificate for the domains [acme-fail.test.example]: resolver: one or more domains had a problem: [acme-fail.test.example: invalid authorization: acme: error: 400 :: urn:ietf:params:acme:error:connection :: ... could not resolve URL ...]" ... providerName=good.acme routerName=acme-fail-validation@file` and `error="cannot get ACME client get directory at 'https://127.0.0.1:1/dir': ... connection refused" ... providerName=down.acme routerName=acme-ca-down@file`.
- The failing hosts serve `CN=TRAEFIK DEFAULT CERT`; the successful host serves the Pebble certificate (SAN `acme-ok.test.example`).
- **Indirect signal that is in the API**: `GET /api/certificates` lists only certificates that were actually loaded. After the run it contained exactly one entry, for the host that succeeded:
  `[{"name":"5bcafe91...","sans":["acme-ok.test.example"],"notAfter":"2026-11-28T12:37:29Z","notBefore":"2026-11-22T12:37:30Z","serialNumber":"4921473709819943031","commonName":"","issuerCN":"Pebble Intermediate CA 184ce4","version":"v3","keyType":"RSA","keySize":4096,"signatureAlgorithm":"ECDSA-SHA256","certFingerprint":"5bcafe91...","publicKeyFingerprint":"88fa87ce...","status":"warning"}]`
  and `/api/overview` `certificates` was `{"total":1,"warnings":1,"errors":0}`. So the Panel can derive "certificate pending or failed" as: router has `tls.certResolver` and no certificate in `/api/certificates` has a matching SAN (also wildcard SANs). It cannot get the reason, nor distinguish "still pending" from "failed" (issuance runs in the background with a 30 s `certificateTimeout` default as seen in the logged static config), so show "no certificate yet" and point at Traefik logs. The certificate `status` here is `warning` only because less than 30 days remain (Pebble's short-lived profile, 6 days); `getCertificateStatus` in `pkg/api/certificate.go` returns `expired` below 0, `warning` under 30 days, else `enabled`. The `/api/certificates` field set was seen in this one response only.
- Failures persist: on every restart the log repeats the error; no retry timer was observed beyond that (UNVERIFIED how Traefik schedules retries).

Other cases seen live while building the matrix (useful for the UI's error vocabulary):

- Router with `entryPoints: [web, nosuchentrypoint]`: `{"name":"warn-ep@file","status":"warning","error":["entryPoint \"nosuchentrypoint\" doesn't exist"],"using":["web"]}`: it still serves on `web`.

## 4. `warning`, and service status

Router `warning` exists and is set only for non-critical problems (source `pkg/config/runtime/runtime_http.go`, `AddError(err, false)`). Live reproductions:

- Some but not all listed entryPoints missing: `warning`, router still routes on the valid ones (above).
- TLS options conflict, two routers same entrypoint+host with different `tls.options`: both routers renamed `websecure-conflicted-warn-tls1@file` / `...tls2@file`, `status:"warning"`, `error:["router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead"]`. Traefik falls back to default TLS options.
- TLS options on a router whose rule has no `Host`: `status:"warning"`, `error:["no domain found in rule PathPrefix(`/zzz`), the TLS option optA@file cannot be applied"]` (also under the `websecure-conflicted-` name).
- Also in source: cyclic reference in multi-layer routers (`router.go:460`).

Meaning: the router works but not as configured; the Panel should show "active (with warning)" and the error text.

Service `status` for HTTP: source shows every `conf.AddError(..., true)` in `pkg/server/service/service.go` uses `critical=true`, so a service is `enabled` or `disabled` and never `warning`. **Service status never reflects unreachable upstreams**, with or without healthCheck (cases e and f: service `enabled` in both). The only upstream-health signal is `serverStatus` with a healthCheck configured. Middleware status is the same (always critical).

The overview counts: `/api/overview` `http.routers` is `{"total":23,"warnings":4,"errors":6}` where `errors` counts `disabled`, matching the router list at that moment (4 warning routers, 6 disabled).

## 5. `/api/version` and `/api/overview`

```json
GET /api/version
{"Version":"3.7.19","Codename":"langres","startDate":"2026-11-22T12:30:12.040726842Z"}
```

Keys are capitalised `Version` and `Codename`, lowercase `startDate`. `Version` has no leading `v`. Pre-release builds were not tried (UNVERIFIED how `Version` looks for `3.8.0-rc.1`); parse with a semver library that tolerates pre-release tags.

```json
GET /api/overview   (healthy start, one host)
{"http":{"routers":{"total":5,"warnings":0,"errors":0},"services":{"total":5,"warnings":0,"errors":0},"middlewares":{"total":0,"warnings":0,"errors":0}},"tcp":{"routers":{"total":0,"warnings":0,"errors":0},"services":{"total":0,"warnings":0,"errors":0},"middlewares":{"total":0,"warnings":0,"errors":0}},"udp":{"routers":{"total":0,"warnings":0,"errors":0},"services":{"total":0,"warnings":0,"errors":0}},"certificates":{"total":0,"warnings":0,"errors":0},"features":{"tracing":"","metrics":"","accessLog":false},"providers":["File"]}
```

Total includes internal resources (5 routers = 1 http host x2 + 3 internal). A system banner can use: version, `startDate` (uptime / "reloaded"), `providers`, `http.routers.errors` + `warnings` counts, `certificates`. The overview's counts are global and include `@internal`, so subtract by listing or just show the deltas.

## 6. Poll cost and pagination

- Parameters `page` and `per_page` (source `pkg/api/criterion.go`): **default `per_page` is 100**, default `page` 1, no maximum in the code. Live: `/api/http/routers` returned 100 of 627; `?per_page=1000` returned all 627. Next page is announced by response header `X-Next-Page: <n>`; on the last page it is **`1`** (wraps), seen live with `per_page=200&page=4` (27 rows, `X-Next-Page: 1`). `page` past the end returns 400 `{"message":"invalid request: page: 99, per_page: 200"}`; `per_page=0` returns 400 `{"message":"invalid request: per_page: 0"}`.
- Also supported (source and live for the first two): `search=` (substring on name, rule etc.), `status=` (case-insensitive: `?status=disabled` returned exactly the 6 disabled routers), `serviceName=`, `middlewareName=`, and `sortBy`/`direction` (source only). Default order is by name.
- Cost, live with 627 routers (300 hosts x 2 plus fixtures; 200 hosts with healthCheck): `/api/http/routers?per_page=1000` 194 KB in 2-4 ms (`curl %{time_total}`); `/api/http/services?per_page=1000` 116 KB in about 2 ms; `/api/overview` 539 B in 0.6 ms; `/api/rawdata` 269 KB in 4.6 ms. Polling routers + services every 10 s is negligible (about 310 KB per poll, loopback). Server-side CPU was not profiled, but the handler reads in-memory runtime config. A cheap pattern: poll `/api/overview` every 10 s and fetch the lists only when counts change, though change in `UP/DOWN` or content would be missed, so fetching the lists each time is simpler and still cheap.
- Always loop until `X-Next-Page` is `1`, or use a large `per_page`, so >100 hosts never truncate silently. A truncated page would make hosts look "missing".

## 7. Cases where the API says healthy but traffic would fail (misleading "active")

| Situation | API shows | What clients get | Where the truth is |
| --- | --- | --- | --- |
| Upstream down, no healthCheck | router `enabled`, service `enabled`, server `UP` | 502 | nowhere in the API |
| Upstream down, healthCheck set | router `enabled`, service `enabled`, server `DOWN` | 503 | `serverStatus` (reason in logs only) |
| Unknown `tls.certResolver` | router `enabled`, no error | default self-signed cert | log line only |
| ACME issuance fails or pending | router `enabled`, no error | default self-signed cert | log; absence of SAN in `/api/certificates` |
| Router `tls: {}` with no certificate for the host | router `enabled` | default cert (`ok.test.example` served `CN=TRAEFIK DEFAULT CERT`) | `/api/certificates` |
| Malformed YAML added or edited | all old routers `enabled` | old config keeps serving, new edit not applied | log only; blocks all later edits |
| Duplicate rules | both `enabled` | one of the two, unspecified | not in the API |
| Router `warning` (TLS options conflict) | `warning` | works with default TLS options | `error` text |

Suggested Panel states: **failed** = router `disabled` (show `error`); **degraded** = router `warning`, or any `serverStatus` entry `DOWN`, or a `certResolver` host with no matching SAN in `/api/certificates`; **unknown** = expected router name missing from the listing; **active** = none of the above. Label "active" as "loaded by Traefik", and add an optional per-host TCP/HTTP probe of the upstream if "reachable" is promised, since without a healthCheck Traefik reports `UP` unconditionally.

## Provenance and gaps

- Live: all tables above, one Traefik process at a time; Pebble v2.10.3 release tarballs (sha256 `7e33cf3e...` for `pebble`, `00b2c5ef...` for `pebble-challtestsrv`, trust-on-first-use, same as the earlier note) with `pebble.json` set to `httpPort` 18081 / `tlsPort` 18443 and fixtures from the `v2.10.3` tag.
- Source: Traefik `v3.7.19` `pkg/config/runtime/runtime_http.go`, `pkg/api/criterion.go`, `pkg/api/certificate.go`, `pkg/server/aggregator.go`, `pkg/server/router/tcp/manager.go`, `pkg/muxer/http/mux.go`, `pkg/provider/file/file.go`.
- Not tried: `dashboard: true` comparison (only `false` run), TCP/UDP routers, multi-layer routers, TLS-ALPN or DNS-01 failures (same log-only path expected, UNVERIFIED), `sortBy`/`direction` live, Traefik API behaviour with `api.insecure: false` plus an explicit router, retry timing of failed ACME orders, the `Version` string of release candidates.
- The doc.traefik.io site was not used; behaviour claims rest on live runs and the source tree.
