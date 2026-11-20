package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
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
