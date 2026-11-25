# Traefik distro package availability

Researched 2026-11-20. Primary sources only. Items marked UNVERIFIED could not be confirmed. Follows up the open item in `docs/wayfinder/map.md` ("Debian, Fedora and EPEL package availability for Traefik") and section 1 of `docs/research/traefik-distribution.md`.

## Summary

| Distro | `traefik` package in official repos? | Releases | Version | v3.x? |
| --- | --- | --- | --- | --- |
| Debian | No (ITP open since 2021, not uploaded) | none (bookworm, trixie, forky, sid, experimental all searched) | n/a | n/a |
| Ubuntu | No | none (jammy, noble, resolute, stonking, tenacious searched) | n/a | n/a |
| Fedora | No | none (Fedora 43, 44, 45, Rawhide) | n/a | n/a |
| EPEL | No | none (EPEL 8, 9, 10.3, 10.4) | n/a | n/a |

No package exists in any of the four, so there is no version to compare against v3.x. Latest upstream stable is v3.7.19 (see `docs/research/traefik-distribution.md`, section 4).

## Access notes

- The agent proxy blocks direct `curl` to tracker.debian.org, sources.debian.org, packages.fedoraproject.org, src.fedoraproject.org, dl.fedoraproject.org, bodhi.fedoraproject.org, launchpad API and the Debian/Fedora mirrors (CONNECT 403 or timeout). packages.ubuntu.com timed out through `curl` this session, although it worked on 2026-11-19.
- All results below were fetched with `firecrawl_scrape`, which is not subject to the proxy block. The pages are the live rendered primary sites.
- Not reached (so not used): the EPEL docs, Bodhi, Launchpad, and the actual repo metadata (`repodata`, `Packages.gz`). Availability is therefore based on the distros' own package search indexes, not on mirror metadata.
- Search indexes match package names, not "provides". A `traefik` binary shipped inside some differently named package would not show up. I did not find any evidence of one.

## 1. Debian: no package (VERIFIED, with caveat)

- packages.debian.org name search for "traefik", all suites, all sections, all architectures: 2 results, both Go libraries, neither the proxy.
  - `golang-github-traefik-paerser-dev` 0.2.0-1 (sid only)
  - `golang-github-traefik-yaegi-dev` 0.14.3-1 (bookworm), 0.16.1+git20260209.fcb76d1-3 (forky, sid)
  - https://packages.debian.org/search?keywords=traefik&searchon=names&suite=all&section=all
- Source-name search: only `golang-github-traefik-paerser` and `golang-github-traefik-yaegi` (the latter also builds a `yaegi` binary package). No `traefik` source package.
  - https://packages.debian.org/search?keywords=traefik&searchon=sourcenames&suite=all&section=all
- The suites listed by the search form include bookworm, trixie, forky, sid, experimental and their updates/backports, so those are covered by "all suites". The result list shows no trixie entry for either library; this is as the page rendered and I did not investigate it.
- sources.debian.org API for source `traefik` returns `{"error":404}`: https://sources.debian.org/api/src/traefik/
- tracker.debian.org/pkg/traefik returns 404 (no such source package): https://tracker.debian.org/pkg/traefik
- There is an open ITP, wnpp bug #983289, "ITP: traefik -- The Cloud Native Application Proxy", filed 2021-02-21 for version 2.4.3-1 (so a v2 package, not v3), owned by Alois Micard. It is blocked by many unpackaged Go dependencies (blocking bugs include #1078949 `golang-github-http-wasm-http-wasm-host-go` and #1079027 `golang-github-infobloxopen-infoblox-go-client`). The last log entry is 2025-11-29 (an "affects src:traefik" annotation by another developer); nothing shows an upload. Page last modified 2026-11-20.
  - https://bugs.debian.org/983289
