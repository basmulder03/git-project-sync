package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckAndDownloadVerify(t *testing.T) {
	t.Parallel()

	artifactBytes := []byte("binary-content")
	sum := sha256.Sum256(artifactBytes)
	checksum := hex.EncodeToString(sum[:])

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			manifest := Manifest{
				Version: "v9.9.9",
				Channel: "stable",
				Artifacts: []Artifact{{
					OS:     runtime.GOOS,
					Arch:   runtime.GOARCH,
					URL:    server.URL + "/artifact.bin",
					SHA256: checksum,
				}},
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case "/artifact.bin":
			_, _ = w.Write(artifactBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updater := NewUpdater("v1.0.0")
	result, err := updater.Check(context.Background(), server.URL+"/manifest.json", "stable")
	if err != nil {
		t.Fatalf("check update: %v", err)
	}
	if !result.Available {
		t.Fatal("expected update to be available")
	}

	output := filepath.Join(t.TempDir(), "artifact.bin")
	if err := updater.DownloadAndVerify(context.Background(), result.Artifact, output); err != nil {
		t.Fatalf("download and verify: %v", err)
	}

	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output artifact: %v", err)
	}
	if string(got) != string(artifactBytes) {
		t.Fatalf("downloaded artifact content mismatch")
	}
}

func TestApplyReplacesTargetBinary(t *testing.T) {
	t.Parallel()

	artifactBytes := []byte("new-binary")
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
	if err := os.WriteFile(target, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write old target: %v", err)
	}

	updater := NewUpdater("v1.0.0")
	result, err := updater.Apply(context.Background(), Artifact{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		URL:    server.URL + "/artifact.bin",
		SHA256: checksum,
	}, target, "v2.0.0")
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}

	if result.TargetPath != target || result.Version != "v2.0.0" {
		t.Fatalf("unexpected apply result: %+v", result)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("target content=%q want new-binary", string(got))
	}
}
