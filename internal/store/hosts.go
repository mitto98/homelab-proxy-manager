package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
)

// DomainTakenError reports that a domain already belongs to another host. It
// matches ErrConflict with errors.Is.
type DomainTakenError struct{ Domain string }

func (e *DomainTakenError) Error() string {
	return fmt.Sprintf("domain %q is already used by another host", e.Domain)
}

func (e *DomainTakenError) Is(target error) bool { return target == ErrConflict }

// dbtx is what both *sql.DB and *sql.Tx provide.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx is a database transaction. Host changes made through it are atomic: they
// are all stored, or none are. The apply pipeline ([2.5]) renders Traefik config
// from the same transaction and returns an error from the WithTx callback to
// roll everything back when validation fails.
type Tx struct{ q dbtx }

// WithTx runs fn in a transaction, committing if it returns nil and rolling
// back otherwise. The store has one database connection, so fn must use only
// the Tx it is given: calling Store methods inside fn would block forever.
func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Tx{q: sqlTx}); err != nil {
		sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}

// CreateHost stores a new host with its domains and returns it as stored.
func (t *Tx) CreateHost(ctx context.Context, h model.Host) (model.Host, error) {
	if err := t.checkDomains(ctx, 0, h.Domains); err != nil {
		return model.Host{}, err
	}
	res, err := t.q.ExecContext(ctx,
		`INSERT INTO hosts (kind, enabled, upstream_scheme, upstream_host, upstream_port, upstream_skip_tls_verify)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		h.Kind, h.Enabled, h.UpstreamScheme, h.UpstreamHost, h.UpstreamPort, h.UpstreamSkipTLSVerify)
	if err != nil {
		return model.Host{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Host{}, err
	}
	if err := t.insertDomains(ctx, id, h.Domains); err != nil {
		return model.Host{}, err
	}
	return t.GetHost(ctx, id)
}

// UpdateHost replaces a host's fields and domains. It returns ErrNotFound if
// the host does not exist.
func (t *Tx) UpdateHost(ctx context.Context, h model.Host) (model.Host, error) {
	if err := t.checkDomains(ctx, h.ID, h.Domains); err != nil {
		return model.Host{}, err
	}
	res, err := t.q.ExecContext(ctx,
		`UPDATE hosts SET enabled = ?, upstream_scheme = ?, upstream_host = ?, upstream_port = ?,
		        upstream_skip_tls_verify = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		  WHERE id = ?`,
		h.Enabled, h.UpstreamScheme, h.UpstreamHost, h.UpstreamPort, h.UpstreamSkipTLSVerify, h.ID)
	if err != nil {
		return model.Host{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return model.Host{}, err
	} else if n == 0 {
		return model.Host{}, ErrNotFound
	}
	if _, err := t.q.ExecContext(ctx, `DELETE FROM host_domains WHERE host_id = ?`, h.ID); err != nil {
		return model.Host{}, err
	}
	if err := t.insertDomains(ctx, h.ID, h.Domains); err != nil {
		return model.Host{}, err
	}
	return t.GetHost(ctx, h.ID)
}

// DeleteHost removes a host and, by cascade, its domains.
func (t *Tx) DeleteHost(ctx context.Context, id int64) error {
	res, err := t.q.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetHost returns one host, or ErrNotFound.
func (t *Tx) GetHost(ctx context.Context, id int64) (model.Host, error) {
	hosts, err := t.listHosts(ctx, `WHERE h.id = ?`, id)
	if err != nil {
		return model.Host{}, err
	}
	if len(hosts) == 0 {
		return model.Host{}, ErrNotFound
	}
	return hosts[0], nil
}

// ListHosts returns every host, oldest first.
func (t *Tx) ListHosts(ctx context.Context) ([]model.Host, error) {
	return t.listHosts(ctx, ``)
}

// CreateHost, UpdateHost, DeleteHost, GetHost and ListHosts on the Store each
// run in their own transaction.

func (s *Store) CreateHost(ctx context.Context, h model.Host) (out model.Host, err error) {
	err = s.WithTx(ctx, func(t *Tx) (e error) { out, e = t.CreateHost(ctx, h); return })
	return
}

func (s *Store) UpdateHost(ctx context.Context, h model.Host) (out model.Host, err error) {
	err = s.WithTx(ctx, func(t *Tx) (e error) { out, e = t.UpdateHost(ctx, h); return })
	return
}

func (s *Store) DeleteHost(ctx context.Context, id int64) error {
	return s.WithTx(ctx, func(t *Tx) error { return t.DeleteHost(ctx, id) })
}

func (s *Store) GetHost(ctx context.Context, id int64) (model.Host, error) {
	return (&Tx{q: s.db}).GetHost(ctx, id)
}

func (s *Store) ListHosts(ctx context.Context) ([]model.Host, error) {
	return (&Tx{q: s.db}).ListHosts(ctx)
}

// checkDomains returns a *DomainTakenError if any domain belongs to a host other
// than ownerID (0 for a host that does not exist yet). The unique index is the
// backstop; checking first lets the error name the domain.
func (t *Tx) checkDomains(ctx context.Context, ownerID int64, domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	args := []any{ownerID}
	for _, d := range domains {
		args = append(args, d)
	}
	var taken string
	err := t.q.QueryRowContext(ctx,
		`SELECT domain FROM host_domains
		  WHERE host_id <> ? AND domain IN (`+placeholders(len(domains))+`)
		  ORDER BY id LIMIT 1`, args...).Scan(&taken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return &DomainTakenError{Domain: taken}
}

func (t *Tx) insertDomains(ctx context.Context, hostID int64, domains []string) error {
	for _, d := range domains {
		if _, err := t.q.ExecContext(ctx,
			`INSERT INTO host_domains (host_id, domain) VALUES (?, ?)`, hostID, d); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) listHosts(ctx context.Context, where string, args ...any) ([]model.Host, error) {
	rows, err := t.q.QueryContext(ctx,
		`SELECT h.id, h.kind, h.enabled, COALESCE(h.upstream_scheme, ''), COALESCE(h.upstream_host, ''),
		        COALESCE(h.upstream_port, 0), h.upstream_skip_tls_verify, h.created_at, h.updated_at
		   FROM hosts h `+where+` ORDER BY h.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []model.Host
	byID := map[int64]int{}
	for rows.Next() {
		var h model.Host
		var created, updated string
		if err := rows.Scan(&h.ID, &h.Kind, &h.Enabled, &h.UpstreamScheme, &h.UpstreamHost,
			&h.UpstreamPort, &h.UpstreamSkipTLSVerify, &created, &updated); err != nil {
			return nil, err
		}
		if h.CreatedAt, err = time.Parse(timeLayout, created); err != nil {
			return nil, err
		}
		if h.UpdatedAt, err = time.Parse(timeLayout, updated); err != nil {
			return nil, err
		}
		h.Domains = []string{}
		byID[h.ID] = len(hosts)
		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Domains come back in insertion order, which is the order the user gave.
	drows, err := t.q.QueryContext(ctx,
		`SELECT d.host_id, d.domain FROM host_domains d JOIN hosts h ON h.id = d.host_id `+where+` ORDER BY d.id`, args...)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var id int64
		var domain string
		if err := drows.Scan(&id, &domain); err != nil {
			return nil, err
		}
		if i, ok := byID[id]; ok {
			hosts[i].Domains = append(hosts[i].Domains, domain)
		}
	}
	return hosts, drows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
