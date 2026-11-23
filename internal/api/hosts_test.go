package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// signedIn returns a client holding an admin session.
func signedIn(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	c := client()
	if got := do(t, c, "POST", ts.URL+"/api/setup", `{"username":"admin","password":"correct horse"}`).StatusCode; got != 201 {
		t.Fatalf("setup: %d", got)
	}
	return c
}

func decodeHost(t *testing.T, res *http.Response) Host {
	t.Helper()
	var h Host
	if err := json.NewDecoder(res.Body).Decode(&h); err != nil {
		t.Fatalf("decode host: %v", err)
	}
	return h
}

func decodeError(t *testing.T, res *http.Response) Error {
	t.Helper()
	var e Error
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil || e.Error == "" {
		t.Fatalf("want a JSON error, got err=%v body=%+v", err, e)
	}
	return e
}

const goodHost = `{"domains":["App.Example.com","www.example.com"],"upstreamScheme":"http","upstreamHost":"192.168.1.20","upstreamPort":8080}`

func TestHostsCRUD(t *testing.T) {
	ts := newServer(t)
	c := signedIn(t, ts)

	res := do(t, c, "GET", ts.URL+"/api/hosts", "")
	if res.StatusCode != 200 {
		t.Fatalf("list: %d", res.StatusCode)
	}
	var empty []Host
	if err := json.NewDecoder(res.Body).Decode(&empty); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("an empty list must be [] (not null), got %v (%v)", empty, err)
	}

	res = do(t, c, "POST", ts.URL+"/api/hosts", goodHost)
	if res.StatusCode != 201 {
		t.Fatalf("create: %d", res.StatusCode)
	}
	created := decodeHost(t, res)
	want := Host{
		Id: created.Id, Kind: "proxy", Enabled: true,
		Domains:        []string{"app.example.com", "www.example.com"}, // lower-cased, order kept
		UpstreamScheme: "http", UpstreamHost: "192.168.1.20", UpstreamPort: 8080,
		UpstreamSkipTlsVerify: false, CreatedAt: created.CreatedAt, UpdatedAt: created.UpdatedAt,
	}
	if created.Id == 0 || !reflect.DeepEqual(created, want) {
		t.Fatalf("created host:\n got  %+v\n want %+v", created, want)
	}
	if time.Since(created.CreatedAt) > time.Minute || created.CreatedAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("createdAt looks wrong: %v", created.CreatedAt)
	}

	url := fmt.Sprintf("%s/api/hosts/%d", ts.URL, created.Id)
	if got := decodeHost(t, do(t, c, "GET", url, "")); !reflect.DeepEqual(got, created) {
		t.Errorf("get differs from create:\n got  %+v\n want %+v", got, created)
	}

	res = do(t, c, "PUT", url, `{"domains":["www.example.com","new.example.com"],"upstreamScheme":"https","upstreamHost":"nas.lan","upstreamPort":8443,"upstreamSkipTlsVerify":true,"enabled":false}`)
	if res.StatusCode != 200 {
		t.Fatalf("update: %d", res.StatusCode)
	}
	updated := decodeHost(t, res)
	if !reflect.DeepEqual(updated.Domains, []string{"www.example.com", "new.example.com"}) ||
		updated.UpstreamScheme != "https" || updated.UpstreamHost != "nas.lan" || updated.UpstreamPort != 8443 ||
		!updated.UpstreamSkipTlsVerify || updated.Enabled || updated.Id != created.Id ||
		!updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("update not applied: %+v", updated)
	}

	var list []Host
	json.NewDecoder(do(t, c, "GET", ts.URL+"/api/hosts", "").Body).Decode(&list)
	if len(list) != 1 || !reflect.DeepEqual(list[0], updated) {
		t.Fatalf("list after update: %+v", list)
	}

	if got := do(t, c, "DELETE", url, "").StatusCode; got != 204 {
		t.Fatalf("delete: %d", got)
	}
	if got := do(t, c, "GET", url, "").StatusCode; got != 404 {
		t.Fatalf("get after delete: %d", got)
	}
	if got := do(t, c, "DELETE", url, "").StatusCode; got != 404 {
		t.Fatalf("second delete: %d", got)
	}
}

func TestHostDefaultsAndNormalization(t *testing.T) {
	ts := newServer(t)
	c := signedIn(t, ts)

	// Optional fields default to enabled = true, skip verify = false.
	h := decodeHost(t, do(t, c, "POST", ts.URL+"/api/hosts",
		`{"domains":["a.example.com"],"upstreamScheme":"http","upstreamHost":"NAS.LAN","upstreamPort":80}`))
	if !h.Enabled || h.UpstreamSkipTlsVerify || h.UpstreamHost != "nas.lan" {
		t.Errorf("defaults/normalization: %+v", h)
	}
	// Skip-verify has no meaning for an http upstream and is cleared.
	h = decodeHost(t, do(t, c, "POST", ts.URL+"/api/hosts",
		`{"domains":["b.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80,"upstreamSkipTlsVerify":true}`))
	if h.UpstreamSkipTlsVerify {
		t.Error("skip TLS verification must be cleared for an http upstream")
	}
}

