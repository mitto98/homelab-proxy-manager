// Package version holds the build version, set with -ldflags at release time.
package version

// Version is overridden by GoReleaser: -X .../internal/version.Version=1.2.3
var Version = "dev"
