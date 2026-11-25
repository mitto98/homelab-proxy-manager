// Package traefik holds the Panel's view of the Traefik process: how to make it
// pick up a changed static configuration. Later stages add the Traefik API
// client and the acme.json reader here.
package traefik

import (
	"context"
	"fmt"
	"strings"
)

// ControllerEnv names the environment variable that selects the controller.
const ControllerEnv = "HPM_TRAEFIK_CONTROLLER"

// Controller makes Traefik pick up a changed static configuration.
//
// Dynamic config needs no controller: Traefik's file provider watches the
// dynamic directory and reloads by itself. Only static config (entrypoints,
// providers, API) needs a restart.
type Controller interface {
	// Restart is called when the content of the static config changed.
	Restart(ctx context.Context) error
}

// None does nothing. It is the default, used in development and tests, and
// for the systemd packaging where a path unit watches traefik.yml and restarts
// Traefik itself, so the Panel needs no privileges.
type None struct{}

// Restart does nothing.
func (None) Restart(context.Context) error { return nil }

// FromName returns the controller called name; the empty name selects "none".
// Unknown names are an error so a typo in the environment cannot silently turn
// restarts off.
func FromName(name string) (Controller, error) {
	switch strings.TrimSpace(name) {
	case "", "none":
		return None{}, nil
	// Slice 2.8 adds `case "s6":` here, restarting the Traefik service with s6-svc.
	default:
		return nil, fmt.Errorf("unknown %s %q: supported values are \"none\"", ControllerEnv, name)
	}
}
