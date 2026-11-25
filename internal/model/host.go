// Package model holds the Panel's domain types and their validation rules.
package model

import (
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Host kinds.
const KindProxy = "proxy"

// Upstream schemes.
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// Host is a Proxy Host: one or more public domain names forwarded to an
// upstream. Redirection Hosts (stage 5) share the storage but are a separate kind.
type Host struct {
	ID      int64
	Kind    string
	Enabled bool
	// Domains are public names, lower-case, in the order the user gave them.
	Domains               []string
	UpstreamScheme        string
	UpstreamHost          string
	UpstreamPort          int
	UpstreamSkipTLSVerify bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ValidationError describes why a value was rejected. Field is the name of the
// offending field in the HTTP API ("domains", "upstreamHost", ...), so clients
// can show the message next to the right input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// Normalize puts a host into its canonical form: surrounding space trimmed,
// names lower-cased, and the TLS verification flag cleared where it has no meaning.
func (h *Host) Normalize() {
	for i, d := range h.Domains {
		h.Domains[i] = strings.ToLower(strings.TrimSpace(d))
	}
	h.UpstreamScheme = strings.ToLower(strings.TrimSpace(h.UpstreamScheme))
	h.UpstreamHost = strings.ToLower(strings.TrimSpace(h.UpstreamHost))
	// Skipping certificate verification only means something for https upstreams.
	if h.UpstreamScheme != SchemeHTTPS {
		h.UpstreamSkipTLSVerify = false
	}
}

// Validate checks a normalized Proxy Host and returns a *ValidationError for
// the first problem found.
func (h *Host) Validate() error {
	if h.Kind != KindProxy {
		return invalid("kind", "only %q hosts can be created here", KindProxy)
	}
	if len(h.Domains) == 0 {
		return invalid("domains", "at least one domain is required")
	}
	seen := make(map[string]bool, len(h.Domains))
	for _, d := range h.Domains {
		if err := validateDomain(d); err != nil {
			return err
		}
		if seen[d] {
			return invalid("domains", "domain %q is listed twice", d)
		}
		seen[d] = true
	}
	if h.UpstreamScheme != SchemeHTTP && h.UpstreamScheme != SchemeHTTPS {
		return invalid("upstreamScheme", "upstream scheme must be %q or %q", SchemeHTTP, SchemeHTTPS)
	}
	if err := validateUpstreamHost(h.UpstreamHost); err != nil {
		return err
	}
	if h.UpstreamPort < 1 || h.UpstreamPort > 65535 {
		return invalid("upstreamPort", "upstream port must be between 1 and 65535")
	}
	return nil
}

func validateDomain(d string) *ValidationError {
	if d == "" {
		return invalid("domains", "a domain must not be empty")
	}
	if strings.HasPrefix(d, "*.") {
		// Wildcards need the DNS challenge; they arrive with stage 4.
		return invalid("domains", "wildcard domain %q is not supported yet", d)
	}
	if !validHostname(d, false) {
		return invalid("domains", "%q is not a valid domain name", d)
	}
	return nil
}

// validateUpstreamHost accepts an IP address or a hostname. Underscores are
// allowed in upstream hostnames because Docker service and container names use them.
func validateUpstreamHost(h string) *ValidationError {
	if h == "" {
		return invalid("upstreamHost", "upstream host is required")
	}
	if a, err := netip.ParseAddr(h); err == nil {
		if a.Zone() != "" {
			return invalid("upstreamHost", "%q must not carry an IPv6 zone", h)
		}
		return nil
	}
	if !validHostname(h, true) {
		return invalid("upstreamHost", "%q is not a valid host name or IP address", h)
	}
	return nil
}

// validHostname reports whether s is a valid RFC 1123 host name: dot-separated
// labels of 1 to 63 letters, digits and hyphens, not starting or ending with a
// hyphen, at most 253 characters, and a last label that is not all digits (so
// that things like "999.1.1.1" are not accepted as names). s must already be
// lower-case. A single label such as "nas" is allowed for home networks.
func validHostname(s string, allowUnderscore bool) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	labels := strings.Split(s, ".")
	for _, l := range labels {
		if len(l) == 0 || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || (allowUnderscore && c == '_')
			if !ok {
				return false
			}
		}
	}
	last := labels[len(labels)-1]
	for i := 0; i < len(last); i++ {
		if last[i] < '0' || last[i] > '9' {
			return true
		}
	}
	return false
}
