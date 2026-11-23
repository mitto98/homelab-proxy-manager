// Package api implements the Panel's JSON HTTP API and serves the embedded SPA.
//
// The API contract is api/openapi.yaml. Go types, routing and the strict server
// interface are generated from it into api.gen.go; handlers live in handlers.go.
// Regenerate with scripts/generate.sh.
package api

//go:generate go tool oapi-codegen -config ../../api/oapi-codegen.yaml -o api.gen.go ../../api/openapi.yaml

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mitto98/homelab-proxy-manager/internal/auth"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
)

const (
	cookieName = "hpm_session"
	sessionTTL = 14 * 24 * time.Hour
	maxBody    = 1 << 20
)

// errUnauthorized is returned by the session middleware and mapped to a 401.
var errUnauthorized = errors.New("not signed in")

// Server holds the API's dependencies and implements StrictServerInterface.
type Server struct {
	store *store.Store
	spa   fs.FS
	now   func() time.Time
	// dummyHash is compared against when a username is unknown, so login timing
	// does not reveal which usernames exist.
	dummyHash string
}

// New builds the HTTP handler. spa may be nil, in which case only /api is served.
func New(st *store.Store, spa fs.FS) (http.Handler, error) {
	dummy, err := auth.HashPassword("dummy-password-for-timing")
	if err != nil {
		return nil, err
	}
	s := &Server{store: st, spa: spa, now: time.Now, dummyHash: dummy}

	secured, err := securedOperations()
	if err != nil {
		return nil, err
	}
	// Authentication is enforced here and only here: every operation that
	// declares `security` in the contract requires a valid session.
	strict := NewStrictHandlerWithOptions(s, []StrictMiddlewareFunc{s.withRequest, s.requireSession(secured)},
		StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
				writeError(w, http.StatusBadRequest, "invalid request body")
			},
			ResponseErrorHandlerFunc: responseError,
		})

	mux := http.NewServeMux()
	HandlerWithOptions(strict, StdHTTPServerOptions{
		BaseURL:    "/api",
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	if spa != nil {
		mux.HandleFunc("/", s.handleSPA)
	}
	return limitBody(requireJSON(mux)), nil
}

// securedOperations returns the operation IDs that declare a security
// requirement in the embedded OpenAPI contract.
func securedOperations() (map[string]bool, error) {
	doc, err := GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI spec: %w", err)
	}
	secured := map[string]bool{}
	for _, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if op.Security != nil && len(*op.Security) > 0 {
				secured[op.OperationID] = true
			}
		}
	}
	return secured, nil
}

// limitBody caps request bodies on /api.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		next.ServeHTTP(w, r)
	})
}

// requireJSON rejects state-changing /api requests that are not JSON. Together
// with SameSite=Lax cookies this blocks cross-site form posts (a plain HTML form
// cannot send application/json without a CORS preflight). It applies even to
// operations without a request body, such as logout.
func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") &&
			r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions &&
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type (
	requestKey struct{}
	userKey    struct{}
)

// withRequest makes the HTTP request available to handlers, which otherwise
// only receive a context (they need it for TLS detection and the session cookie).
func (s *Server) withRequest(next StrictHandlerFunc, _ string) StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, in any) (any, error) {
		return next(context.WithValue(ctx, requestKey{}, r), w, r, in)
	}
}

// requireSession rejects requests without a valid session for every operation
// in secured, and puts the signed-in user in the context.
func (s *Server) requireSession(secured map[string]bool) StrictMiddlewareFunc {
	return func(next StrictHandlerFunc, operationID string) StrictHandlerFunc {
		if !secured[operationID] {
			return next
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, in any) (any, error) {
			c, err := r.Cookie(cookieName)
			if err != nil {
				return nil, errUnauthorized
			}
			u, err := s.store.SessionUser(ctx, auth.HashToken(c.Value), s.now())
			if errors.Is(err, store.ErrNotFound) {
				return nil, errUnauthorized
			}
			if err != nil {
				return nil, err
			}
			return next(context.WithValue(ctx, userKey{}, u), w, r, in)
		}
	}
}

// responseError maps errors returned by handlers and middleware to JSON errors.
func responseError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errUnauthorized) {
		writeError(w, http.StatusUnauthorized, errUnauthorized.Error())
		return
	}
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// handleSPA serves embedded files, falling back to index.html so client-side
// routes work on reload.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if f, err := s.spa.Open(name); err == nil {
		st, serr := f.Stat()
		f.Close()
		if serr == nil && !st.IsDir() {
			http.ServeFileFS(w, r, s.spa, name)
			return
		}
	}
	if _, err := fs.Stat(s.spa, "index.html"); err != nil {
		http.Error(w, "web UI not built", http.StatusNotFound)
		return
	}
	http.ServeFileFS(w, r, s.spa, "index.html")
}
