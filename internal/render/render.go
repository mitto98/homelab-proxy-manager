// Package render turns the stored model into Traefik's files. Every function is
// pure: the same input gives the same bytes, nothing touches the disk, and no
// Traefik has to run, so golden files can pin the output down.
//
// The pipeline in internal/apply calls [Build], then [Model.Validate], then the
// YAML methods, so an inconsistent model is caught before any file is written.
// One invalid YAML file in Traefik's watched directory freezes every change
// (see internal/traefiktest), which is why validation is not optional.
package render

import (
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	// YAML is marshalled by a library rather than formatted by hand: domains and
	// hostnames come from users and must be quoted correctly. go.yaml.in/yaml/v4
	// is the maintained successor of gopkg.in/yaml.v3 and was already in the
	// module graph through the OpenAPI tooling. Struct fields marshal in
	// declaration order and map keys sorted, which makes the output deterministic.
	"go.yaml.in/yaml/v4"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
)

// Names fixed by the design (docs/architecture.md section 7.3).
const (
	EntryPointWeb       = "web"
	EntryPointWebsecure = "websecure"
	EntryPointAPI       = "traefik"
	// SkipTLSVerifyTransport is the serversTransport for upstreams whose
	// certificate is not verified.
	SkipTLSVerifyTransport = "skip-tls-verify"
)

// Input is everything the renderer needs.
type Input struct {
	Settings model.Settings
	Hosts    []model.Host
	// DynamicDir is the absolute path of the directory Traefik's file provider
	// watches. It is an input so the renderer stays free of I/O and of the
	// Panel's data directory layout.
	DynamicDir string
}

// Model is the rendered configuration as typed values, before it becomes YAML.
type Model struct {
	Dynamic Dynamic
	Static  Static

	// duplicateRouters records router names produced twice. The maps cannot hold
	// both, so Build notes it here and Validate reports it.
	duplicateRouters []string
}

// Dynamic is the content of dynamic/panel.yml.
type Dynamic struct {
	HTTP HTTP `yaml:"http"`
}

// HTTP is Traefik's "http" section. Empty maps are omitted, so a Panel with no
// enabled host renders "http: {}", which Traefik accepts.
type HTTP struct {
	Routers           map[string]Router    `yaml:"routers,omitempty"`
	Services          map[string]Service   `yaml:"services,omitempty"`
	ServersTransports map[string]Transport `yaml:"serversTransports,omitempty"`
}

// Router routes requests matching Rule to Service.
type Router struct {
	Rule        string   `yaml:"rule"`
	EntryPoints []string `yaml:"entryPoints"`
	Service     string   `yaml:"service"`
}

// Service is a load balancer; v1 has exactly one server per service.
type Service struct {
	LoadBalancer LoadBalancer `yaml:"loadBalancer"`
}

// LoadBalancer lists the upstream servers.
type LoadBalancer struct {
	Servers          []Server `yaml:"servers"`
	ServersTransport string   `yaml:"serversTransport,omitempty"`
}

// Server is one upstream.
type Server struct {
	URL string `yaml:"url"`
}

