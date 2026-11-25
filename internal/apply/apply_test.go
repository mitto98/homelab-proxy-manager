package apply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
	"github.com/mitto98/homelab-proxy-manager/internal/render"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
)

// recorder is a Controller that counts restarts.
type recorder struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (r *recorder) Restart(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.err
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type env struct {
	st   *store.Store
	a    *Applier
	ctrl *recorder
	data string
}

func setup(t *testing.T, opts ...Option) *env {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	data := filepath.Join(t.TempDir(), "data")
	ctrl := &recorder{}
	a, err := New(st, ctrl, data, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return &env{st: st, a: a, ctrl: ctrl, data: data}
}

func (e *env) dynamicPath() string { return filepath.Join(e.data, DynamicDir, DynamicFile) }
func (e *env) staticPath() string  { return filepath.Join(e.data, StaticFile) }

func (e *env) read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newHost(domain string) model.Host {
	return model.Host{Kind: model.KindProxy, Enabled: true, Domains: []string{domain},
		UpstreamScheme: "http", UpstreamHost: "10.0.0.1", UpstreamPort: 8080}
}

func (e *env) create(t *testing.T, domain string) model.Host {
	t.Helper()
	var h model.Host
	_, err := e.a.Apply(context.Background(), func(tx *store.Tx) (err error) {
		h, err = tx.CreateHost(context.Background(), newHost(domain))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestChangeAppearsInFiles(t *testing.T) {
	e := setup(t)
	h := e.create(t, "app.example.com")

	dyn := e.read(t, e.dynamicPath())
	for _, want := range []string{fmt.Sprintf("host-%d-http:", h.ID), "Host(`app.example.com`)", "http://10.0.0.1:8080"} {
		if !strings.Contains(dyn, want) {
			t.Errorf("panel.yml lacks %q:\n%s", want, dyn)
		}
	}
	if st := e.read(t, e.staticPath()); !strings.Contains(st, "127.0.0.1:8899") {
		t.Errorf("traefik.yml lacks the loopback API entrypoint:\n%s", st)
	}
	// Modes: files 0640, directories 0750 (before umask, which only removes bits).
	for path, want := range map[string]os.FileMode{e.dynamicPath(): 0o640, e.staticPath(): 0o640} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&^want != 0 {
			t.Errorf("%s mode = %v, want at most %v", path, fi.Mode().Perm(), want)
		}
	}
	// The static file names the absolute dynamic directory.
	if !strings.Contains(e.read(t, e.staticPath()), filepath.Join(e.data, DynamicDir)) {
		t.Error("traefik.yml does not contain the absolute dynamic directory")
	}

	// Disabling removes the router; deleting removes the rest.
	h.Enabled = false
	if _, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
		_, err := tx.UpdateHost(context.Background(), h)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if dyn := e.read(t, e.dynamicPath()); strings.Contains(dyn, "app.example.com") {
		t.Errorf("a disabled host is still rendered:\n%s", dyn)
	}
}

func TestValidationFailureRollsBackEverything(t *testing.T) {
	var reject atomic.Bool
	e := setup(t, WithValidator(func(m render.Model) error {
		if reject.Load() {
			return errors.New("injected")
		}
		return m.Validate()
	}))
	e.create(t, "keep.example.com")
	dynBefore, staticBefore := e.read(t, e.dynamicPath()), e.read(t, e.staticPath())
	restarts := e.ctrl.count()

	reject.Store(true)
	_, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
		_, err := tx.CreateHost(context.Background(), newHost("new.example.com"))
		return err
	})
	var ice *InvalidConfigError
	if !errors.As(err, &ice) {
		t.Fatalf("Apply error = %v, want *InvalidConfigError", err)
	}
	hosts, err := e.st.ListHosts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Errorf("%d hosts stored after a rejected change, want 1 (rollback)", len(hosts))
	}
	if e.read(t, e.dynamicPath()) != dynBefore || e.read(t, e.staticPath()) != staticBefore {
		t.Error("files changed although the change was rejected")
	}
	if e.ctrl.count() != restarts {
		t.Error("Traefik was restarted for a rejected change")
	}
}

func TestFailingChangeRollsBack(t *testing.T) {
	e := setup(t)
	e.create(t, "keep.example.com")
	before := e.read(t, e.dynamicPath())

	boom := errors.New("boom")
	_, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
		if _, err := tx.CreateHost(context.Background(), newHost("half.example.com")); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the change function's error unchanged", err)
	}
	if hosts, _ := e.st.ListHosts(context.Background()); len(hosts) != 1 {
		t.Errorf("%d hosts after a failed change, want 1", len(hosts))
	}
	if e.read(t, e.dynamicPath()) != before {
		t.Error("dynamic file changed after a failed change")
	}
}

