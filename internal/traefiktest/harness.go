package traefiktest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// RequireEnv makes tests fail instead of skip when Traefik cannot be obtained.
// CI sets it so a download problem cannot hide as a skip.
const RequireEnv = "HPM_REQUIRE_TRAEFIK"

const (
	pollInterval = 100 * time.Millisecond
	// startTimeout covers process start and the first API answer, not provider reloads.
	startTimeout = 30 * time.Second
)

// Config is what a static-config generator needs to know: where the harness put
// things. The generated config must bind these ports and watch this directory, or
// the helpers will not find Traefik.
type Config struct {
	HTTPPort   int
	HTTPSPort  int
	APIPort    int
	DynamicDir string
}

// Option customises Start.
type Option func(*options)

type options struct {
	staticConfig func(Config) string
}

// WithStaticConfig replaces the harness's minimal static config with the output
// of gen, for tests of a generated traefik.yml. gen must keep the API on
// 127.0.0.1:APIPort and the file provider on DynamicDir.
func WithStaticConfig(gen func(Config) string) Option {
	return func(o *options) { o.staticConfig = gen }
}

// Traefik is a running Traefik process owned by one test.
type Traefik struct {
	t    testing.TB
	cfg  Config
	dir  string
	out  *syncBuffer
	done chan struct{} // closed when the process has exited
	err  error         // exit error, valid once done is closed
	http *http.Client
}

// Start runs the pinned Traefik against an empty dynamic directory and returns once
// its API answers. The process is killed when the test ends.
//
// If the binary cannot be obtained the test is skipped, or failed when
// HPM_REQUIRE_TRAEFIK=1.
func Start(t testing.TB, opts ...Option) *Traefik {
	t.Helper()
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	bin, err := Binary()
	if err != nil {
		if os.Getenv(RequireEnv) == "1" {
			t.Fatalf("traefik binary unavailable (%s=1): %v", RequireEnv, err)
		}
		t.Skipf("traefik binary unavailable, skipping (set %s=1 to fail instead): %v", RequireEnv, err)
	}

	dir := t.TempDir()
	dynDir := filepath.Join(dir, "dynamic")
	if err := os.Mkdir(dynDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ports := freePorts(t, 3)
	cfg := Config{HTTPPort: ports[0], HTTPSPort: ports[1], APIPort: ports[2], DynamicDir: dynDir}

	gen := o.staticConfig
	if gen == nil {
		gen = defaultStaticConfig
	}
	cfgPath := filepath.Join(dir, "traefik.yml")
	if err := os.WriteFile(cfgPath, []byte(gen(cfg)), 0o600); err != nil {
		t.Fatal(err)
	}

	tr := &Traefik{
		t:    t,
		cfg:  cfg,
		dir:  dir,
		out:  &syncBuffer{},
		done: make(chan struct{}),
		http: &http.Client{
			Timeout: 5 * time.Second,
			// Tests look at redirects; they must not follow them.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{DisableKeepAlives: true},
		},
	}

	cmd := exec.Command(bin, "--configFile="+cfgPath)
	cmd.Stdout = tr.out
	cmd.Stderr = tr.out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start traefik: %v", err)
	}
	go func() {
		tr.err = cmd.Wait()
		close(tr.done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-tr.done
		// Traefik's log is the first thing needed when a test fails.
		if t.Failed() || testing.Verbose() {
			t.Logf("traefik output:\n%s", tr.out.String())
		}
	})

	if err := tr.poll(startTimeout, func() error {
		_, err := tr.APIGet("/api/version")
		return err
	}); err != nil {
		t.Fatalf("traefik did not become ready: %v\ntraefik output:\n%s", err, tr.out.String())
	}
	return tr
}

// defaultStaticConfig is deliberately minimal. Entry points listen on loopback
// only so a test run exposes nothing. The API entry point is named "traefik" and
// given an explicit 127.0.0.1 address: without that block Traefik invents one on
// ":8080", all interfaces, and api.insecure would serve the API there.
func defaultStaticConfig(c Config) string {
	return fmt.Sprintf(`global:
  checkNewVersion: false
  sendAnonymousUsage: false
log:
  level: INFO
  noColor: true
entryPoints:
  web:
    address: "127.0.0.1:%d"
  websecure:
    address: "127.0.0.1:%d"
  traefik:
    address: "127.0.0.1:%d"
api:
  insecure: true
  dashboard: false
providers:
  file:
    directory: %q
    watch: true
`, c.HTTPPort, c.HTTPSPort, c.APIPort, c.DynamicDir)
}

// HTTPAddr is the host:port of the "web" entry point.
func (tr *Traefik) HTTPAddr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(tr.cfg.HTTPPort))
}

// HTTPSAddr is the host:port of the "websecure" entry point.
func (tr *Traefik) HTTPSAddr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(tr.cfg.HTTPSPort))
}

// APIPort is the port of the "traefik" entry point.
func (tr *Traefik) APIPort() int { return tr.cfg.APIPort }

// APIURL is the base URL of the API, without a trailing slash.
func (tr *Traefik) APIURL() string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(tr.cfg.APIPort))
}

