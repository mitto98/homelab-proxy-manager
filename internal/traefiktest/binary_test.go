package traefiktest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeTarball builds a release-shaped tarball with a license, a nested decoy and a
// "traefik" file, and returns it with its sha256.
func fakeTarball(t *testing.T, binary string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct{ name, body string }{
		{"LICENSE.md", "license"},
		{"sub/traefik", "decoy"},
		{"traefik", binary},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func serve(t *testing.T, body []byte, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPinIsComplete(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		sum := tarballSHA256[arch]
		if len(sum) != 64 {
			t.Errorf("%s: pinned hash %q is not a sha256 hex digest", arch, sum)
		}
		url := tarballURL(arch)
		if !strings.HasSuffix(url, "/v"+Version+"/traefik_v"+Version+"_linux_"+arch+".tar.gz") {
			t.Errorf("%s: unexpected URL %s", arch, url)
		}
	}
}

func TestVerifySHA256RejectsCorruptedCopy(t *testing.T) {
	tarball, sum := fakeTarball(t, "real")
	good := filepath.Join(t.TempDir(), "good.tgz")
	if err := os.WriteFile(good, tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(good, sum); err != nil {
		t.Fatalf("intact file rejected: %v", err)
	}

	corrupt := append([]byte(nil), tarball...)
	corrupt[len(corrupt)/2] ^= 0xff
	bad := filepath.Join(t.TempDir(), "bad.tgz")
	if err := os.WriteFile(bad, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(bad, sum); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("corrupted file accepted, err = %v", err)
	}
}

func TestInstallerRejectsHashMismatch(t *testing.T) {
	tarball, sum := fakeTarball(t, "real")
	tarball[len(tarball)-10] ^= 0xff // served bytes differ from the pinned hash
	var hits atomic.Int32
	srv := serve(t, tarball, &hits)
	dir := filepath.Join(t.TempDir(), "cache")

	in := installer{url: srv.URL, wantSHA: sum, dir: dir, client: srv.Client()}
	if _, err := in.ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("err = %v, want sha256 mismatch", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "traefik")); !os.IsNotExist(err) {
		t.Errorf("binary installed despite the mismatch: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestInstallerDownloadsOnceAndExtractsOnlyTheBinary(t *testing.T) {
	tarball, sum := fakeTarball(t, "#!/bin/sh\n")
	var hits atomic.Int32
	srv := serve(t, tarball, &hits)
	dir := filepath.Join(t.TempDir(), "cache")
	in := installer{url: srv.URL, wantSHA: sum, dir: dir, client: srv.Client()}

	bin, err := in.ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "#!/bin/sh\n" {
		t.Errorf("binary content = %q (the nested decoy must not be used)", got)
	}
	if fi, _ := os.Stat(bin); fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("binary is not executable: %v", fi.Mode())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		switch e.Name() {
		case "traefik", "traefik.sha256", ".lock":
		default:
			t.Errorf("unexpected file in cache: %s", e.Name())
		}
	}

	// A second call is served from the cache.
	if _, err := in.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Errorf("downloaded %d times, want 1", hits.Load())
	}

	// A cache entry that no longer matches its recorded hash is replaced.
	if err := os.WriteFile(bin, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := in.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Errorf("tampered cache not repaired (downloads = %d)", hits.Load())
	}
	if got, _ := os.ReadFile(bin); string(got) != "#!/bin/sh\n" {
		t.Errorf("binary not restored: %q", got)
	}
}

func TestInstallerTarballWithoutBinary(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "LICENSE.md", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(buf.Bytes())
	var hits atomic.Int32
	srv := serve(t, buf.Bytes(), &hits)
	in := installer{url: srv.URL, wantSHA: hex.EncodeToString(sum[:]), dir: filepath.Join(t.TempDir(), "c"), client: srv.Client()}
	if _, err := in.ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "no traefik binary") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	in := installer{url: srv.URL, wantSHA: "00", dir: filepath.Join(t.TempDir(), "c"), client: srv.Client()}
	if _, err := in.ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v", err)
	}
}