func TestConflictErrorIsPassedThrough(t *testing.T) {
	e := setup(t)
	e.create(t, "dup.example.com")
	_, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
		_, err := tx.CreateHost(context.Background(), newHost("dup.example.com"))
		return err
	})
	var taken *store.DomainTakenError
	if !errors.As(err, &taken) {
		t.Fatalf("error = %v, want *store.DomainTakenError", err)
	}
}

func TestRestartOnlyWhenStaticContentChanges(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// First write: the static file did not exist, so it counts as changed.
	e.create(t, "a.example.com")
	if got := e.ctrl.count(); got != 1 {
		t.Fatalf("restarts after first apply = %d, want 1", got)
	}
	// Host changes alter only dynamic config.
	e.create(t, "b.example.com")
	res, err := e.a.Apply(ctx, func(*store.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := e.ctrl.count(); got != 1 || res.StaticChanged || res.Restarted {
		t.Fatalf("restarts = %d, result %+v after host-only changes, want no new restart", got, res)
	}
	// A port change alters the static file.
	res, err = e.a.Apply(ctx, func(tx *store.Tx) error {
		return tx.UpdateSettings(ctx, model.Settings{HTTPPort: 8080, HTTPSPort: 443, APIPort: 8899})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.ctrl.count(); got != 2 || !res.StaticChanged || !res.Restarted {
		t.Fatalf("restarts = %d, result %+v after a port change, want a second restart", got, res)
	}
	// Writing the same settings again changes nothing.
	if _, err := e.a.Apply(ctx, func(tx *store.Tx) error {
		return tx.UpdateSettings(ctx, model.Settings{HTTPPort: 8080, HTTPSPort: 443, APIPort: 8899})
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.ctrl.count(); got != 2 {
		t.Errorf("restarts = %d after identical settings, want 2", got)
	}
}

func TestRestartFailureIsReportedNotFatal(t *testing.T) {
	e := setup(t)
	e.ctrl.err = errors.New("s6-svc not found")
	var h model.Host
	res, err := e.a.Apply(context.Background(), func(tx *store.Tx) (err error) {
		h, err = tx.CreateHost(context.Background(), newHost("a.example.com"))
		return err
	})
	if err != nil {
		t.Fatalf("Apply failed because Restart failed: %v", err)
	}
	if res.RestartErr == nil || !res.Restarted {
		t.Errorf("result %+v does not report the failed restart", res)
	}
	if _, err := e.st.GetHost(context.Background(), h.ID); err != nil {
		t.Errorf("host not stored: %v", err)
	}
}

func TestStartRegeneratesAndRepairs(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.create(t, "a.example.com")
	e.create(t, "b.example.com")
	wantDyn, wantStatic := e.read(t, e.dynamicPath()), e.read(t, e.staticPath())

	// Deleted files, and an extra file the Panel does not own.
	other := filepath.Join(e.data, DynamicDir, "user.yml")
	if err := os.WriteFile(other, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(e.data, StaticFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.dynamicPath()); err != nil {
		t.Fatal(err)
	}
	if err := e.a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if e.read(t, e.dynamicPath()) != wantDyn || e.read(t, e.staticPath()) != wantStatic {
		t.Error("Start did not regenerate the deleted files")
	}
	if e.read(t, other) != "# mine\n" {
		t.Error("Start touched a file it does not own")
	}

	// A corrupted file.
	if err := os.WriteFile(e.dynamicPath(), []byte("http: [unclosed\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := e.a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if e.read(t, e.dynamicPath()) != wantDyn {
		t.Error("Start did not repair a corrupted panel.yml")
	}

	// Whole data directory gone (a restored database without generated files).
	if err := os.RemoveAll(e.data); err != nil {
		t.Fatal(err)
	}
	if err := e.a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if e.read(t, e.dynamicPath()) != wantDyn {
		t.Error("Start did not recreate a missing data directory")
	}
}

func TestStartFailsLoudly(t *testing.T) {
	e := setup(t)
	// A regular file where the data directory should be.
	if err := os.WriteFile(e.data, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.a.Start(context.Background()); err == nil {
		t.Error("Start succeeded although the files cannot be written")
	}
}

func TestWriteFailureAfterCommitIsReportedAndRepairedLater(t *testing.T) {
	e := setup(t)
	if err := os.WriteFile(e.data, []byte("x"), 0o644); err != nil { // data dir unusable
		t.Fatal(err)
	}
	_, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
		_, err := tx.CreateHost(context.Background(), newHost("a.example.com"))
		return err
	})
	if err == nil {
		t.Fatal("Apply hid a file write failure")
	}
	if hosts, _ := e.st.ListHosts(context.Background()); len(hosts) != 1 {
		t.Errorf("the committed change was lost: %d hosts", len(hosts))
	}
	if err := os.Remove(e.data); err != nil {
		t.Fatal(err)
	}
	if err := e.a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.read(t, e.dynamicPath()), "a.example.com") {
		t.Error("Start did not repair the files after the failed write")
	}
}

// While the temporary file exists, the watched directory must hold no file a
// Traefik provider would load other than the previous, complete panel.yml.
func TestAtomicWriteNeverExposesWatchedExtensions(t *testing.T) {
	e := setup(t)
	e.create(t, "a.example.com") // panel.yml exists, so the next write is a replace

	var seen atomic.Int32
	e.a.afterTempWritten = func(dir string) {
		seen.Add(1)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Error(err)
			return
		}
		var temps int
		for _, en := range entries {
			name := en.Name()
			if name == DynamicFile || name == StaticFile {
				// Still the old, parseable content, not a partial write.
				if dir == filepath.Join(e.data, DynamicDir) {
					var v map[string]any
					b, _ := os.ReadFile(filepath.Join(dir, name))
					if err := yaml.Unmarshal(b, &v); err != nil {
						t.Errorf("%s is not valid YAML mid-write: %v", name, err)
					}
				}
				continue
			}
			temps++
			for _, ext := range []string{".yml", ".yaml", ".toml"} {
				if strings.HasSuffix(name, ext) {
					t.Errorf("temporary file %q ends in %q; Traefik would load it", name, ext)
				}
			}
		}
		if temps == 0 {
			t.Error("expected a temporary file to exist during the write")
		}
	}
	e.create(t, "b.example.com")
	if seen.Load() == 0 {
		t.Fatal("the hook never ran")
	}
	// No temp file is left behind.
	entries, _ := os.ReadDir(filepath.Join(e.data, DynamicDir))
	if len(entries) != 1 || entries[0].Name() != DynamicFile {
		t.Errorf("dynamic dir holds %v after the write, want only panel.yml", entries)
	}
}

func TestConcurrentAppliesAreSerialized(t *testing.T) {
	e := setup(t)
	var inside, maxInside atomic.Int32
	var wg sync.WaitGroup
	const n = 12
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
				cur := inside.Add(1)
				defer inside.Add(-1)
				for {
					m := maxInside.Load()
					if cur <= m || maxInside.CompareAndSwap(m, cur) {
						break
					}
				}
				_, err := tx.CreateHost(context.Background(), newHost(fmt.Sprintf("h%d.example.com", i)))
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if maxInside.Load() != 1 {
		t.Errorf("%d change functions ran at once, want 1", maxInside.Load())
	}
	dyn := e.read(t, e.dynamicPath())
	for i := range n {
		if !strings.Contains(dyn, fmt.Sprintf("h%d.example.com", i)) {
			t.Errorf("host %d missing from the final panel.yml", i)
		}
	}
}

// The database serialises transactions on its single connection, so the mutex
// matters for what happens after commit: a second apply must not render and
// write while the first is still writing, or the older render could land last
// and leave the files stale.
func TestMutexCoversTheFileWrites(t *testing.T) {
	e := setup(t)
	e.create(t, "first.example.com") // so later writes go through the hook

	blocked := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	e.a.afterTempWritten = func(string) {
		once.Do(func() {
			close(blocked)
			<-release
		})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.create(t, "second.example.com") // blocks inside its file write
	}()
	<-blocked

	var secondRan atomic.Bool
	third := make(chan struct{})
	go func() {
		defer close(third)
		_, err := e.a.Apply(context.Background(), func(tx *store.Tx) error {
			secondRan.Store(true)
			_, err := tx.CreateHost(context.Background(), newHost("third.example.com"))
			return err
		})
		if err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-third:
		t.Fatal("a second apply finished while the first was still writing files")
	case <-time.After(300 * time.Millisecond):
	}
	if secondRan.Load() {
		t.Error("a second change function ran while the first apply was still writing files")
	}
	close(release)
	<-done
	<-third
	dyn := e.read(t, e.dynamicPath())
	for _, d := range []string{"first", "second", "third"} {
		if !strings.Contains(dyn, d+".example.com") {
			t.Errorf("%s missing from the final panel.yml:\n%s", d, dyn)
		}
	}
}