- Not checked: Debian NEW queue, and whether the ITP owner has a private or salsa repository. UNVERIFIED whether packaging is further along than the bug log shows.
- Conclusion: no `traefik` in Debian 12, 13 or testing/unstable. Even if the ITP lands it would arrive in a future release or sid first, and would be a v2 package unless the target version is revised.

## 2. Ubuntu: no package (VERIFIED)

- packages.ubuntu.com name search "traefik", all suites: 2 results, the same two libraries, both in universe.
  - `golang-github-traefik-paerser-dev` 0.2.0-1 in resolute (26.04 LTS), stonking (26.10), tenacious
  - `golang-github-traefik-yaegi-dev` 0.10.0-2 (jammy 22.04), 0.15.1-2 (noble 24.04), 0.16.1-2 (resolute), 0.16.1+git20260209.fcb76d1-2 (stonking), 0.16.1+git20260209.fcb76d1-3 (tenacious)
  - https://packages.ubuntu.com/search?keywords=traefik&searchon=names&suite=all&section=all
- Suites covered by that search: jammy, noble, resolute, stonking, tenacious (plus -updates and -backports). The earlier note also found no `traefik` source package on a source-name search; I did not repeat it.
- Launchpad was not reachable, so a `traefik` source in a PPA or an unlisted pocket is UNVERIFIED. PPAs are not official repositories anyway.

## 3. Fedora: no package (VERIFIED)

- packages.fedoraproject.org search "traefik" returns zero results in every release filter: Fedora 43, 44, 45, Rawhide. The page only suggests "traffic".
  - https://packages.fedoraproject.org/search?query=traefik
  - With "show related packages" also on: still 0 for every release. https://packages.fedoraproject.org/search?query=traefik&show_related=true&start=0
- src.fedoraproject.org/rpms/traefik returns 404 "Project not found", so there is no dist-git repository for it, not even a retired one: https://src.fedoraproject.org/rpms/traefik
- Not checked: Bodhi, Fedora review requests in Bugzilla, and COPR (COPR is not official anyway).

## 4. EPEL (RHEL 9+): no package (VERIFIED)

- The same Fedora Packages search covers EPEL branches and returns 0 for Fedora EPEL 8, EPEL 9, EPEL 10.3 and EPEL 10.4 (RHEL 9 and 10 are the relevant ones for the map's "RHEL-family 9+").
  - https://packages.fedoraproject.org/search?query=traefik
- No dist-git project exists (see Fedora, above), so no EPEL branch can exist for it.
- RHEL's own repositories (BaseOS, AppStream, CRB) were not checked; no Red Hat source was reachable or searched. UNVERIFIED, though there is no sign Red Hat ships Traefik. EPEL packages never replace RHEL packages, and there is no `traefik` in EPEL either way.

## 5. Traefik Labs side

Already covered in `docs/research/traefik-distribution.md`: upstream publishes no `.deb`/`.rpm`, only tarballs/zips and the Docker image. I did not re-fetch the Traefik docs, since doc.traefik.io is blocked and the repo copy was already read.

## Conclusion

Nothing here changes the map's decision. No distro in the supported-target list (Debian 12+, Ubuntu 22.04+, Fedora current, RHEL-family 9+) ships a `traefik` package, so there is nothing a package could declare `Depends:` on. Keep requiring an external `traefik` binary on PATH and no `Depends:` on a Traefik package.

Follow-ups for the map:

- The "Not yet specified" item can be closed: Debian, Ubuntu, Fedora and EPEL have no Traefik package. The only residual unknowns are the Debian ITP progress (not visible in the bug log) and RHEL's own repos.
- The post-install message should not suggest `apt install traefik` or `dnf install traefik`. It should point to the upstream release tarball (`traefik_vX.Y.Z_linux_<arch>.tar.gz`) or Docker.
- Revisit if Debian #983289 ships. A distro package would likely lag upstream and may be v2 (the ITP targets 2.4.3), which would fall outside the Panel's supported v3.x range. If it ever matters, use `Recommends:`/`Suggests:` with a version guard rather than `Depends:`.
