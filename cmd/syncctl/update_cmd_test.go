package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

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
