package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Every operation that declares `security` in api/openapi.yaml must reject a
// request without a session, and public operations must not. This reads the
// contract, so a new secured route is covered without editing this test.
func TestSecuredOperationsRequireSession(t *testing.T) {
	ts := newServer(t)
	doc, err := GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security == nil || len(*op.Security) == 0 {
				continue
			}
			checked++
			// Fill path parameters with a placeholder; none exist yet.
			url := ts.URL + "/api" + path
			res := do(t, http.DefaultClient, method, url, `{}`)
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s (%s): want 401 without a session, got %d", method, path, op.OperationID, res.StatusCode)
			}
			var body struct{ Error string }
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.Error == "" {
				t.Errorf("%s %s: 401 must carry the JSON error shape, got err=%v body=%+v", method, path, err, body)
			}
		}
	}
	if checked == 0 {
		t.Fatal("contract declares no secured operation; the session check is untested")
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	ts := newServer(t)
	c := &http.Client{} // no jar: inspect Set-Cookie ourselves
	res := do(t, c, "POST", ts.URL+"/api/setup", `{"username":"admin","password":"correct horse"}`)
	if res.StatusCode != 201 {
		t.Fatalf("setup: %d", res.StatusCode)
	}
	cs := res.Cookies()
	if len(cs) != 1 || cs[0].Name != "hpm_session" {
		t.Fatalf("setup must set exactly the session cookie, got %v", cs)
	}
	got := cs[0]
	if !got.HttpOnly || got.SameSite != http.SameSiteLaxMode || got.Path != "/" || got.Value == "" || got.Expires.IsZero() {
		t.Errorf("session cookie attributes wrong: %+v", got)
	}
	if got.Secure {
		t.Error("cookie must not be Secure over plain HTTP (the Panel may be reached without TLS)")
	}

	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/logout", nil)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(got)
	out, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	if out.StatusCode != 204 {
		t.Fatalf("logout without a body: %d", out.StatusCode)
	}
	cleared := out.Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 || cleared[0].Value != "" {
		t.Errorf("logout must expire the session cookie, got %v", cleared)
	}
}

func TestErrorShapes(t *testing.T) {
	ts := newServer(t)
	for name, tc := range map[string]struct {
		method, path, body string
		want               int
	}{
		"malformed JSON":     {"POST", "/api/auth/login", `{"username":`, 400},
		"empty body":         {"POST", "/api/auth/login", ``, 400},
		"oversized body":     {"POST", "/api/auth/login", `{"username":"` + strings.Repeat("a", 2<<20) + `"}`, 400},
		"unknown api route":  {"GET", "/api/nope", ``, 404},
		"wrong method":       {"DELETE", "/api/status", `{}`, 404},
		"username too long":  {"POST", "/api/setup", `{"username":"` + strings.Repeat("a", 65) + `","password":"correct horse"}`, 400},
		"blank username":     {"POST", "/api/setup", `{"username":"   ","password":"correct horse"}`, 400},
		"password too long":  {"POST", "/api/setup", `{"username":"admin","password":"` + strings.Repeat("p", 73) + `"}`, 400},
		"unknown user login": {"POST", "/api/auth/login", `{"username":"ghost","password":"whatever"}`, 401},
	} {
		t.Run(name, func(t *testing.T) {
			res := do(t, http.DefaultClient, tc.method, ts.URL+tc.path, tc.body)
			if res.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", res.StatusCode, tc.want)
			}
			if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("error responses must be JSON, got %q", ct)
			}
			var body struct{ Error string }
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.Error == "" {
				t.Errorf("want {\"error\": ...}, got err=%v body=%+v", err, body)
			}
		})
	}
}

func TestStatusShape(t *testing.T) {
	ts := newServer(t)
	var got Status
	if err := json.NewDecoder(do(t, http.DefaultClient, "GET", ts.URL+"/api/status", "").Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.SetupRequired || got.Version == "" {
		t.Errorf("unexpected status: %+v", got)
	}
}
