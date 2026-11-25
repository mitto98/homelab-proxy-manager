package traefiktest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// reloadTimeout is generous: Traefik applies file changes only after its 2 s
// provider throttle, and CI machines are slow.
const reloadTimeout = 20 * time.Second

// upstream starts a server that answers with body and returns its URL.
func upstream(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s host=%s", body, r.Host)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// routeYAML is a dynamic file with one router, name, sending host to url.
func routeYAML(name, host, url string) string {
	return fmt.Sprintf("http:\n  routers:\n    %[1]s:\n      rule: Host(`%[2]s`)\n      entryPoints: [web]\n      service: %[1]s\n  services:\n    %[1]s:\n      loadBalancer:\n        servers:\n          - url: %[3]q\n", name, host, url)
}

func TestRequestReachesUpstream(t *testing.T) {
	t.Parallel()
	up := upstream(t, "hello")
	tr := Start(t)

	tr.WriteDynamic("app", routeYAML("app", "app.test", up.URL))
	// The router carries the provider suffix.
	tr.WaitForRouter("app@file", reloadTimeout)

	body := tr.WaitForStatus("app.test", "/", http.StatusOK, reloadTimeout)
	if !strings.HasPrefix(body, "hello host=app.test") {
		t.Errorf("body = %q, want the upstream's answer with the original Host header", body)
	}
	// A host nobody routes is Traefik's own 404, not the upstream.
	if status, _, err := tr.Get("other.test", "/"); err != nil || status != http.StatusNotFound {
		t.Errorf("unrouted host: status %d, err %v; want 404", status, err)
	}
}

func TestHotReloadAddsAndRemovesRouters(t *testing.T) {
	t.Parallel()
	up := upstream(t, "x")
	tr := Start(t)

	tr.WriteDynamic("one", routeYAML("one", "one.test", up.URL))
	tr.WaitForRouter("one@file", reloadTimeout)
	tr.WriteDynamic("two", routeYAML("two", "two.test", up.URL))
	tr.WaitForRouter("two@file", reloadTimeout)

	tr.RemoveDynamic("one")
	tr.WaitForStatus("one.test", "/", http.StatusNotFound, reloadTimeout)
	tr.WaitForStatus("two.test", "/", http.StatusOK, reloadTimeout)
}

// The API must be reachable on loopback and nowhere else. Without an explicit
// 127.0.0.1 address on the "traefik" entry point, Traefik listens on :8080 on
// every interface and api.insecure would expose the whole configuration.
func TestAPIIsLoopbackOnly(t *testing.T) {
	t.Parallel()
	tr := Start(t)

	if _, err := tr.APIGet("/api/version"); err != nil {
		t.Fatalf("API not reachable on 127.0.0.1: %v", err)
	}

	addr := nonLoopbackAddr(t)
	if addr == "" {
		t.Skip("this machine has no non-loopback IPv4 address to probe")
	}
	// Prove the address is usable at all, or a refusal below would mean nothing.
	l, err := net.Listen("tcp", net.JoinHostPort(addr, "0"))
	if err != nil {
		t.Skipf("cannot bind %s here, so a refusal there proves nothing: %v", addr, err)
	}
	l.Close()

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, fmt.Sprint(tr.APIPort())), 2*time.Second)
	if err == nil {
		conn.Close()
		t.Fatalf("API port %d accepted a connection on %s; it must be bound to 127.0.0.1 only", tr.APIPort(), addr)
	}
	t.Logf("dial %s:%d refused as expected: %v", addr, tr.APIPort(), err)

	var ep struct{ Address string }
	body, err := tr.APIGet("/api/entrypoints/traefik")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(body), &ep); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("127.0.0.1:%d", tr.APIPort()); ep.Address != want {
		t.Errorf("traefik entry point address = %q, want %q", ep.Address, want)
	}
}

// nonLoopbackAddr returns an IPv4 address of this machine that is not loopback.
func nonLoopbackAddr(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() || ipn.IP.To4() == nil {
			continue
		}
		return ipn.IP.String()
	}
	return ""
}

