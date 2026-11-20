// Package store owns the SQLite database: opening, migrations and queries.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, registers "sqlite"

	"github.com/mitto98/homelab-proxy-manager/migrations"
)

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

// timeLayout is a fixed-width UTC format, so string comparison matches time order.
const timeLayout = "2006-01-02T15:04:05Z"

// Open opens (creating if needed) the database at path and applies migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// One connection serialises writes and avoids SQLITE_BUSY; the panel is low traffic.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx, migrations.FS); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context, fsys fs.FS) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')))`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %q: name must start with a number: %w", name, err)
		}
		var applied int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
			return err
		}
		if applied > 0 {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Errors returned by the store.
var (
	ErrSetupDone = errors.New("setup already completed")
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("already exists")
)

// User is a Panel user.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
}

// CountUsers returns how many users exist.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateFirstUser creates the initial admin, atomically, only if no user exists.
func (s *Store) CreateFirstUser(ctx context.Context, username, passwordHash string) (User, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role)
		 SELECT ?, ?, 'admin' WHERE NOT EXISTS (SELECT 1 FROM users)`,
		username, passwordHash)
	if err != nil {
		return User{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if n == 0 {
		return User{}, ErrSetupDone
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, PasswordHash: passwordHash, Role: "admin"}, nil
}

// UserByUsername looks a user up by login name (case-insensitive).
func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// CreateSession stores a session by the hash of its token.
func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, expires time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash, userID, expires.UTC().Format(timeLayout))
	return err
}

// SessionUser returns the user owning an unexpired session.
func (s *Store) SessionUser(ctx context.Context, tokenHash string, now time.Time) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.password_hash, u.role
		   FROM sessions s JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = ? AND s.expires_at > ?`,
		tokenHash, now.UTC().Format(timeLayout)).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// DeleteSession removes one session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteExpiredSessions removes sessions past their expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at <= ?`, now.UTC().Format(timeLayout))
	return err
}
