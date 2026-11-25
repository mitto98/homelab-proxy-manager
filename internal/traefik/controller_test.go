package traefik

import (
	"context"
	"strings"
	"testing"
)

func TestFromName(t *testing.T) {
	for _, name := range []string{"", "none", " none "} {
		c, err := FromName(name)
		if err != nil {
			t.Fatalf("FromName(%q): %v", name, err)
		}
		if _, ok := c.(None); !ok {
			t.Errorf("FromName(%q) = %T, want None", name, c)
		}
		if err := c.Restart(context.Background()); err != nil {
			t.Errorf("None.Restart: %v", err)
		}
	}
	_, err := FromName("systemd")
	if err == nil || !strings.Contains(err.Error(), ControllerEnv) || !strings.Contains(err.Error(), `"systemd"`) {
		t.Errorf("FromName(unknown) = %v, want an error naming the variable and the value", err)
	}
}
