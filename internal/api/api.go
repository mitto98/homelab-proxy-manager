// Package api implements the Panel's JSON HTTP API and serves the embedded SPA.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/mitto98/homelab-proxy-manager/internal/auth"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
	"github.com/mitto98/homelab-proxy-manager/internal/version"
)

const (
	cookieName = "hpm_session"
	sessionTTL = 14 * 24 * time.Hour
	maxBody    = 1 << 20
)

// Server holds the API's dependencies.
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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.Handle("GET /api/auth/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	if spa != nil {
		mux.HandleFunc("/", s.handleSPA)
	}
	return requireJSON(mux), nil
}

// requireJSON rejects state-changing /api requests that are not JSON. Together
// with SameSite=Lax cookies this blocks cross-site form posts (a plain HTML form
// cannot send application/json without a CORS preflight).
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

type ctxKey struct{}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		u, err := s.store.SessionUser(r.Context(), auth.HashToken(c.Value), s.now())
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusUnauthorized, "not signed in")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

type userJSON struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func toUserJSON(u store.User) userJSON { return userJSON{u.ID, u.Username, u.Role} }

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setupRequired": n == 0,
		"version":       version.Version,
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || len(in.Username) > 64 {
		writeError(w, http.StatusBadRequest, "username must be 1 to 64 characters")
		return
	}
	if len(in.Password) < auth.MinPasswordLength || len(in.Password) > 72 {
		writeError(w, http.StatusBadRequest, "password must be 8 to 72 characters")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	u, err := s.store.CreateFirstUser(r.Context(), in.Username, hash)
	if errors.Is(err, store.ErrSetupDone) {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !s.startSession(w, r, u) {
		return
	}
	writeJSON(w, http.StatusCreated, toUserJSON(u))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	u, err := s.store.UserByUsername(r.Context(), strings.TrimSpace(in.Username))
	hash := s.dummyHash
	if err == nil {
		hash = u.PasswordHash
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !auth.CheckPassword(hash, in.Password) || err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	_ = s.store.DeleteExpiredSessions(r.Context(), s.now())
	if !s.startSession(w, r, u) {
		return
	}
	writeJSON(w, http.StatusOK, toUserJSON(u))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), auth.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toUserJSON(r.Context().Value(ctxKey{}).(store.User)))
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u store.User) bool {
	token, hash, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	expires := s.now().Add(sessionTTL)
	if err := s.store.CreateSession(r.Context(), hash, u.ID, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
	})
	return true
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

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil || dec.More() {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
