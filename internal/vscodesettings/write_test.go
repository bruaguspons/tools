package vscodesettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	t.Run("creates dir and file, leaves no temp file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".vscode", "settings.json")
		if err := writeAtomic(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("writeAtomic() error = %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "{}\n" {
			t.Fatalf("content = %q, err = %v", got, err)
		}
		entries, _ := os.ReadDir(filepath.Dir(path))
		if len(entries) != 1 || entries[0].Name() != "settings.json" {
			t.Errorf("dir entries = %v, want only settings.json", entries)
		}
	})

	t.Run("applies the requested mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := writeAtomic(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("writeAtomic() error = %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("failure keeps original and leaves no temp file", func(t *testing.T) {
		// A non-empty directory at the target path makes the final rename
		// fail regardless of uid (root bypasses permission bits).
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		inner := filepath.Join(path, "keep")
		if err := os.WriteFile(inner, []byte("orig"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := writeAtomic(path, []byte("new"), 0o644); err == nil {
			t.Fatal("writeAtomic() error = nil, want failure")
		}
		got, err := os.ReadFile(inner)
		if err != nil || string(got) != "orig" {
			t.Errorf("inner content = %q, err = %v, want original", got, err)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 || entries[0].Name() != "settings.json" {
			t.Errorf("dir entries = %v, want only settings.json", entries)
		}
	})
}
