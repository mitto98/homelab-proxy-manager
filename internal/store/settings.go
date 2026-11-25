package store

import (
	"context"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
)

// GetSettings returns the single settings row.
func (t *Tx) GetSettings(ctx context.Context) (model.Settings, error) {
	var s model.Settings
	err := t.q.QueryRowContext(ctx,
		`SELECT http_port, https_port, api_port FROM settings WHERE id = 1`).
		Scan(&s.HTTPPort, &s.HTTPSPort, &s.APIPort)
	return s, err
}

// UpdateSettings replaces the settings row. The database rejects ports outside
// 1..65535.
func (t *Tx) UpdateSettings(ctx context.Context, s model.Settings) error {
	_, err := t.q.ExecContext(ctx,
		`UPDATE settings SET http_port = ?, https_port = ?, api_port = ?,
		        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = 1`,
		s.HTTPPort, s.HTTPSPort, s.APIPort)
	return err
}

// GetSettings and UpdateSettings on the Store each run in their own transaction.

func (s *Store) GetSettings(ctx context.Context) (model.Settings, error) {
	return (&Tx{q: s.db}).GetSettings(ctx)
}

func (s *Store) UpdateSettings(ctx context.Context, v model.Settings) error {
	return s.WithTx(ctx, func(t *Tx) error { return t.UpdateSettings(ctx, v) })
}
