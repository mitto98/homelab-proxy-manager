package render

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func proxy(id int64, enabled bool, upstream string, port int, domains ...string) model.Host {
	scheme, host := "http", upstream
	if strings.HasPrefix(upstream, "https://") {
		scheme, host = "https", strings.TrimPrefix(upstream, "https://")
	}
	return model.Host{ID: id, Kind: model.KindProxy, Enabled: enabled, Domains: domains,
		UpstreamScheme: scheme, UpstreamHost: host, UpstreamPort: port}
}

func skipVerify(h model.Host) model.Host { h.UpstreamSkipTLSVerify = true; return h }

const testDir = "/data/dynamic"

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden.yml")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from golden file.\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

func TestDynamicGolden(t *testing.T) {
	many := []model.Host{}
	for i := int64(12); i >= 1; i-- { // deliberately unsorted input
		many = append(many, proxy(i, true, "10.0.0.5", 8000+int(i), fmt.Sprintf("h%d.example.com", i)))
	}
	cases := []struct {
		name  string
		hosts []model.Host
	}{
		{"one-host", []model.Host{proxy(1, true, "192.168.1.10", 8080, "app.example.com")}},
		{"several-domains", []model.Host{proxy(7, true, "nas.lan", 5000, "nas.example.com", "www.nas.example.com", "files.example.org")}},
		{"https-upstream-verified", []model.Host{proxy(2, true, "https://secure.lan", 443, "secure.example.com")}},
		{"https-upstream-skip-verify", []model.Host{skipVerify(proxy(3, true, "https://10.0.0.9", 8443, "unifi.example.com"))}},
		{"skip-verify-shared-transport", []model.Host{
			skipVerify(proxy(3, true, "https://10.0.0.9", 8443, "a.example.com")),
			skipVerify(proxy(4, true, "https://10.0.0.10", 8443, "b.example.com")),
			proxy(5, true, "https://ok.lan", 443, "c.example.com"),
		}},
		{"mixed-enabled-disabled", []model.Host{
			proxy(1, true, "10.0.0.1", 80, "on.example.com"),
			proxy(2, false, "10.0.0.2", 80, "off.example.com"),
			proxy(3, true, "10.0.0.3", 80, "on2.example.com"),
		}},
		{"disabled-skip-verify-emits-no-transport", []model.Host{
			skipVerify(proxy(1, false, "https://10.0.0.1", 443, "off.example.com")),
		}},
		{"no-enabled-hosts", []model.Host{proxy(1, false, "10.0.0.1", 80, "off.example.com")}},
		{"no-hosts", nil},
		{"ipv6-upstream", []model.Host{proxy(1, true, "2001:db8::1", 8080, "v6.example.com")}},
		{"many-hosts-ordered", many},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Build(Input{Settings: model.DefaultSettings(), Hosts: tc.hosts, DynamicDir: testDir})
			if err := m.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			b, err := m.DynamicYAML()
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "dynamic-"+tc.name, b)
		})
	}
}

func TestStaticGolden(t *testing.T) {
	for name, s := range map[string]model.Settings{
		"default": model.DefaultSettings(),
		"custom":  {HTTPPort: 8080, HTTPSPort: 8443, APIPort: 9100},
	} {
		t.Run(name, func(t *testing.T) {
			m := Build(Input{Settings: s, DynamicDir: testDir})
			b, err := m.StaticYAML()
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "static-"+name, b)
			// The golden file could be updated by mistake; this line cannot.
			if got := m.Static.EntryPoints[EntryPointAPI].Address; !strings.HasPrefix(got, "127.0.0.1:") {
				t.Errorf("API entrypoint address = %q, want 127.0.0.1:<port>", got)
			}
		})
	}
}

func TestOutputIsIndependentOfInputOrder(t *testing.T) {
	a := []model.Host{proxy(1, true, "h", 1, "a.test"), proxy(2, true, "h", 2, "b.test"), proxy(3, true, "h", 3, "c.test")}
	b := []model.Host{a[2], a[0], a[1]}
	render := func(h []model.Host) string {
		out, err := Build(Input{Settings: model.DefaultSettings(), Hosts: h, DynamicDir: testDir}).DynamicYAML()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if render(a) != render(b) {
		t.Error("output depends on the order of the input hosts")
	}
}

func TestValidate(t *testing.T) {
	good := func() Model {
		return Build(Input{Settings: model.DefaultSettings(), DynamicDir: testDir,
			Hosts: []model.Host{skipVerify(proxy(1, true, "https://h", 443, "a.test"))}})
	}
	if err := good().Validate(); err != nil {
		t.Fatalf("baseline is invalid: %v", err)
	}
	cases := []struct {
		name    string
		mutate  func(*Model)
		wantErr string
	}{
		{"empty rule", func(m *Model) {
			r := m.Dynamic.HTTP.Routers["host-1-http"]
			r.Rule = " "
			m.Dynamic.HTTP.Routers["host-1-http"] = r
		}, "empty rule"},
		{"missing service", func(m *Model) { delete(m.Dynamic.HTTP.Services, "host-1") }, "service"},
		{"missing transport", func(m *Model) { delete(m.Dynamic.HTTP.ServersTransports, SkipTLSVerifyTransport) }, "serversTransport"},
		{"no servers", func(m *Model) {
			m.Dynamic.HTTP.Services["host-1"] = Service{}
		}, "no servers"},
		{"no entry points", func(m *Model) {
			r := m.Dynamic.HTTP.Routers["host-1-http"]
			r.EntryPoints = nil
			m.Dynamic.HTTP.Routers["host-1-http"] = r
		}, "entry points"},
		{"relative dynamic dir", func(m *Model) { m.Static.Providers.File.Directory = "dynamic" }, "absolute"},
		{"duplicate router", func(m *Model) { m.duplicateRouters = []string{"host-1-http"} }, "more than once"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := good()
			tc.mutate(&m)
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

// An enabled host without domains would render an empty rule; Validate must
// stop it even though the API rejects such hosts earlier.
func TestValidateCatchesEnabledHostWithoutDomains(t *testing.T) {
	m := Build(Input{Settings: model.DefaultSettings(), DynamicDir: testDir,
		Hosts: []model.Host{proxy(1, true, "h", 80)}})
	if err := m.Validate(); err == nil {
		t.Error("an enabled host with no domains passed validation")
	}
}

func TestDuplicateHostIDsAreReported(t *testing.T) {
	m := Build(Input{Settings: model.DefaultSettings(), DynamicDir: testDir,
		Hosts: []model.Host{proxy(1, true, "h", 80, "a.test"), proxy(1, true, "h", 81, "b.test")}})
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("Validate() = %v, want duplicate router error", err)
	}
}