func TestHostValidationErrors(t *testing.T) {
	ts := newServer(t)
	c := signedIn(t, ts)
	for name, tc := range map[string]struct{ body, field string }{
		"no domains":      {`{"domains":[],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`, "domains"},
		"missing domains": {`{"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`, "domains"},
		"bad domain":      {`{"domains":["not a domain"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`, "domains"},
		"wildcard":        {`{"domains":["*.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`, "domains"},
		"duplicate":       {`{"domains":["a.example.com","A.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`, "domains"},
		"missing host":    {`{"domains":["a.example.com"],"upstreamScheme":"http","upstreamPort":80}`, "upstreamHost"},
		"url as host":     {`{"domains":["a.example.com"],"upstreamScheme":"http","upstreamHost":"http://nas","upstreamPort":80}`, "upstreamHost"},
		"missing port":    {`{"domains":["a.example.com"],"upstreamScheme":"http","upstreamHost":"nas"}`, "upstreamPort"},
		"port too high":   {`{"domains":["a.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":70000}`, "upstreamPort"},
		"missing scheme":  {`{"domains":["a.example.com"],"upstreamHost":"nas","upstreamPort":80}`, "upstreamScheme"},
		"bad scheme":      {`{"domains":["a.example.com"],"upstreamScheme":"ftp","upstreamHost":"nas","upstreamPort":80}`, "upstreamScheme"},
	} {
		for _, call := range []struct{ method, path string }{{"POST", "/api/hosts"}, {"PUT", "/api/hosts/1"}} {
			t.Run(name+" "+call.method, func(t *testing.T) {
				res := do(t, c, call.method, ts.URL+call.path, tc.body)
				if res.StatusCode != 400 {
					t.Fatalf("status %d, want 400", res.StatusCode)
				}
				e := decodeError(t, res)
				if e.Field == nil || *e.Field != tc.field {
					t.Errorf("field %v, want %q (message %q)", e.Field, tc.field, e.Error)
				}
			})
		}
	}

	// Bodies the decoder itself rejects have no field.
	for name, body := range map[string]string{
		"port as string": `{"domains":["a.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":"80"}`,
		"domains string": `{"domains":"a.example.com","upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`,
		"not JSON":       `domains=a.example.com`,
	} {
		t.Run(name, func(t *testing.T) {
			res := do(t, c, "POST", ts.URL+"/api/hosts", body)
			if res.StatusCode != 400 {
				t.Fatalf("status %d, want 400", res.StatusCode)
			}
			decodeError(t, res)
		})
	}

	if hosts := listHosts(t, c, ts); len(hosts) != 0 {
		t.Fatalf("rejected requests must store nothing, found %d hosts", len(hosts))
	}
}

func listHosts(t *testing.T, c *http.Client, ts *httptest.Server) []Host {
	t.Helper()
	var hosts []Host
	if err := json.NewDecoder(do(t, c, "GET", ts.URL+"/api/hosts", "").Body).Decode(&hosts); err != nil {
		t.Fatal(err)
	}
	return hosts
}

func TestHostDomainConflicts(t *testing.T) {
	ts := newServer(t)
	c := signedIn(t, ts)
	first := decodeHost(t, do(t, c, "POST", ts.URL+"/api/hosts", goodHost))

	// Case-insensitive, and the message names the domain and the field.
	res := do(t, c, "POST", ts.URL+"/api/hosts",
		`{"domains":["free.example.com","WWW.EXAMPLE.COM"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`)
	if res.StatusCode != 409 {
		t.Fatalf("duplicate domain: %d", res.StatusCode)
	}
	e := decodeError(t, res)
	if !strings.Contains(e.Error, "www.example.com") || e.Field == nil || *e.Field != "domains" {
		t.Errorf("conflict must name the domain and field: %+v", e)
	}
	if hosts := listHosts(t, c, ts); len(hosts) != 1 {
		t.Fatalf("a refused create must store nothing: %d hosts", len(hosts))
	}

	second := decodeHost(t, do(t, c, "POST", ts.URL+"/api/hosts",
		`{"domains":["second.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`))
	url := fmt.Sprintf("%s/api/hosts/%d", ts.URL, second.Id)
	if got := do(t, c, "PUT", url,
		`{"domains":["second.example.com","app.example.com"],"upstreamScheme":"http","upstreamHost":"nas","upstreamPort":80}`).StatusCode; got != 409 {
		t.Fatalf("update onto a taken domain: %d", got)
	}
	// A host can keep its own domains.
	if got := do(t, c, "PUT", fmt.Sprintf("%s/api/hosts/%d", ts.URL, first.Id), goodHost).StatusCode; got != 200 {
		t.Fatalf("re-saving a host unchanged: %d", got)
	}
}

func TestHostNotFound(t *testing.T) {
	ts := newServer(t)
	c := signedIn(t, ts)
	for _, id := range []string{"999", "0", "-3"} {
		url := ts.URL + "/api/hosts/" + id
		for method, body := range map[string]string{"GET": "", "DELETE": "", "PUT": goodHost} {
			if got := do(t, c, method, url, body).StatusCode; got != 404 {
				t.Errorf("%s %s: %d, want 404", method, url, got)
			}
		}
	}
	res := do(t, c, "GET", ts.URL+"/api/hosts/abc", "")
	if res.StatusCode != 400 {
		t.Errorf("a non-numeric id: %d, want 400", res.StatusCode)
	}
	decodeError(t, res)
}

func TestHostsNeedASession(t *testing.T) {
	ts := newServer(t)
	for _, call := range []struct{ method, path, body string }{
		{"GET", "/api/hosts", ""}, {"POST", "/api/hosts", goodHost},
		{"GET", "/api/hosts/1", ""}, {"PUT", "/api/hosts/1", goodHost}, {"DELETE", "/api/hosts/1", ""},
	} {
		if got := do(t, http.DefaultClient, call.method, ts.URL+call.path, call.body).StatusCode; got != 401 {
			t.Errorf("%s %s without a session: %d, want 401", call.method, call.path, got)
		}
	}
	// After signing out the session no longer works.
	c := signedIn(t, ts)
	do(t, c, "POST", ts.URL+"/api/auth/logout", `{}`)
	if got := do(t, c, "GET", ts.URL+"/api/hosts", "").StatusCode; got != 401 {
		t.Errorf("after logout: %d, want 401", got)
	}
}
