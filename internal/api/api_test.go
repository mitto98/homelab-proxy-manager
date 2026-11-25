package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mitto98/homelab-proxy-manager/internal/apply"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
	"github.com/mitto98/homelab-proxy-manager/internal/traefik"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts, _ := newServerAt(t, filepath.Join(t.TempDir(), "data"))
	return ts
}

// newServerAt serves the API with a real Applier writing under dataDir, with the
// "none" controller. It also returns the store for assertions.
func newServerAt(t *testing.T, dataDir string, opts ...apply.Option) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	spa := fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}}
	ap, err := apply.New(st, traefik.None{}, dataDir, opts...)
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(st, ap, spa)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, st
}

func do(t *testing.T, c *http.Client, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func client() *http.Client {
	jar, _ := newJar()
	return &http.Client{Jar: jar}
}

func TestSetupLoginFlow(t *testing.T) {
	ts := newServer(t)
	c := client()

	var st struct{ SetupRequired bool }
	json.NewDecoder(do(t, c, "GET", ts.URL+"/api/status", "").Body).Decode(&st)
	if !st.SetupRequired {
		t.Fatal("fresh database should require setup")
	}
	if got := do(t, c, "GET", ts.URL+"/api/auth/me", "").StatusCode; got != 401 {
		t.Fatalf("me before login: %d", got)
	}
	if got := do(t, c, "POST", ts.URL+"/api/setup", `{"username":"admin","password":"short"}`).StatusCode; got != 400 {
		t.Fatalf("short password: %d", got)
	}
	if got := do(t, c, "POST", ts.URL+"/api/setup", `{"username":"admin","password":"correct horse"}`).StatusCode; got != 201 {
		t.Fatalf("setup: %d", got)
	}
	if got := do(t, c, "GET", ts.URL+"/api/auth/me", "").StatusCode; got != 200 {
		t.Fatalf("me after setup (session cookie): %d", got)
	}
	if got := do(t, c, "POST", ts.URL+"/api/setup", `{"username":"x","password":"correct horse"}`).StatusCode; got != 409 {
		t.Fatalf("second setup should conflict: %d", got)
	}

	other := client()
	if got := do(t, other, "POST", ts.URL+"/api/auth/login", `{"username":"admin","password":"wrong"}`).StatusCode; got != 401 {
		t.Fatalf("bad password: %d", got)
	}
	if got := do(t, other, "POST", ts.URL+"/api/auth/login", `{"username":"nobody","password":"x"}`).StatusCode; got != 401 {
		t.Fatalf("unknown user: %d", got)
	}
	if got := do(t, other, "POST", ts.URL+"/api/auth/login", `{"username":"ADMIN","password":"correct horse"}`).StatusCode; got != 200 {
		t.Fatalf("login (case-insensitive username): %d", got)
	}
	if got := do(t, other, "POST", ts.URL+"/api/auth/logout", `{}`).StatusCode; got != 204 {
		t.Fatalf("logout: %d", got)
	}
	if got := do(t, other, "GET", ts.URL+"/api/auth/me", "").StatusCode; got != 401 {
		t.Fatalf("me after logout: %d", got)
	}
}

func TestRejectsNonJSONWrites(t *testing.T) {
	ts := newServer(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader("username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 415 {
		t.Fatalf("form post should be rejected, got %d", res.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	ts := newServer(t)
	res := do(t, http.DefaultClient, "GET", ts.URL+"/hosts/12", "")
	if res.StatusCode != 200 {
		t.Fatalf("spa fallback: %d", res.StatusCode)
	}
	if got := do(t, http.DefaultClient, "GET", ts.URL+"/api/nope", "").StatusCode; got != 404 {
		t.Fatalf("unknown api route: %d", got)
	}
}
