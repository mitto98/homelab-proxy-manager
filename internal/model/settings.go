package model

// Default entrypoint ports, matching the settings migration. The Traefik API
// port avoids 8080, which upstreams commonly use.
const (
	DefaultHTTPPort  = 80
	DefaultHTTPSPort = 443
	DefaultAPIPort   = 8899
)

// Settings are the instance-wide options. Later stages add fields.
type Settings struct {
	// HTTPPort and HTTPSPort are the Traefik entrypoints "web" and "websecure".
	HTTPPort  int
	HTTPSPort int
	// APIPort is the loopback-only port of Traefik's API.
	APIPort int
}

// DefaultSettings returns the values the migration seeds.
func DefaultSettings() Settings {
	return Settings{HTTPPort: DefaultHTTPPort, HTTPSPort: DefaultHTTPSPort, APIPort: DefaultAPIPort}
}