// DynamicDir is the directory the file provider watches.
func (tr *Traefik) DynamicDir() string { return tr.cfg.DynamicDir }

// Output returns what Traefik has logged so far.
func (tr *Traefik) Output() string { return tr.out.String() }

// WriteDynamic writes <name>.yml into the watched directory atomically. Pass the
// name without an extension.
func (tr *Traefik) WriteDynamic(name, yaml string) {
	tr.t.Helper()
	if strings.ContainsAny(name, `/\`) || strings.HasSuffix(name, ".yml") {
		tr.t.Fatalf("WriteDynamic: name %q must be a bare name without extension", name)
	}
	if err := writeFileAtomic(tr.cfg.DynamicDir, name+".yml", []byte(yaml), 0o644); err != nil {
		tr.t.Fatalf("WriteDynamic %s: %v", name, err)
	}
}

// RemoveDynamic deletes <name>.yml from the watched directory.
func (tr *Traefik) RemoveDynamic(name string) {
	tr.t.Helper()
	if err := os.Remove(filepath.Join(tr.cfg.DynamicDir, name+".yml")); err != nil {
		tr.t.Fatalf("RemoveDynamic %s: %v", name, err)
	}
}

// Routers lists every router Traefik reports, internal ones included.
func (tr *Traefik) Routers() ([]Router, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return listRouters(ctx, tr.http, tr.APIURL())
}

// WaitForRouter polls the API until a router called name (with its provider
// suffix, for example "host-1@file") is listed and enabled. It polls rather than
// sleeps because Traefik applies file changes only after its 2 s provider throttle.
// On timeout the test fails with every router seen.
func (tr *Traefik) WaitForRouter(name string, timeout time.Duration) {
	tr.t.Helper()
	var seen string
	err := tr.poll(timeout, func() error {
		routers, err := tr.Routers()
		if err != nil {
			return err
		}
		found, desc := findRouter(routers, name)
		if !found {
			seen = desc
			return fmt.Errorf("router %q not enabled", name)
		}
		return nil
	})
	if err != nil {
		tr.t.Fatalf("router %q not enabled within %s: %v\n%s\ntraefik output:\n%s", name, timeout, err, seen, tr.out.String())
	}
}

// WaitForLog polls Traefik's output until it contains substr.
func (tr *Traefik) WaitForLog(substr string, timeout time.Duration) {
	tr.t.Helper()
	err := tr.poll(timeout, func() error {
		if !strings.Contains(tr.out.String(), substr) {
			return fmt.Errorf("log does not contain %q", substr)
		}
		return nil
	})
	if err != nil {
		tr.t.Fatalf("%v within %s\ntraefik output:\n%s", err, timeout, tr.out.String())
	}
}

// WaitForStatus polls GET http://host<path> through the web entry point until it
// answers with the wanted status code, and returns the body. A router can be listed
// a moment before the new configuration serves requests, so tests wait on the
// behaviour they assert.
func (tr *Traefik) WaitForStatus(host, path string, want int, timeout time.Duration) string {
	tr.t.Helper()
	var body string
	err := tr.poll(timeout, func() error {
		status, b, err := tr.Get(host, path)
		if err != nil {
			return err
		}
		if status != want {
			return fmt.Errorf("status %d, want %d", status, want)
		}
		body = b
		return nil
	})
	if err != nil {
		tr.t.Fatalf("GET %s%s via traefik: %v\ntraefik output:\n%s", host, path, err, tr.out.String())
	}
	return body
}

// Get sends GET path to the "web" entry point with the given Host header, as a
// browser would for that site, and returns the status and body. Redirects are not
// followed.
func (tr *Traefik) Get(host, path string) (status int, body string, err error) {
	req, err := http.NewRequest(http.MethodGet, "http://"+tr.HTTPAddr()+path, nil)
	if err != nil {
		return 0, "", err
	}
	req.Host = host
	resp, err := tr.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return resp.StatusCode, string(b), err
}

// APIGet fetches a path from the API and returns the body. A non-200 answer is an error.
func (tr *Traefik) APIGet(path string) (string, error) {
	resp, err := tr.http.Get(tr.APIURL() + path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return string(b), fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return string(b), nil
}

// poll runs fn until it returns nil, the timeout passes, or Traefik exits. The last
// error from fn is reported on timeout.
func (tr *Traefik) poll(timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		select {
		case <-tr.done:
			return fmt.Errorf("traefik exited: %v", tr.err)
		default:
		}
		if last = fn(); last == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s: %w", timeout, last)
		}
		time.Sleep(pollInterval)
	}
}

// freePorts asks the kernel for n distinct free loopback ports. There is a small
// window before Traefik binds them; a collision shows up as Traefik exiting early,
// with its output in the failure message.
func freePorts(t testing.TB, n int) []int {
	t.Helper()
	var ls []net.Listener
	var ports []int
	for range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ls = append(ls, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	for _, l := range ls {
		l.Close()
	}
	return ports
}

// syncBuffer is a bytes.Buffer safe for the process's output copier and the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
