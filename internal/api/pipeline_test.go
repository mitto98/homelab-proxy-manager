package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mitto98/homelab-proxy-manager/internal/apply"
	"github.com/mitto98/homelab-proxy-manager/internal/render"
)

// Host writes go through the apply pipeline: they produce the Traefik files, and
// a pipeline failure stores nothing and changes no file.
func TestHostWritesGoThroughThePipeline(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	var reject atomic.Bool
	ts, st := newServerAt(t, data, apply.WithValidator(func(m render.Model) error {
		if reject.Load() {
			return errors.New("injected pipeline failure")
		}
		return m.Validate()
	}))
	c := signedIn(t, ts)
	panel := filepath.Join(data, "dynamic", "panel.yml")

	res := do(t, c, "POST", ts.URL+"/api/hosts", goodHost)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", res.StatusCode)
	}
	created := decodeHost(t, res)
	before, err := os.ReadFile(panel)
	if err != nil || !strings.Contains(string(before), "app.example.com") {
		t.Fatalf("panel.yml after create: %v\n%s", err, before)
	}

	reject.Store(true)
	for name, req := range map[string][2]string{
		"create": {"POST", ts.URL + "/api/hosts"},
		"update": {"PUT", ts.URL + "/api/hosts/" + strconv.FormatInt(created.Id, 10)},
	} {
		body := `{"domains":["other.example.com"],"upstreamScheme":"http","upstreamHost":"10.1.1.1","upstreamPort":81}`
		if got := do(t, c, req[0], req[1], body).StatusCode; got != http.StatusInternalServerError {
			t.Errorf("%s with a failing pipeline: status %d, want 500", name, got)
		}
	}
	if got := do(t, c, "DELETE", ts.URL+"/api/hosts/"+strconv.FormatInt(created.Id, 10), "").StatusCode; got != http.StatusInternalServerError {
		t.Errorf("delete with a failing pipeline: status %d, want 500", got)
	}

	hosts, err := st.ListHosts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Domains[0] != "app.example.com" || hosts[0].UpstreamPort != 8080 {
		t.Errorf("stored hosts after failed pipeline runs = %+v, want the original host unchanged", hosts)
	}
	if after, _ := os.ReadFile(panel); string(after) != string(before) {
		t.Error("panel.yml changed although the pipeline failed")
	}

	reject.Store(false)
	if got := do(t, c, "DELETE", ts.URL+"/api/hosts/"+strconv.FormatInt(created.Id, 10), "").StatusCode; got != http.StatusNoContent {
		t.Fatalf("delete: %d", got)
	}
	if after, _ := os.ReadFile(panel); strings.Contains(string(after), "app.example.com") {
		t.Errorf("deleted host still in panel.yml:\n%s", after)
	}
}
