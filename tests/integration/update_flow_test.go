package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/update"
)

func TestUpdateApplySuccessAndRollbackPaths(t *testing.T) {
	t.Parallel()

	artifactBytes := []byte("new-bin")
	sum := sha256.Sum256(artifactBytes)
	checksum := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifact.bin" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(artifactBytes)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "syncctl")
	if err := os.WriteFile(target, []byte("old-bin"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	updater := update.NewUpdater("v1.0.0")
	_, err := updater.Apply(context.Background(), update.Artifact{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		URL:    server.URL + "/artifact.bin",
		SHA256: checksum,
	}, target, "v2.0.0")
	if err != nil {
		t.Fatalf("apply success path failed: %v", err)
	}

	if got, err := os.ReadFile(target); err != nil || string(got) != "new-bin" {
		t.Fatalf("target not updated correctly: err=%v content=%q", err, string(got))
	}

	if err := os.WriteFile(target, []byte("rollback-source"), 0o755); err != nil {
		t.Fatalf("reset target: %v", err)
	}

	rollbackErr := update.ReplaceBinaryWithRollback(target, filepath.Join(t.TempDir(), "missing"))
	if rollbackErr == nil {
		t.Fatal("expected rollback path failure")
	}

	applyErr, ok := rollbackErr.(update.ApplyError)
	if !ok || !applyErr.RollbackPerformed {
		t.Fatalf("expected ApplyError with rollback=true, got %T %#v", rollbackErr, rollbackErr)
	}

	if got, err := os.ReadFile(target); err != nil || string(got) != "rollback-source" {
		t.Fatalf("target should be restored on rollback: err=%v content=%q", err, string(got))
	}
}
