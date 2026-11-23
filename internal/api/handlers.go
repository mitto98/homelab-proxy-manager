package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mitto98/homelab-proxy-manager/internal/auth"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
	"github.com/mitto98/homelab-proxy-manager/internal/version"
)

var _ StrictServerInterface = (*Server)(nil)

func toUser(u store.User) User { return User{Id: u.ID, Username: u.Username, Role: UserRole(u.Role)} }

func errorBody(msg string) ErrorJSONResponse { return ErrorJSONResponse{Error: msg} }

func requestFrom(ctx context.Context) *http.Request {
	r, _ := ctx.Value(requestKey{}).(*http.Request)
	return r
}

func (s *Server) GetStatus(ctx context.Context, _ GetStatusRequestObject) (GetStatusResponseObject, error) {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	return GetStatus200JSONResponse{SetupRequired: n == 0, Version: version.Version}, nil
}

func (s *Server) Setup(ctx context.Context, req SetupRequestObject) (SetupResponseObject, error) {
	in := *req.Body
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || len(in.Username) > 64 {
		return Setup400JSONResponse{errorBody("username must be 1 to 64 characters")}, nil
	}
	if len(in.Password) < auth.MinPasswordLength || len(in.Password) > 72 {
		return Setup400JSONResponse{errorBody("password must be 8 to 72 characters")}, nil
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u, err := s.store.CreateFirstUser(ctx, in.Username, hash)
	if errors.Is(err, store.ErrSetupDone) {
		return Setup409JSONResponse{Error: "setup already completed"}, nil
	}
	if err != nil {
		return nil, err
	}
	cookie, err := s.startSession(ctx, u)
	if err != nil {
		return nil, err
	}
	return Setup201JSONResponse{Body: toUser(u), Headers: Setup201ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *Server) Login(ctx context.Context, req LoginRequestObject) (LoginResponseObject, error) {
	in := *req.Body
	u, err := s.store.UserByUsername(ctx, strings.TrimSpace(in.Username))
	hash := s.dummyHash
	if err == nil {
		hash = u.PasswordHash
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if !auth.CheckPassword(hash, in.Password) || err != nil {
		return Login401JSONResponse{Error: "invalid username or password"}, nil
	}
	_ = s.store.DeleteExpiredSessions(ctx, s.now())
	cookie, err := s.startSession(ctx, u)
	if err != nil {
		return nil, err
	}
	return Login200JSONResponse{Body: toUser(u), Headers: Login200ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *Server) Logout(ctx context.Context, _ LogoutRequestObject) (LogoutResponseObject, error) {
	r := requestFrom(ctx)
	if c, err := r.Cookie(cookieName); err == nil {
		_ = s.store.DeleteSession(ctx, auth.HashToken(c.Value))
	}
	cleared := (&http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
	}).String()
	return Logout204Response{Headers: Logout204ResponseHeaders{SetCookie: &cleared}}, nil
}

func (s *Server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	// requireSession has already authenticated the request and stored the user;
	// failing closed here keeps a missing middleware from becoming a panic.
	u, ok := ctx.Value(userKey{}).(store.User)
	if !ok {
		return nil, errUnauthorized
	}
	return GetMe200JSONResponse(toUser(u)), nil
}

// startSession creates a session for u and returns the Set-Cookie header value.
func (s *Server) startSession(ctx context.Context, u store.User) (string, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	expires := s.now().Add(sessionTTL)
	if err := s.store.CreateSession(ctx, hash, u.ID, expires); err != nil {
		return "", err
	}
	return (&http.Cookie{
		Name: cookieName, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: requestFrom(ctx).TLS != nil, SameSite: http.SameSiteLaxMode,
	}).String(), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
