package traefiktest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Router is the subset of Traefik's /api/http/routers entry the harness needs.
type Router struct {
	// Name carries the provider suffix: a router "host-1" from the file provider
	// is listed as "host-1@file".
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	Status   string   `json:"status"` // "enabled", "disabled" or "warning"
	Rule     string   `json:"rule"`
	Errors   []string `json:"error"`
}

// maxPages bounds the pagination loop so a misbehaving server cannot spin it.
const maxPages = 1000

// listRouters returns every router Traefik reports.
//
// Traefik pages /api/http/routers at 100 entries by default and silently drops the
// rest, so the request asks for a large page and still follows X-Next-Page in case
// that cap is lower than the router count.
func listRouters(ctx context.Context, client *http.Client, apiURL string) ([]Router, error) {
	var all []Router
	page := 1
	for range maxPages {
		url := fmt.Sprintf("%s/api/http/routers?per_page=1000&page=%d", apiURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var batch []Router
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("GET %s: decode: %w", url, err)
		}
		all = append(all, batch...)

		next, ok := nextPage(resp.Header.Get("X-Next-Page"), page)
		if !ok {
			return all, nil
		}
		page = next
	}
	return nil, fmt.Errorf("router listing did not end after %d pages", maxPages)
}

// nextPage decodes X-Next-Page. Traefik sends "1" on the last page (the value wraps
// to the first page) and no header at all on some endpoints, so only a page number
// beyond the current one means there is more.
func nextPage(header string, current int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || n <= current {
		return 0, false
	}
	return n, true
}

// findRouter reports whether name is listed and enabled, and describes what was
// seen for the failure message when it is not.
func findRouter(routers []Router, name string) (found bool, seen string) {
	lines := make([]string, 0, len(routers))
	for _, r := range routers {
		if r.Name == name && r.Status == "enabled" {
			return true, ""
		}
		line := r.Name + " (" + r.Status + ")"
		if len(r.Errors) > 0 {
			line += ": " + strings.Join(r.Errors, "; ")
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return false, "no routers listed"
	}
	return false, "routers seen:\n  " + strings.Join(lines, "\n  ")
}
