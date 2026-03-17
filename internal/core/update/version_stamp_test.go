package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteLastVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Non-existent file returns empty string without error.
	got, err := ReadLastVersion(dir)
	if err != nil {
		t.Fatalf("ReadLastVersion on empty dir: %v", err)
	}
	if got != "" {
		t.Fatalf("ReadLastVersion on empty dir = %q, want empty", got)
	}

	// Write a version and read it back.
	if err := WriteLastVersion(dir, "v1.2.3"); err != nil {
		t.Fatalf("WriteLastVersion: %v", err)
	}
	got, err = ReadLastVersion(dir)
	if err != nil {
		t.Fatalf("ReadLastVersion after write: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("ReadLastVersion = %q, want v1.2.3", got)
	}
}

func TestWriteLastVersion_CreatesDirectory(t *testing.T) {
	t.Parallel()

	// Use a non-existent sub-directory to verify MkdirAll behaviour.
	dir := filepath.Join(t.TempDir(), "sub", "dir")

	if err := WriteLastVersion(dir, "v2.0.0"); err != nil {
		t.Fatalf("WriteLastVersion: %v", err)
	}

	path := filepath.Join(dir, LastVersionFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stamp file should exist: %v", err)
	}
}

func TestWasUpdated(t *testing.T) {
	t.Parallel()

	t.Run("no stamp file – not an update", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		updated, err := WasUpdated(dir, "v1.0.0")
		if err != nil {
			t.Fatalf("WasUpdated: %v", err)
		}
		if updated {
			t.Fatal("expected false on first run (no stamp file)")
		}
	})

	t.Run("same version – not an update", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := WriteLastVersion(dir, "v1.0.0"); err != nil {
			t.Fatalf("WriteLastVersion: %v", err)
		}
		updated, err := WasUpdated(dir, "v1.0.0")
		if err != nil {
			t.Fatalf("WasUpdated: %v", err)
		}
		if updated {
			t.Fatal("expected false when versions match")
		}
	})

	t.Run("different version – is an update", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := WriteLastVersion(dir, "v1.0.0"); err != nil {
			t.Fatalf("WriteLastVersion: %v", err)
		}
		updated, err := WasUpdated(dir, "v1.1.0")
		if err != nil {
			t.Fatalf("WasUpdated: %v", err)
		}
		if !updated {
			t.Fatal("expected true when version changed")
		}
	})

	t.Run("v-prefix normalisation", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// Store without "v" prefix and compare with it – should still match.
		if err := WriteLastVersion(dir, "1.2.3"); err != nil {
			t.Fatalf("WriteLastVersion: %v", err)
		}
		updated, err := WasUpdated(dir, "v1.2.3")
		if err != nil {
			t.Fatalf("WasUpdated: %v", err)
		}
		if updated {
			t.Fatal("expected false – versions are equivalent after normalisation")
		}
	})
}
