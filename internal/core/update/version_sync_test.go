package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubProbe returns a ComponentVersionFunc that serves pre-canned versions.
func stubProbe(versions map[string]string) ComponentVersionFunc {
	return func(_ string, component string) (string, error) {
		v, ok := versions[component]
		if !ok {
			return "", fmt.Errorf("component %q not found", component)
		}
		return v, nil
	}
}

func TestVersionSyncReport_InSync(t *testing.T) {
	t.Parallel()

	t.Run("all in sync", func(t *testing.T) {
		t.Parallel()
		r := VersionSyncReport{
			CLIVersion: "v1.2.3",
			Components: map[string]string{"syncd": "v1.2.3", "synctui": "v1.2.3"},
		}
		if !r.InSync() {
			t.Fatal("expected InSync() = true")
		}
	})

	t.Run("one out of sync", func(t *testing.T) {
		t.Parallel()
		r := VersionSyncReport{
			CLIVersion: "v1.2.3",
			Components: map[string]string{"syncd": "v1.1.0", "synctui": "v1.2.3"},
			OutOfSync:  []string{"syncd"},
		}
		if r.InSync() {
			t.Fatal("expected InSync() = false")
		}
	})
}

func TestVersionSyncer_Check_AllInSync(t *testing.T) {
	t.Parallel()

	syncer := &VersionSyncer{
		CLIVersion: "v2.0.0",
		Components: []string{"syncd", "synctui"},
		ProbeVersion: stubProbe(map[string]string{
			"syncd":   "v2.0.0",
			"synctui": "v2.0.0",
		}),
	}

	report, err := syncer.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if !report.InSync() {
		t.Fatalf("expected all in sync, out-of-sync: %v", report.OutOfSync)
	}
	if report.CLIVersion != "v2.0.0" {
		t.Fatalf("unexpected CLIVersion: %s", report.CLIVersion)
	}
}

func TestVersionSyncer_Check_OutOfSync(t *testing.T) {
	t.Parallel()

	syncer := &VersionSyncer{
		CLIVersion: "v2.1.0",
		Components: []string{"syncd", "synctui"},
		ProbeVersion: stubProbe(map[string]string{
			"syncd":   "v2.0.0", // outdated
			"synctui": "v2.1.0", // current
		}),
	}

	report, err := syncer.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if report.InSync() {
		t.Fatal("expected out-of-sync report")
	}
	if len(report.OutOfSync) != 1 || report.OutOfSync[0] != "syncd" {
		t.Fatalf("unexpected OutOfSync: %v", report.OutOfSync)
	}
}

func TestVersionSyncer_Check_MissingBinary(t *testing.T) {
	t.Parallel()

	// When a companion binary is not found the version is empty, which differs
	// from the CLI version and is therefore reported as out of sync.
	syncer := &VersionSyncer{
		CLIVersion: "v1.0.0",
		Components: []string{"syncd"},
		ProbeVersion: func(_ string, _ string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	report, err := syncer.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if report.InSync() {
		t.Fatal("missing binary should be out of sync")
	}
	if report.Components["syncd"] != "" {
		t.Fatalf("missing binary should have empty version, got %q", report.Components["syncd"])
	}
}

func TestVersionSyncer_Sync_AppliesOutOfSyncOnly(t *testing.T) {
	t.Parallel()

	// Build a fake artifact for the "syncd" component.
	artifactBytes := []byte("new-syncd-binary")
	sum := sha256.Sum256(artifactBytes)
	checksum := hex.EncodeToString(sum[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			releases := []map[string]interface{}{
				{
					"tag_name":   "v2.0.0",
					"prerelease": false,
					"assets": []map[string]interface{}{
						{"name": "manifest.json", "browser_download_url": server.URL + "/manifest.json"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(releases)
		case r.URL.Path == "/manifest.json":
			manifest := Manifest{
				Version: "v2.0.0",
				Channel: "stable",
				Artifacts: []Artifact{
					// ListCandidates uses matchArtifact which requires a "syncctl"
					// (or empty component) artifact to consider a release valid.
					{
						Component: "syncctl",
						OS:        runtime.GOOS,
						Arch:      runtime.GOARCH,
						URL:       server.URL + "/syncctl.bin",
						SHA256:    checksum,
					},
					{
						Component: "syncd",
						OS:        runtime.GOOS,
						Arch:      runtime.GOARCH,
						URL:       server.URL + "/syncd.bin",
						SHA256:    checksum,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case r.URL.Path == "/syncctl.bin":
			_, _ = w.Write(artifactBytes)
		case r.URL.Path == "/syncd.bin":
			_, _ = w.Write(artifactBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Create a temp target directory with an existing "syncd" binary placeholder.
	tmpDir := t.TempDir()
	syncdTarget := filepath.Join(tmpDir, binaryName("syncd"))
	if err := os.WriteFile(syncdTarget, []byte("old"), 0o755); err != nil {
		t.Fatalf("write placeholder: %v", err)
	}

	u := NewUpdater("v2.0.0")
	u.Client = server.Client()
	// Point the updater directly at the test server so we don't mutate the
	// shared package-level githubAPIBaseURL variable (which would cause a
	// data race when tests run in parallel).
	u.BaseURL = server.URL

	syncer := &VersionSyncer{
		CLIVersion: "v2.0.0",
		SiblingDir: tmpDir,
		Components: []string{"syncd", "synctui"},
		ProbeVersion: stubProbe(map[string]string{
			"syncd":   "v1.9.0", // outdated → will be synced
			"synctui": "v2.0.0", // already current → skipped
		}),
		Updater: u,
	}

	results := syncer.Sync(context.Background(), "owner/repo")

	// synctui was in sync so it should not be present in results.
	if _, present := results["synctui"]; present {
		t.Fatal("synctui was already in sync and should not appear in results")
	}

	// syncd was out of sync; the sync should have succeeded.
	syncErr, present := results["syncd"]
	if !present {
		t.Fatal("syncd was out of sync but no result recorded")
	}
	if syncErr != nil {
		t.Fatalf("syncd sync failed: %v", syncErr)
	}

	// The binary should now contain the updated content.
	got, err := os.ReadFile(syncdTarget)
	if err != nil {
		t.Fatalf("read replaced syncd: %v", err)
	}
	if string(got) != string(artifactBytes) {
		t.Fatalf("syncd content after sync = %q, want %q", string(got), string(artifactBytes))
	}
}

func TestVersionSyncer_Sync_AlreadyInSync(t *testing.T) {
	t.Parallel()

	syncer := &VersionSyncer{
		CLIVersion: "v1.0.0",
		Components: []string{"syncd"},
		ProbeVersion: stubProbe(map[string]string{
			"syncd": "v1.0.0",
		}),
	}

	results := syncer.Sync(context.Background(), "owner/repo")
	if len(results) != 0 {
		t.Fatalf("expected no results when already in sync, got %v", results)
	}
}
