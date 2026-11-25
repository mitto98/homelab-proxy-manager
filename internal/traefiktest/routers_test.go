package traefiktest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestNextPage(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		current int
		want    int
		more    bool
	}{
		{"absent", "", 1, 0, false},
		{"wraps to first page on the last page", "1", 1, 0, false},
		{"zero", "0", 1, 0, false},
		{"garbage", "next", 1, 0, false},
		{"more pages", "2", 1, 2, true},
		{"whitespace", " 3 ", 2, 3, true},
		{"never go backwards", "1", 3, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, more := nextPage(tt.header, tt.current)
			if got != tt.want || more != tt.more {
				t.Errorf("nextPage(%q, %d) = %d, %v; want %d, %v", tt.header, tt.current, got, more, tt.want, tt.more)
			}
		})
	}
}

// fakeAPI serves routers like Traefik: pages of size perPage (capped by the cap
// argument, to mimic a server that ignores a big per_page) and X-Next-Page.
func fakeAPI(t *testing.T, routers []Router, serverCap int, requests *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.URL.RequestURI())
		if r.URL.Path != "/api/http/routers" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		perPage = min(perPage, serverCap)
		start := (page - 1) * perPage
		end := min(start+perPage, len(routers))
		next := page + 1
		if end >= len(routers) {
			next = 1 // Traefik wraps to the first page when there is no more
		}
		w.Header().Set("X-Next-Page", strconv.Itoa(next))
		_ = json.NewEncoder(w).Encode(routers[start:end])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListRoutersFollowsPagination(t *testing.T) {
	var routers []Router
	for i := range 7 {
		routers = append(routers, Router{Name: "r" + strconv.Itoa(i) + "@file", Status: "enabled"})
	}
	var reqs []string
	srv := fakeAPI(t, routers, 3, &reqs)

	got, err := listRouters(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(routers) {
		t.Fatalf("got %d routers, want %d (requests: %v)", len(got), len(routers), reqs)
	}
	for i, r := range got {
		if r.Name != routers[i].Name {
			t.Errorf("router %d = %q, want %q", i, r.Name, routers[i].Name)
		}
	}
	if len(reqs) != 3 {
		t.Errorf("made %d requests, want 3: %v", len(reqs), reqs)
	}
	for _, r := range reqs {
		if !strings.Contains(r, "per_page=1000") {
			t.Errorf("request %q does not ask for a large page; Traefik truncates at 100 by default", r)
		}
	}
}

func TestListRoutersSinglePage(t *testing.T) {
	var reqs []string
	srv := fakeAPI(t, []Router{{Name: "a@file", Status: "enabled"}}, 1000, &reqs)
	got, err := listRouters(context.Background(), srv.Client(), srv.URL)
	if err != nil || len(got) != 1 || len(reqs) != 1 {
		t.Fatalf("got %v, %v after %d requests", got, err, len(reqs))
	}
}

func TestListRoutersErrors(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer srv.Close()
		if _, err := listRouters(context.Background(), srv.Client(), srv.URL); err == nil || !strings.Contains(err.Error(), "500") {
			t.Errorf("err = %v, want a 500 error", err)
		}
	})
	t.Run("bad json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{")) }))
		defer srv.Close()
		if _, err := listRouters(context.Background(), srv.Client(), srv.URL); err == nil {
			t.Error("want a decode error")
		}
	})
	t.Run("endless pages stop", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
			_, _ = w.Write([]byte("[]"))
		}))
		defer srv.Close()
		if _, err := listRouters(context.Background(), srv.Client(), srv.URL); err == nil {
			t.Error("want an error instead of looping forever")
		}
	})
}

func TestFindRouter(t *testing.T) {
	routers := []Router{
		{Name: "a@file", Status: "enabled"},
		{Name: "b@file", Status: "disabled", Errors: []string{"service \"x@file\" does not exist"}},
	}
	if found, _ := findRouter(routers, "a@file"); !found {
		t.Error("enabled router not found")
	}
	found, seen := findRouter(routers, "b@file")
	if found {
		t.Error("a disabled router must not count as found")
	}
	for _, want := range []string{"a@file (enabled)", "b@file (disabled)", "does not exist"} {
		if !strings.Contains(seen, want) {
			t.Errorf("seen = %q, missing %q", seen, want)
		}
	}
	if found, _ := findRouter(routers, "a"); found {
		t.Error("name must match exactly, including the @file suffix")
	}
	if _, seen := findRouter(nil, "a@file"); !strings.Contains(seen, "no routers") {
		t.Errorf("seen = %q", seen)
	}
}