// Transport configures how Traefik connects to upstreams.
type Transport struct {
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

// Static is the content of traefik.yml.
type Static struct {
	Global      Global                `yaml:"global"`
	EntryPoints map[string]EntryPoint `yaml:"entryPoints"`
	Providers   Providers             `yaml:"providers"`
	API         API                   `yaml:"api"`
}

// Global switches off Traefik's network calls home.
type Global struct {
	CheckNewVersion    bool `yaml:"checkNewVersion"`
	SendAnonymousUsage bool `yaml:"sendAnonymousUsage"`
}

// EntryPoint is a listening address.
type EntryPoint struct {
	Address string `yaml:"address"`
}

// Providers holds the file provider.
type Providers struct {
	File FileProvider `yaml:"file"`
}

// FileProvider watches a directory of dynamic config.
type FileProvider struct {
	Directory string `yaml:"directory"`
	Watch     bool   `yaml:"watch"`
}

// API is Traefik's API. The dashboard is off; the JSON API stays on the
// loopback-only entrypoint.
type API struct {
	Insecure  bool `yaml:"insecure"`
	Dashboard bool `yaml:"dashboard"`
}

// Names derived from IDs only, never from user input.
func routerName(id int64) string  { return "host-" + strconv.FormatInt(id, 10) + "-http" }
func serviceName(id int64) string { return "host-" + strconv.FormatInt(id, 10) }

// Build maps the model to Traefik's configuration. It does not validate; call
// [Model.Validate] before writing anything.
func Build(in Input) Model {
	hosts := slices.Clone(in.Hosts)
	slices.SortFunc(hosts, func(a, b model.Host) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})

	var http HTTP
	var dups []string
	for _, h := range hosts {
		// Disabled hosts are left out entirely; so are kinds the renderer does not
		// know yet (Redirection Hosts arrive in stage 5).
		if !h.Enabled || h.Kind != model.KindProxy {
			continue
		}
		if http.Routers == nil {
			http.Routers = map[string]Router{}
			http.Services = map[string]Service{}
		}
		rules := make([]string, len(h.Domains))
		for i, d := range h.Domains {
			rules[i] = "Host(`" + d + "`)"
		}
		if _, dup := http.Routers[routerName(h.ID)]; dup {
			dups = append(dups, routerName(h.ID))
		}
		http.Routers[routerName(h.ID)] = Router{
			Rule:        strings.Join(rules, " || "),
			EntryPoints: []string{EntryPointWeb},
			Service:     serviceName(h.ID),
		}
		lb := LoadBalancer{Servers: []Server{{URL: upstreamURL(h)}}}
		if h.UpstreamSkipTLSVerify {
			lb.ServersTransport = SkipTLSVerifyTransport
			if http.ServersTransports == nil {
				http.ServersTransports = map[string]Transport{}
			}
			http.ServersTransports[SkipTLSVerifyTransport] = Transport{InsecureSkipVerify: true}
		}
		http.Services[serviceName(h.ID)] = Service{LoadBalancer: lb}
	}

	return Model{
		duplicateRouters: dups,
		Dynamic:          Dynamic{HTTP: http},
		Static: Static{
			Global: Global{},
			EntryPoints: map[string]EntryPoint{
				EntryPointWeb:       {Address: ":" + strconv.Itoa(in.Settings.HTTPPort)},
				EntryPointWebsecure: {Address: ":" + strconv.Itoa(in.Settings.HTTPSPort)},
				// Loopback only. Without an explicit 127.0.0.1 here Traefik would
				// serve its unauthenticated API on every interface.
				EntryPointAPI: {Address: net.JoinHostPort("127.0.0.1", strconv.Itoa(in.Settings.APIPort))},
			},
			Providers: Providers{File: FileProvider{Directory: in.DynamicDir, Watch: true}},
			API:       API{Insecure: true, Dashboard: false},
		},
	}
}

// upstreamURL joins scheme, host and port; JoinHostPort brackets IPv6 literals.
func upstreamURL(h model.Host) string {
	return h.UpstreamScheme + "://" + net.JoinHostPort(h.UpstreamHost, strconv.Itoa(h.UpstreamPort))
}

// Validate checks that the rendered model is consistent: router names are
// unique (they are map keys, so a duplicate can only come from two hosts with
// one ID, which Build flags here), every router has a rule and an existing
// service, every referenced serversTransport exists, and the static config
// points at an absolute directory. The first problem is returned.
func (m Model) Validate() error {
	h := m.Dynamic.HTTP
	if len(m.duplicateRouters) > 0 {
		return fmt.Errorf("router %q is defined more than once", m.duplicateRouters[0])
	}
	for _, name := range sortedKeys(h.Routers) {
		r := h.Routers[name]
		if strings.TrimSpace(r.Rule) == "" {
			return fmt.Errorf("router %q has an empty rule", name)
		}
		if _, ok := h.Services[r.Service]; !ok {
			return fmt.Errorf("router %q refers to service %q, which does not exist", name, r.Service)
		}
		if len(r.EntryPoints) == 0 {
			return fmt.Errorf("router %q has no entry points", name)
		}
	}
	for _, name := range sortedKeys(h.Services) {
		lb := h.Services[name].LoadBalancer
		if len(lb.Servers) == 0 {
			return fmt.Errorf("service %q has no servers", name)
		}
		if lb.ServersTransport != "" {
			if _, ok := h.ServersTransports[lb.ServersTransport]; !ok {
				return fmt.Errorf("service %q refers to serversTransport %q, which does not exist", name, lb.ServersTransport)
			}
		}
	}
	if !filepath.IsAbs(m.Static.Providers.File.Directory) {
		return fmt.Errorf("file provider directory %q is not an absolute path", m.Static.Providers.File.Directory)
	}
	return nil
}

// DynamicYAML is the bytes of dynamic/panel.yml.
func (m Model) DynamicYAML() ([]byte, error) { return marshal(m.Dynamic) }

// StaticYAML is the bytes of traefik.yml.
func (m Model) StaticYAML() ([]byte, error) { return marshal(m.Static) }

func marshal(v any) ([]byte, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return b, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
