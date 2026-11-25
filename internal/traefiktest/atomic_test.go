package traefiktest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := writeFileAtomic(dir, "a.yml", []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(dir, "a.yml", []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.yml")); string(got) != "two" {
		t.Errorf("content = %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

// The temp file must never look like a config file to Traefik's file provider.
func TestTempNameIsNeutral(t *testing.T) {
	for _, name := range []string{"a.yml", "a.yaml", "a.toml", "a"} {
		tmp, err := os.CreateTemp(t.TempDir(), tempPattern(name))
		if err != nil {
			t.Fatal(err)
		}
		tmp.Close()
		for _, ext := range []string{".yml", ".yaml", ".toml"} {
			if strings.HasSuffix(tmp.Name(), ext) {
				t.Errorf("temp name %s ends in %s", tmp.Name(), ext)
			}
		}
	}
}

func TestWriteFileAtomicRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "../x.yml", "sub/x.yml", ".hidden.yml"} {
		if err := writeFileAtomic(t.TempDir(), name, nil, 0o644); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
}
