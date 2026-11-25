// Package apply is the pipeline that turns a change to the Panel's data into
// Traefik's files (docs/architecture.md section 7.1). It owns the single writer
// mutex, and it is the only code that writes the generated files.
//
// # Failure semantics
//
// Before commit, everything is atomic: if the change function, rendering or
// validation fails, the transaction rolls back and no file is touched.
//
// After commit the database is the truth and the files are derived. If writing
// a file fails (disk full, permissions), Apply returns the error although the
// change is stored; the files are then stale until the next Apply or the next
// Panel start repairs them, because both rewrite every file whose content
// differs. This is deliberate: the alternative, undoing a committed change,
// would need a second transaction that can fail the same way.
//
// A failed Restart is not an error of the request. The static config is
// already on disk, the change is stored, and Traefik picks the file up on its
// next start; the failure is logged and reported in [Result].
package apply

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/mitto98/homelab-proxy-manager/internal/render"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
	"github.com/mitto98/homelab-proxy-manager/internal/traefik"
)

// File names inside the data directory (docs/architecture.md section 7.2).
const (
	StaticFile  = "traefik.yml"
	DynamicDir  = "dynamic"
	DynamicFile = "panel.yml"
)

// InvalidConfigError reports that the rendered configuration failed validation.
// Nothing was stored and no file changed.
type InvalidConfigError struct{ Err error }

func (e *InvalidConfigError) Error() string { return "rendered config is invalid: " + e.Err.Error() }
func (e *InvalidConfigError) Unwrap() error { return e.Err }

// Result describes what an apply did to the files.
type Result struct {
	// DynamicChanged and StaticChanged say whether the file's content differed
	// from what was on disk and was rewritten.
	DynamicChanged bool
	StaticChanged  bool
	// Restarted is true when the controller was asked to restart Traefik.
	Restarted bool
	// RestartErr is the controller's error, if any. It does not fail the apply.
	RestartErr error
}

// Applier runs the pipeline. Create it with [New]; it is safe for concurrent use.
type Applier struct {
	st      *store.Store
	ctrl    traefik.Controller
	staticP string // <data>/traefik.yml
	dynDir  string // <data>/dynamic
	log     *slog.Logger

	// validate checks the rendered model; tests replace it to force failures.
	validate func(render.Model) error
	// afterTempWritten, set by tests, runs while the temporary file exists.
	afterTempWritten func(dir string)

	mu sync.Mutex // the single writer lock: one pipeline run at a time
}

// Option customises an Applier.
type Option func(*Applier)

// WithLogger sets the logger; the default is slog.Default().
func WithLogger(l *slog.Logger) Option { return func(a *Applier) { a.log = l } }

// WithValidator replaces the validation step. It exists for tests that need
// the pipeline to fail after rendering.
func WithValidator(fn func(render.Model) error) Option { return func(a *Applier) { a.validate = fn } }

// New returns an Applier writing under dataDir: <dataDir>/traefik.yml and
// <dataDir>/dynamic/panel.yml. dataDir is made absolute because the path of the
// dynamic directory is written into traefik.yml and Traefik must find it from
// its own working directory.
func New(st *store.Store, ctrl traefik.Controller, dataDir string, opts ...Option) (*Applier, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("data directory: %w", err)
	}
	a := &Applier{
		st:       st,
		ctrl:     ctrl,
		staticP:  filepath.Join(abs, StaticFile),
		dynDir:   filepath.Join(abs, DynamicDir),
		log:      slog.Default(),
		validate: render.Model.Validate,
	}
	for _, o := range opts {
		o(a)
	}
	return a, nil
}

// Apply runs the pipeline: change is called with a transaction, the result is
// rendered and validated from that same transaction, and only then committed
// and written. An error from change is returned unchanged so callers can
// recognise it (for example a conflicting domain); a validation failure is an
// *[InvalidConfigError]. The mutex is held until the files are written.
func (a *Applier) Apply(ctx context.Context, change func(*store.Tx) error) (Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.run(ctx, change)
}

// Start renders everything from the database and writes the files, creating
// the directories if needed. Call it once at Panel start: it repairs a restored
// backup or a hand-deleted or corrupted file. An error is fatal for the
// Panel, which should not serve a state it cannot publish to Traefik.
func (a *Applier) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	res, err := a.run(ctx, nil)
	if err != nil {
		return err
	}
	a.log.Info("traefik config written", "static_changed", res.StaticChanged, "dynamic_changed", res.DynamicChanged)
	return nil
}

func (a *Applier) run(ctx context.Context, change func(*store.Tx) error) (Result, error) {
	var dynamic, static []byte
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		// 1. Apply the change.
		if change != nil {
			if err := change(tx); err != nil {
				return err
			}
		}
		// 2. Render from this transaction's view of the data.
		settings, err := tx.GetSettings(ctx)
		if err != nil {
			return fmt.Errorf("read settings: %w", err)
		}
		hosts, err := tx.ListHosts(ctx)
		if err != nil {
			return fmt.Errorf("read hosts: %w", err)
		}
		m := render.Build(render.Input{Settings: settings, Hosts: hosts, DynamicDir: a.dynDir})
		// 3. Validate; returning an error rolls the transaction back.
		if err := a.validate(m); err != nil {
			return &InvalidConfigError{Err: err}
		}
		if dynamic, err = m.DynamicYAML(); err != nil {
			return err
		}
		if static, err = m.StaticYAML(); err != nil {
			return err
		}
		return nil
		// 4. WithTx commits when this function returns nil.
	})
	if err != nil {
		return Result{}, err
	}

	// 5. Write the files.
	res, err := a.publish(dynamic, static)
	if err != nil {
		return res, err
	}
	// 6. Restart Traefik only if the static config content changed.
	if res.StaticChanged {
		res.Restarted = true
		if err := a.ctrl.Restart(ctx); err != nil {
			res.RestartErr = err
			a.log.Error("restart traefik failed; the new static config is on disk and applies at its next start", "error", err)
		}
	}
	return res, nil
}

// publish writes the dynamic file first, then the static one. Files are only
// replaced when their content differs, which also keeps Traefik's file watcher
// quiet for changes that do not alter a file.
func (a *Applier) publish(dynamic, static []byte) (Result, error) {
	var res Result
	for _, d := range []string{filepath.Dir(a.staticP), a.dynDir} {
		if err := os.MkdirAll(d, dirMode); err != nil {
			return res, fmt.Errorf("create %s: %w (the change is stored; files are rewritten by the next apply or start)", d, err)
		}
	}
	var err error
	if res.DynamicChanged, err = a.writeIfChanged(a.dynDir, DynamicFile, dynamic); err != nil {
		return res, fmt.Errorf("write dynamic config: %w (the change is stored; files are rewritten by the next apply or start)", err)
	}
	if res.StaticChanged, err = a.writeIfChanged(filepath.Dir(a.staticP), StaticFile, static); err != nil {
		return res, fmt.Errorf("write static config: %w (the change is stored; files are rewritten by the next apply or start)", err)
	}
	return res, nil
}
