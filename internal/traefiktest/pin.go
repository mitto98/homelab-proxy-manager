package traefiktest

import "fmt"

// Version is the one Traefik release the project tests against. Bump it here,
// together with the hashes below and the Dockerfile tag, in a single pull request.
const Version = "3.7.19"

// tarballSHA256 holds the sha256 of each linux release tarball, copied from
// traefik_v3.7.19_checksums.txt on the GitHub release and cross-checked against a
// second download of the tarball.
//
// These committed values are the trust anchor. Traefik's release workflow does not
// sign anything, and the upstream checksums file sits next to the tarball in the
// same release, so it only detects corruption: someone who could replace the
// tarball could replace that file too. Pinning the hashes here means a changed
// upstream asset fails the build instead of being trusted.
var tarballSHA256 = map[string]string{
	"amd64": "97534a68335a087137d0f2adecefe37c8216ac10114dc7e5b510f2a8a12f89be",
	"arm64": "6d796c41b8d3c8c31bb2faf1bfff1b5fcf603676ad9a773b29ffa7efa678881a",
}

// tarballURL is the official release asset for the given GOARCH.
func tarballURL(arch string) string {
	return fmt.Sprintf("https://github.com/traefik/traefik/releases/download/v%[1]s/traefik_v%[1]s_linux_%[2]s.tar.gz", Version, arch)
}
