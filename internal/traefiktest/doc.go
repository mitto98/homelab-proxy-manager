// Package traefiktest runs a real, pinned Traefik for integration tests.
//
// A test calls [Start], writes dynamic config with [Traefik.WriteDynamic], waits
// with [Traefik.WaitForRouter] and sends requests with [Traefik.Get]:
//
//	tr := traefiktest.Start(t)
//	tr.WriteDynamic("app", yaml)
//	tr.WaitForRouter("app@file", 20*time.Second)
//	status, body, err := tr.Get("app.test", "/")
//
// # The binary
//
// [Version] and the sha256 of the linux amd64 and arm64 release tarballs are pinned
// in pin.go. The tarball is downloaded from the official GitHub release, checked
// against the committed hash (a mismatch is an error, nothing is installed), and
// only the "traefik" file is extracted into a cache, by default
// os.UserCacheDir()/homelab-proxy-manager/traefik/<version>/linux_<arch>/, or
// $HPM_TRAEFIK_CACHE/<version>/linux_<arch>/. A lock file and an atomic rename keep
// parallel test packages from corrupting it; a cached binary is reused while it
// still matches the hash recorded at install time. Linux only.
//
// When the binary cannot be obtained, tests that need it skip with the reason.
// With HPM_REQUIRE_TRAEFIK=1 (CI sets it) they fail instead, so a download problem
// cannot hide as a skip.
//
// # Findings on Traefik 3.7.19
//
// These were checked against the live binary by the tests in this package (and
// agree with docs/research/traefik-api-status.md). They are the facts the apply
// pipeline and the status endpoint rely on.
//
// Resource names (c). The API lists a resource from the file provider as
// "<name>@file", in its "name" field and in references. A router defined as "app"
// in a dynamic file is "app@file", and its JSON has "provider":"file". Traefik also
// lists its own "@internal" routers (api, ping, acme-http, noop); filter on the
// "@file" suffix. [Traefik.WaitForRouter] takes the full name.
//
// Dashboard off (b). With api.dashboard: false and api.insecure: true the JSON API
// still serves /api/version, /api/http/routers, /api/http/services, /api/entrypoints
// and /api/rawdata. Only the dashboard (/dashboard/) disappears and answers 404.
//
// Invalid dynamic file (a). If a file in the watched directory is not valid YAML,
// Traefik logs "Error occurred during watcher callback ... collecting file configs:
// <path>: yaml: ..." at ERR level and keeps serving the previous configuration:
// routers stay enabled in the API and requests keep being routed. Nothing in the
// API reveals the problem, only the log does. But the provider rebuilds the whole
// directory on every event and aborts at the first bad file, so while the bad file
// is present, valid changes to other files are not applied either (a new router
// stays absent and its host answers 404). Once the bad file is removed or fixed,
// the next reload applies everything, including the changes made meanwhile.
// Consequence: the apply pipeline must validate before writing, and must never
// leave a half-valid file in the directory; one bad file freezes all hosts.
//
// Other details the helpers encode:
//   - The API is loopback-only only because the "traefik" entry point is declared
//     as 127.0.0.1:<port>. Without that block Traefik invents ":8080" on every
//     interface.
//   - /api/http/routers returns 100 entries by default and drops the rest. The
//     helper asks for per_page=1000 and follows X-Next-Page, which holds the next
//     page number and wraps to 1 on the last page.
//   - The file provider loads only .yml, .yaml and .toml files, so temp files use a
//     ".tmp" suffix and are renamed into place.
//   - File changes apply after the provider's 2 s throttle, so tests poll.
package traefiktest
