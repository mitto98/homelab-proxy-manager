package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	for i := 0; i < 2; i++ {
		s, err := Open(context.Background(), path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		s.Close()
	}
}

func TestCreateFirstUserOnlyOnce(t *testing.T) {
	s, ctx := open(t), context.Background()
	if _, err := s.CreateFirstUser(ctx, "admin", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFirstUser(ctx, "other", "h"); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("want ErrSetupDone, got %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s, ctx := open(t), context.Background()
	u, _ := s.CreateFirstUser(ctx, "admin", "h")
	now := time.Now()
	if err := s.CreateSession(ctx, "tok", u.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SessionUser(ctx, "tok", now); err != nil || got.ID != u.ID {
		t.Fatalf("valid session: %v %v", got, err)
	}
	if _, err := s.SessionUser(ctx, "tok", now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session should not resolve, got %v", err)
	}
	if err := s.DeleteSession(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, "tok", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session should not resolve, got %v", err)
	}
}

func TestSettingsDefaultsAndChecks(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	got, err := st.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != model.DefaultSettings() {
		t.Errorf("defaults = %+v, want %+v", got, model.DefaultSettings())
	}

	want := model.Settings{HTTPPort: 8080, HTTPSPort: 8443, APIPort: 9000}
	if err := st.UpdateSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.GetSettings(ctx); got != want {
		t.Errorf("after update = %+v, want %+v", got, want)
	}
	for _, bad := range []model.Settings{{HTTPPort: 0, HTTPSPort: 443, APIPort: 8899}, {HTTPPort: 80, HTTPSPort: 65536, APIPort: 8899}, {HTTPPort: 80, HTTPSPort: 443, APIPort: -1}} {
		if err := st.UpdateSettings(ctx, bad); err == nil {
			t.Errorf("UpdateSettings(%+v) accepted an out-of-range port", bad)
		}
	}
	// A second row must be impossible.
	if _, err := st.db.ExecContext(ctx, `INSERT INTO settings (id) VALUES (2)`); err == nil {
		t.Error("a second settings row was accepted")
	}
}
