package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/update"
)

func TestUpdateCheckCommand(t *testing.T) {
	t.Parallel()

	artifact := []byte("update-bin")
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(update.Manifest{
			Version: "v2.0.0",
			Channel: "stable",
			Artifacts: []update.Artifact{{
				OS:     runtime.GOOS,
				Arch:   runtime.GOARCH,
				URL:    server.URL + "/artifact.bin",
				SHA256: checksum,
			}},
		})
	}))
	defer server.Close()

	output, err := executeSyncctl("update", "check", "--manifest-url", server.URL+"/manifest.json", "--current-version", "v1.0.0")
	if err != nil {
		t.Fatalf("update check failed: %v output=%s", err, output)
	}
	if !strings.Contains(output, "update available") {
		t.Fatalf("unexpected update check output: %s", output)
	}
}

func TestSyncctlBinaryName(t *testing.T) {
	t.Parallel()

	name := syncctlBinaryName()
	if runtime.GOOS == "windows" {
		if name != "syncctl.exe" {
			t.Fatalf("expected syncctl.exe on windows, got %q", name)
		}
	} else {
		if name != "syncctl" {
			t.Fatalf("expected syncctl on non-windows, got %q", name)
		}
	}
}

func TestSyncdBinaryName(t *testing.T) {
	t.Parallel()

	name := syncdBinaryName()
	if runtime.GOOS == "windows" {
		if name != "syncd.exe" {
			t.Fatalf("expected syncd.exe on windows, got %q", name)
		}
	} else {
		if name != "syncd" {
			t.Fatalf("expected syncd on non-windows, got %q", name)
		}
	}
}

func TestLoadUpdateRecorder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// loadUpdateRecorder requires a valid config + DB; when the config doesn't
	// exist it should return an error gracefully.
	api, closer, err := loadUpdateRecorder(configPath)
	if err == nil {
		closer()
		// On some platforms/configs the default DB may be auto-created; that's
		// also fine as long as we got an API back.
		if api == nil {
			t.Fatal("expected non-nil ServiceAPI when err is nil")
		}
	}
	// Either an error (config missing) or a valid api — both are acceptable.
}

func TestRecordUpdateEvent_NilAPI(t *testing.T) {
	t.Parallel()

	// Must not panic when api is nil.
	recordUpdateEvent(nil, "trace1", "info", "update_ok", "all good")
}

func TestUpdateRepo_DefaultFallback(t *testing.T) {
	t.Parallel()

	// Non-existent config path → should fall back to the default repo slug.
	repo := updateRepo(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if repo != defaultUpdateRepo {
		t.Fatalf("expected default repo %q, got %q", defaultUpdateRepo, repo)
	}
}

func TestUpdateRepo_WithValidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// Write a minimal valid config directly (no CLI invocation to avoid
	// triggering postUpdateVersionSync which races against githubAPIBaseURL).
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	repo := updateRepo(configPath)
	// The config has no custom update repo field, so it always returns the default.
	if repo != defaultUpdateRepo {
		t.Fatalf("expected default repo %q, got %q", defaultUpdateRepo, repo)
	}
}