// Findings (b) and (c): with api.dashboard false the JSON API still answers, and the
// dashboard does not.
func TestAPIWorksWithDashboardOff(t *testing.T) {
	t.Parallel()
	tr := Start(t)
	for _, p := range []string{"/api/version", "/api/http/routers", "/api/http/services", "/api/entrypoints", "/api/rawdata"} {
		if _, err := tr.APIGet(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if body, err := tr.APIGet("/dashboard/"); err == nil {
		t.Errorf("dashboard is served with dashboard: false: %.80s", body)
	}
}

// Finding (a): an invalid file in the watched directory.
func TestInvalidDynamicFile(t *testing.T) {
	t.Parallel()
	up := upstream(t, "x")
	tr := Start(t)

	tr.WriteDynamic("a-good", routeYAML("good", "good.test", up.URL))
	tr.WaitForRouter("good@file", reloadTimeout)
	tr.WaitForStatus("good.test", "/", http.StatusOK, reloadTimeout)

	// A broken file: the previous configuration keeps serving.
	tr.WriteDynamic("z-bad", "http: [unclosed\n  routers: {")
	tr.WaitForLog("Error occurred during watcher callback", reloadTimeout)
	if status, _, err := tr.Get("good.test", "/"); err != nil || status != http.StatusOK {
		t.Errorf("after the bad file: status %d, err %v; the old config should keep serving", status, err)
	}
	if err := tr.poll(0, func() error {
		routers, err := tr.Routers()
		if err != nil {
			return err
		}
		if ok, seen := findRouter(routers, "good@file"); !ok {
			return fmt.Errorf("%s", seen)
		}
		return nil
	}); err != nil {
		t.Errorf("good router vanished from the API: %v", err)
	}

	// While the bad file stays, valid changes elsewhere are not applied: the
	// provider rebuilds the whole directory and gives up on the first error. Wait
	// longer than one 2 s throttle window so a reload would have happened.
	tr.WriteDynamic("b-new", routeYAML("fresh", "fresh.test", up.URL))
	time.Sleep(5 * time.Second)
	if status, _, _ := tr.Get("fresh.test", "/"); status != http.StatusNotFound {
		t.Errorf("fresh.test answered %d while a bad file is present; Traefik used to apply nothing", status)
	}

	// Removing the bad file lets the pending change through.
	tr.RemoveDynamic("z-bad")
	tr.WaitForRouter("fresh@file", reloadTimeout)
	tr.WaitForStatus("fresh.test", "/", http.StatusOK, reloadTimeout)
	tr.WaitForStatus("good.test", "/", http.StatusOK, reloadTimeout)
}

func TestWithStaticConfig(t *testing.T) {
	t.Parallel()
	var got Config
	tr := Start(t, WithStaticConfig(func(c Config) string {
		got = c
		return defaultStaticConfig(c)
	}))
	if got.DynamicDir != tr.DynamicDir() || got.APIPort != tr.APIPort() {
		t.Errorf("generator saw %+v, harness uses dir %s port %d", got, tr.DynamicDir(), tr.APIPort())
	}
}

// Traefik's paging, which listRouters relies on: X-Next-Page is the next page
// number, and wraps to 1 on the last page.
func TestRouterPagingHeaderOnRealTraefik(t *testing.T) {
	t.Parallel()
	up := upstream(t, "x")
	tr := Start(t)
	// Three routers in one file, written by hand to keep the YAML explicit.
	tr.WriteDynamic("many", fmt.Sprintf(`http:
  routers:
    r1: {rule: "Host(`+"`r1.test`"+`)", service: s}
    r2: {rule: "Host(`+"`r2.test`"+`)", service: s}
    r3: {rule: "Host(`+"`r3.test`"+`)", service: s}
  services:
    s:
      loadBalancer:
        servers: [{url: %q}]
`, up.URL))
	tr.WaitForRouter("r3@file", reloadTimeout)

	for _, c := range []struct{ query, wantNext string }{
		{"per_page=2&page=1", "2"},
		{"per_page=2&page=2", "1"},
	} {
		resp, err := http.Get(tr.APIURL() + "/api/http/routers?" + c.query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("X-Next-Page"); got != c.wantNext {
			t.Errorf("?%s: X-Next-Page = %q, want %q", c.query, got, c.wantNext)
		}
	}

	routers, err := tr.Routers()
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, r := range routers {
		if r.Provider == "file" {
			files = append(files, r.Name)
		}
	}
	if len(files) != 3 {
		t.Errorf("file routers = %v, want r1@file r2@file r3@file", files)
	}
}
