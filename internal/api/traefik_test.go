package api

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
	"github.com/mitto98/homelab-proxy-manager/internal/render"
	"github.com/mitto98/homelab-proxy-manager/internal/traefiktest"
)

// These tests put the whole walking skeleton together: a host created through
// the Panel's API becomes files, a real Traefik loads them, and a request is
// routed to an upstream. They skip when the Traefik binary cannot be obtained
// and fail with HPM_REQUIRE_TRAEFIK=1 (see internal/traefiktest).

const reloadTimeout = 30 * time.Second // Traefik applies file changes after a 2 s throttle

// upstream answers every request with its name.
func upstream(t *testing.T, name string) (scheme, host string, port int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "upstream=%s host=%s", name, r.Host)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	p, _ := strconv.Atoi(u.Port())
	return u.Scheme, u.Hostname(), p
}

func hostBody(domain, host string, port int, enabled bool) string {
	return fmt.Sprintf(`{"domains":[%q],"upstreamScheme":"http","upstreamHost":%q,"upstreamPort":%d,"enabled":%t}`, domain, host, port, enabled)
}

// waitBody polls Traefik until the response for host contains want.
func waitBody(t *testing.T, tr *traefiktest.Traefik, host, want string) {
	t.Helper()
	deadline := time.Now().Add(reloadTimeout)
	var status int
	var body string
	for {
		var err error
		if status, body, err = tr.Get(host, "/"); err == nil && status == http.StatusOK && strings.Contains(body, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s via traefik: status %d body %q, want 200 containing %q\ntraefik output:\n%s", host, status, body, want, tr.Output())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func routerListed(t *testing.T, tr *traefiktest.Traefik, name string) bool {
	t.Helper()
	routers, err := tr.Routers()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range routers {
		if r.Name == name {
			return true
		}
	}
	return false
}

func TestHostLifecycleThroughTraefik(t *testing.T) {
	t.Parallel()
	tr := traefiktest.Start(t)

	// The Panel writes into the harness's watched directory. Its traefik.yml
	// lands next to it and replaces the harness's; the running Traefik has
	// already read its own, so this only matters to the second test below.
	ts, _ := newServerAt(t, filepath.Dir(tr.DynamicDir()))
	c := signedIn(t, ts)

	_, h1, p1 := upstream(t, "one")
	_, h2, p2 := upstream(t, "two")

	// Create.
	res := do(t, c, "POST", ts.URL+"/api/hosts", hostBody("app.test", h1, p1, true))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", res.StatusCode)
	}
	host := decodeHost(t, res)
	router := fmt.Sprintf("host-%d-http@file", host.Id)
	tr.WaitForRouter(router, reloadTimeout)
	waitBody(t, tr, "app.test", "upstream=one host=app.test")

	// Update: new upstream.
	url := fmt.Sprintf("%s/api/hosts/%d", ts.URL, host.Id)
	if got := do(t, c, "PUT", url, hostBody("app.test", h2, p2, true)).StatusCode; got != http.StatusOK {
		t.Fatalf("update: %d", got)
	}
	waitBody(t, tr, "app.test", "upstream=two")

	// Disable: routed nowhere, router gone.
	if got := do(t, c, "PUT", url, hostBody("app.test", h2, p2, false)).StatusCode; got != http.StatusOK {
		t.Fatalf("disable: %d", got)
	}
	tr.WaitForStatus("app.test", "/", http.StatusNotFound, reloadTimeout)
	if routerListed(t, tr, router) {
		t.Error("a disabled host's router is still listed")
	}

	// Enable again, and change the domain.
	if got := do(t, c, "PUT", url, hostBody("renamed.test", h2, p2, true)).StatusCode; got != http.StatusOK {
		t.Fatalf("re-enable: %d", got)
	}
	waitBody(t, tr, "renamed.test", "upstream=two")
	if status, _, err := tr.Get("app.test", "/"); err != nil || status != http.StatusNotFound {
		t.Errorf("old domain: status %d, err %v; want 404", status, err)
	}

	// Delete.
	if got := do(t, c, "DELETE", url, "").StatusCode; got != http.StatusNoContent {
		t.Fatalf("delete: %d", got)
	}
	tr.WaitForStatus("renamed.test", "/", http.StatusNotFound, reloadTimeout)
	if routerListed(t, tr, router) {
		t.Error("a deleted host's router is still listed")
	}
}

// Traefik must accept the static config the Panel generates, and with it the API
// must be reachable on loopback and nowhere else.
func TestPanelStaticConfigStartsTraefik(t *testing.T) {
	t.Parallel()
	var cfg traefiktest.Config
	tr := traefiktest.Start(t, traefiktest.WithStaticConfig(func(c traefiktest.Config) string {
		cfg = c
		m := render.Build(render.Input{
			Settings:   model.Settings{HTTPPort: c.HTTPPort, HTTPSPort: c.HTTPSPort, APIPort: c.APIPort},
			DynamicDir: c.DynamicDir,
		})
		b, err := m.StaticYAML()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}))

	// The Panel runs on the same data directory with the same ports, so its own
	// traefik.yml is byte-identical to the one Traefik started with.
	ts, st := newServerAt(t, filepath.Dir(tr.DynamicDir()))
	if err := st.UpdateSettings(t.Context(), model.Settings{HTTPPort: cfg.HTTPPort, HTTPSPort: cfg.HTTPSPort, APIPort: cfg.APIPort}); err != nil {
		t.Fatal(err)
	}
	c := signedIn(t, ts)
	_, h, p := upstream(t, "generated")
	res := do(t, c, "POST", ts.URL+"/api/hosts", hostBody("gen.test", h, p, true))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", res.StatusCode)
	}
	waitBody(t, tr, "gen.test", "upstream=generated")

	// Loopback only: the entrypoint address as Traefik reports it, and a refused
	// dial on a non-loopback address of this machine.
	body, err := tr.APIGet("/api/entrypoints/traefik")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, fmt.Sprintf("127.0.0.1:%d", tr.APIPort())) {
		t.Errorf("traefik entrypoint = %s, want 127.0.0.1:%d", body, tr.APIPort())
	}
	addr := nonLoopbackIPv4(t)
	if addr == "" {
		t.Log("no non-loopback IPv4 address here; skipping the dial check")
		return
	}
	if l, err := net.Listen("tcp", net.JoinHostPort(addr, "0")); err != nil {
		t.Logf("cannot bind %s here, skipping the dial check: %v", addr, err)
		return
	} else {
		l.Close()
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, strconv.Itoa(tr.APIPort())), 2*time.Second)
	if err == nil {
		conn.Close()
		t.Fatalf("the API accepted a connection on %s:%d; it must listen on 127.0.0.1 only", addr, tr.APIPort())
	}
}

func nonLoopbackIPv4(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	return ""
}
