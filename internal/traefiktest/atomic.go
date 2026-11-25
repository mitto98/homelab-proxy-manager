package traefiktest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tempPattern is the os.CreateTemp pattern for name's temp file.
func tempPattern(name string) string { return "." + name + ".*.tmp" }

// writeFileAtomic writes content to dir/name so a reader sees the old file or the
// whole new one, never a partial write.
//
// The temp name ends in ".tmp" on purpose. Traefik's file provider only loads
// files ending in .yml, .yaml or .toml, so a temp file with one of those endings in
// a watched directory could be parsed half-written. The final rename is a single
// directory event, which is what triggers the reload.
func writeFileAtomic(dir, name string, content []byte, perm os.FileMode) error {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid file name %q", name)
	}
	tmp, err := os.CreateTemp(dir, tempPattern(name))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}
