//go:build !linux

package traefiktest

// lockFile is a no-op off linux: the harness refuses to run there anyway (see
// ErrUnsupported), this stub only keeps the package compiling for `go vet ./...`.
func lockFile(string) (func(), error) { return func() {}, nil }
