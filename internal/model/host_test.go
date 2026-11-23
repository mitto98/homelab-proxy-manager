package model

import (
	"errors"
	"strings"
	"testing"
)

func proxy() Host {
	return Host{
		Kind: KindProxy, Enabled: true,
		Domains:        []string{"app.example.com"},
		UpstreamScheme: SchemeHTTP, UpstreamHost: "192.168.1.20", UpstreamPort: 8080,
	}
}

func TestNormalize(t *testing.T) {
	h := proxy()
	h.Domains = []string{"  App.Example.COM ", "WWW.example.com"}
	h.UpstreamScheme = " HTTPS "
	h.UpstreamHost = " NAS.Lan "
	h.UpstreamSkipTLSVerify = true
	h.Normalize()
	if got := strings.Join(h.Domains, ","); got != "app.example.com,www.example.com" {
		t.Errorf("domains: %q", got)
	}
	if h.UpstreamScheme != "https" || h.UpstreamHost != "nas.lan" || !h.UpstreamSkipTLSVerify {
		t.Errorf("upstream not normalized: %+v", h)
	}

	h.UpstreamScheme = "http"
	h.Normalize()
	if h.UpstreamSkipTLSVerify {
		t.Error("skip TLS verification must be cleared for an http upstream")
	}
}

func TestValidateAcceptsGoodHosts(t *testing.T) {
	for name, mutate := range map[string]func(*Host){
		"plain":              func(h *Host) {},
		"https upstream":     func(h *Host) { h.UpstreamScheme = SchemeHTTPS; h.UpstreamSkipTLSVerify = true },
		"disabled":           func(h *Host) { h.Enabled = false },
		"several domains":    func(h *Host) { h.Domains = []string{"a.example.com", "b.example.com", "example.org"} },
		"single label":       func(h *Host) { h.Domains = []string{"nas"} },
		"hyphens and digits": func(h *Host) { h.Domains = []string{"my-app2.home-lab.example.com"} },
		"punycode":           func(h *Host) { h.Domains = []string{"xn--bcher-kva.example"} },
		"hostname upstream":  func(h *Host) { h.UpstreamHost = "proxmox.lan" },
		"docker upstream":    func(h *Host) { h.UpstreamHost = "my_app_1" },
		"ipv6 upstream":      func(h *Host) { h.UpstreamHost = "fd00::1" },
		"port 1":             func(h *Host) { h.UpstreamPort = 1 },
		"port 65535":         func(h *Host) { h.UpstreamPort = 65535 },
		"63 char label":      func(h *Host) { h.Domains = []string{strings.Repeat("a", 63) + ".example.com"} },
	} {
		t.Run(name, func(t *testing.T) {
			h := proxy()
			mutate(&h)
			h.Normalize()
			if err := h.Validate(); err != nil {
				t.Fatalf("want valid, got %v", err)
			}
		})
	}
}

func TestValidateRejectsBadHosts(t *testing.T) {
	long := strings.Repeat("a", 64)
	var tooLong []string
	for i := 0; i < 5; i++ {
		tooLong = append(tooLong, strings.Repeat("a", 60))
	}
	for name, tc := range map[string]struct {
		mutate func(*Host)
		field  string
	}{
		"no domains":            {func(h *Host) { h.Domains = nil }, "domains"},
		"empty domain":          {func(h *Host) { h.Domains = []string{""} }, "domains"},
		"blank domain":          {func(h *Host) { h.Domains = []string{"   "} }, "domains"},
		"duplicate domain":      {func(h *Host) { h.Domains = []string{"a.example.com", "A.example.com"} }, "domains"},
		"wildcard (stage 4)":    {func(h *Host) { h.Domains = []string{"*.example.com"} }, "domains"},
		"underscore in domain":  {func(h *Host) { h.Domains = []string{"my_app.example.com"} }, "domains"},
		"url as domain":         {func(h *Host) { h.Domains = []string{"https://example.com"} }, "domains"},
		"domain with port":      {func(h *Host) { h.Domains = []string{"example.com:8080"} }, "domains"},
		"domain with path":      {func(h *Host) { h.Domains = []string{"example.com/app"} }, "domains"},
		"domain with space":     {func(h *Host) { h.Domains = []string{"my app.example.com"} }, "domains"},
		"leading hyphen":        {func(h *Host) { h.Domains = []string{"-a.example.com"} }, "domains"},
		"trailing hyphen":       {func(h *Host) { h.Domains = []string{"a-.example.com"} }, "domains"},
		"trailing dot":          {func(h *Host) { h.Domains = []string{"example.com."} }, "domains"},
		"empty label":           {func(h *Host) { h.Domains = []string{"a..example.com"} }, "domains"},
		"label over 63":         {func(h *Host) { h.Domains = []string{long + ".example.com"} }, "domains"},
		"name over 253":         {func(h *Host) { h.Domains = []string{strings.Join(tooLong, ".")} }, "domains"},
		"non-ascii":             {func(h *Host) { h.Domains = []string{"bücher.example"} }, "domains"},
		"ip as domain":          {func(h *Host) { h.Domains = []string{"192.168.1.5"} }, "domains"},
		"numeric top label":     {func(h *Host) { h.Domains = []string{"example.123"} }, "domains"},
		"bad scheme":            {func(h *Host) { h.UpstreamScheme = "ftp" }, "upstreamScheme"},
		"empty scheme":          {func(h *Host) { h.UpstreamScheme = "" }, "upstreamScheme"},
		"no upstream host":      {func(h *Host) { h.UpstreamHost = "" }, "upstreamHost"},
		"upstream with scheme":  {func(h *Host) { h.UpstreamHost = "http://nas" }, "upstreamHost"},
		"upstream with port":    {func(h *Host) { h.UpstreamHost = "nas:8080" }, "upstreamHost"},
		"upstream with path":    {func(h *Host) { h.UpstreamHost = "nas/app" }, "upstreamHost"},
		"bracketed ipv6":        {func(h *Host) { h.UpstreamHost = "[fd00::1]" }, "upstreamHost"},
		"ipv6 zone":             {func(h *Host) { h.UpstreamHost = "fe80::1%eth0" }, "upstreamHost"},
		"invalid ipv4":          {func(h *Host) { h.UpstreamHost = "999.1.1.1" }, "upstreamHost"},
		"port 0":                {func(h *Host) { h.UpstreamPort = 0 }, "upstreamPort"},
		"port negative":         {func(h *Host) { h.UpstreamPort = -1 }, "upstreamPort"},
		"port 65536":            {func(h *Host) { h.UpstreamPort = 65536 }, "upstreamPort"},
		"redirect kind refused": {func(h *Host) { h.Kind = "redirect" }, "kind"},
	} {
		t.Run(name, func(t *testing.T) {
			h := proxy()
			tc.mutate(&h)
			h.Normalize()
			err := h.Validate()
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want *ValidationError, got %v", err)
			}
			if ve.Field != tc.field {
				t.Errorf("field %q, want %q (message %q)", ve.Field, tc.field, ve.Message)
			}
			if ve.Message == "" {
				t.Error("message must not be empty")
			}
		})
	}
}
