package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceBinaryWithRollbackSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "syncd")
	candidate := filepath.Join(dir, "candidate")

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("new"), 0o755); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	if err := ReplaceBinaryWithRollback(target, candidate); err != nil {
		t.Fatalf("replace with rollback: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("target content=%q want new", string(got))
	}
}

func TestReplaceBinaryWithRollbackRestoresOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "syncd")
	candidate := filepath.Join(dir, "missing-candidate")

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	err := ReplaceBinaryWithRollback(target, candidate)
	if err == nil {
		t.Fatal("expected replace failure")
	}

	applyErr, ok := err.(ApplyError)
	if !ok {
		t.Fatalf("expected ApplyError, got %T", err)
	}
	if !applyErr.RollbackPerformed {
		t.Fatal("expected rollback to be performed")
	}

	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read rolled-back target: %v", readErr)
	}
	if string(got) != "old" {
		t.Fatalf("target content after rollback=%q want old", string(got))
	}
}
