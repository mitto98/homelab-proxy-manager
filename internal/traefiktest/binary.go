package traefiktest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	// cacheEnv overrides the directory the binary is cached in.
	cacheEnv = "HPM_TRAEFIK_CACHE"

	// Size caps keep a bad or hostile download from filling the disk. The real
	// tarball is about 50 MB and the binary about 150 MB.
	maxTarballBytes = 256 << 20
	maxBinaryBytes  = 512 << 20

	downloadTimeout = 5 * time.Minute
)

// ErrUnsupported means the harness cannot run Traefik on this platform.
var ErrUnsupported = errors.New("traefiktest: only linux amd64 and arm64 are supported")

var (
	binaryOnce sync.Mutex
	binaryDone bool
	binaryPath string
	binaryErr  error
)

// Binary returns the path of the pinned, hash-verified traefik binary, downloading
// it into the cache on first use. The outcome, including a failure, is remembered
// for the life of the process, so a test run without network does not retry a slow
// download in every test.
func Binary() (string, error) {
	binaryOnce.Lock()
	defer binaryOnce.Unlock()
	if binaryDone {
		return binaryPath, binaryErr
	}
	binaryDone = true
	binaryPath, binaryErr = locateBinary()
	return binaryPath, binaryErr
}

func locateBinary() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("%w (running on %s/%s)", ErrUnsupported, runtime.GOOS, runtime.GOARCH)
	}
	want, ok := tarballSHA256[runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("%w (running on %s/%s)", ErrUnsupported, runtime.GOOS, runtime.GOARCH)
	}
	base := os.Getenv(cacheEnv)
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("find cache directory (set %s): %w", cacheEnv, err)
		}
		base = filepath.Join(cache, "homelab-proxy-manager", "traefik")
	}
	in := installer{
		url:     tarballURL(runtime.GOARCH),
		wantSHA: want,
		dir:     filepath.Join(base, Version, "linux_"+runtime.GOARCH),
		client:  http.DefaultClient, // honours HTTPS_PROXY
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	return in.ensure(ctx)
}

// installer puts one verified binary into dir. It is separate from Binary so tests
// can drive it with a local server and a tiny fake tarball.
type installer struct {
	url     string
	wantSHA string // hex sha256 of the tarball
	dir     string
	client  *http.Client
}

// ensure returns the cached binary or installs it. A lock file serialises
// concurrent test processes (go test runs one process per package), and the binary
// appears by atomic rename, so nobody sees a half-written file.
func (in installer) ensure(ctx context.Context) (string, error) {
	bin := filepath.Join(in.dir, "traefik")
	if cachedOK(bin) {
		return bin, nil
	}
	if err := os.MkdirAll(in.dir, 0o755); err != nil {
		return "", err
	}
	unlock, err := lockFile(filepath.Join(in.dir, ".lock"))
	if err != nil {
		return "", fmt.Errorf("lock cache: %w", err)
	}
	defer unlock()

	// Another process may have finished the install while we waited for the lock.
	if cachedOK(bin) {
		return bin, nil
	}

	tarball, err := in.download(ctx)
	if err != nil {
		return "", err
	}
	defer os.Remove(tarball)

	binHash, err := installBinary(tarball, bin)
	if err != nil {
		return "", err
	}
	// The sidecar is written last: a crash before this point leaves a cache miss,
	// never a binary that looks verified.
	if err := writeFileAtomic(in.dir, "traefik.sha256", []byte(binHash+"\n"), 0o644); err != nil {
		return "", err
	}
	return bin, nil
}

// cachedOK reports whether bin exists and still hashes to the value recorded when
// it was installed. That catches a truncated or edited cache file; it is not a
// defence against someone with write access to the cache, who could edit both.
func cachedOK(bin string) bool {
	rec, err := os.ReadFile(bin + ".sha256")
	if err != nil {
		return false
	}
	got, err := hashFile(bin)
	if err != nil {
		return false
	}
	return got+"\n" == string(rec)
}

// download fetches the tarball to a temp file in the cache dir and checks it
// against the pinned hash. The file is removed on mismatch.
func (in installer) download(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.url, nil)
	if err != nil {
		return "", err
	}
	resp, err := in.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", in.url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %s", in.url, resp.Status)
	}

	f, err := os.CreateTemp(in.dir, ".download-*.part")
	if err != nil {
		return "", err
	}
	name := f.Name()
	fail := func(err error) (string, error) {
		f.Close()
		os.Remove(name)
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxTarballBytes+1))
	if err != nil {
		return fail(fmt.Errorf("download %s: %w", in.url, err))
	}
	if n > maxTarballBytes {
		return fail(fmt.Errorf("download %s: larger than %d bytes", in.url, maxTarballBytes))
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != in.wantSHA {
		return fail(fmt.Errorf("download %s: sha256 mismatch: got %s, pinned %s", in.url, got, in.wantSHA))
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// verifySHA256 checks a file against a hex digest. It is what a test corrupts a
// copy to exercise.
func verifySHA256(file, want string) error {
	got, err := hashFile(file)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("sha256 mismatch for %s: got %s, pinned %s", file, got, want)
	}
	return nil
}

func hashFile(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// installBinary extracts only the top-level "traefik" file from the tarball and
// renames it to dest, returning its sha256. Every other entry (license, readme) is
// ignored, so nothing from the archive is written under a path it chose.
func installBinary(tarball, dest string) (string, error) {
	f, err := os.Open(tarball)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("read tarball: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", errors.New("tarball has no traefik binary")
		}
		if err != nil {
			return "", fmt.Errorf("read tarball: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || path.Clean(hdr.Name) != "traefik" {
			continue
		}
		return writeExecutable(dest, io.LimitReader(tr, maxBinaryBytes+1))
	}
}

func writeExecutable(dest string, r io.Reader) (string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".extract-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		tmp.Close()
		return "", err
	}
	if n > maxBinaryBytes {
		tmp.Close()
		return "", fmt.Errorf("binary larger than %d bytes", maxBinaryBytes)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
