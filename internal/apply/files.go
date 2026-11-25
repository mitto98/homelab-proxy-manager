package apply

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// File modes: the data and dynamic directories are for the Panel's user and
// its group (Traefik runs in that group in the packages); config files are
// group-readable and never world-readable.
const (
	dirMode  os.FileMode = 0o750
	fileMode os.FileMode = 0o640
)

// writeIfChanged atomically replaces dir/name with content unless the file
// already holds exactly that. It reports whether it wrote.
//
// A reader, Traefik's file provider above all, must see either the old file or
// the whole new one. The content goes to a temporary file in the same
// directory (so the rename cannot cross filesystems), is synced, and is renamed
// into place. The temporary name ends in ".tmp": Traefik loads every .yml,
// .yaml and .toml file in the watched directory, so a temporary file with one of
// those endings could be parsed half-written, and one invalid file freezes every
// change (see internal/traefiktest).
func (a *Applier) writeIfChanged(dir, name string, content []byte) (bool, error) {
	path := filepath.Join(dir, name)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, content) {
		return false, nil
	}
	tmp, err := os.CreateTemp(dir, tempPattern(name))
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Chmod(fileMode); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if a.afterTempWritten != nil {
		a.afterTempWritten(dir)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return false, fmt.Errorf("replace %s: %w", path, err)
	}
	// Make the rename durable; best effort, some filesystems refuse to sync a directory.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return true, nil
}

// tempPattern is the os.CreateTemp pattern for name's temporary file. It never
// ends in a watched extension.
func tempPattern(name string) string { return "." + name + ".*.tmp" }
