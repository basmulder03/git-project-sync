package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ApplyError
// ---------------------------------------------------------------------------

func TestApplyErrorError(t *testing.T) {
	t.Parallel()

	e := ApplyError{Cause: errors.New("bang"), RollbackPerformed: true}
	if e.Error() != "bang" {
		t.Fatalf("unexpected error string: %q", e.Error())
	}

	e2 := ApplyError{}
	if e2.Error() != "update apply failed" {
		t.Fatalf("unexpected default error string: %q", e2.Error())
	}
}

func TestApplyErrorUnwrap(t *testing.T) {
	t.Parallel()

	inner := errors.New("inner")
	e := ApplyError{Cause: inner}
	if !errors.Is(e, inner) {
		t.Fatal("Unwrap should expose Cause")
	}
}

// ---------------------------------------------------------------------------
// artifactsForPlatform
// ---------------------------------------------------------------------------

func TestArtifactsForPlatform(t *testing.T) {
	t.Parallel()

	artifacts := []Artifact{
		{OS: "linux", Arch: "amd64", URL: "https://example/linux"},
		{OS: "windows", Arch: "amd64", URL: "https://example/windows"},
		{OS: "linux", Arch: "arm64", URL: "https://example/linux-arm"},
	}

	got := artifactsForPlatform(artifacts, "linux", "amd64")
	if len(got) != 1 || got[0].URL != "https://example/linux" {
		t.Fatalf("unexpected result: %+v", got)
	}

	none := artifactsForPlatform(artifacts, "darwin", "amd64")
	if len(none) != 0 {
		t.Fatalf("expected empty slice for darwin, got: %+v", none)
	}
}

// ---------------------------------------------------------------------------
// matchComponentArtifact
// ---------------------------------------------------------------------------

func TestMatchComponentArtifact(t *testing.T) {
	t.Parallel()

	artifacts := []Artifact{
		{Component: "syncd", OS: "linux", Arch: "amd64", URL: "https://example/syncd"},
		{Component: "syncctl", OS: "linux", Arch: "amd64", URL: "https://example/syncctl"},
		{Component: "", OS: "linux", Arch: "amd64", URL: "https://example/default"},
	}

	// Specific component match.
	a, ok := matchComponentArtifact(artifacts, "syncd", "linux", "amd64")
	if !ok || a.URL != "https://example/syncd" {
		t.Fatalf("expected syncd artifact, got: %+v ok=%v", a, ok)
	}

	// Empty component matches empty or "syncctl".
	a2, ok2 := matchComponentArtifact(artifacts, "", "linux", "amd64")
	if !ok2 {
		t.Fatal("expected match for empty component")
	}
	// Should have matched "syncctl" or the empty-component artifact.
	if a2.URL != "https://example/syncctl" && a2.URL != "https://example/default" {
		t.Fatalf("unexpected match: %+v", a2)
	}

	// No match.
	_, ok3 := matchComponentArtifact(artifacts, "syncd", "windows", "amd64")
	if ok3 {
		t.Fatal("expected no match for syncd on windows")
	}
}

// ---------------------------------------------------------------------------
// SelectCandidate
// ---------------------------------------------------------------------------

func TestSelectCandidateEmpty(t *testing.T) {
	t.Parallel()

	u := NewUpdater("v1.0.0")
	_, err := u.SelectCandidate(nil, "")
	if err == nil {
		t.Fatal("expected error for empty candidates")
	}
}

func TestSelectCandidateLatest(t *testing.T) {
	t.Parallel()

	candidates := []ReleaseCandidate{
		{Version: "v2.0.0"},
		{Version: "v1.5.0"},
	}
	u := NewUpdater("v1.0.0")
	got, err := u.SelectCandidate(candidates, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != "v2.0.0" {
		t.Fatalf("expected latest candidate, got: %s", got.Version)
	}
}

func TestSelectCandidateByVersion(t *testing.T) {
	t.Parallel()

	candidates := []ReleaseCandidate{
		{Version: "v2.0.0"},
		{Version: "v1.5.0"},
	}
	u := NewUpdater("v1.0.0")
	got, err := u.SelectCandidate(candidates, "v1.5.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != "v1.5.0" {
		t.Fatalf("expected v1.5.0, got: %s", got.Version)
	}
}

func TestSelectCandidateNotFound(t *testing.T) {
	t.Parallel()

	candidates := []ReleaseCandidate{{Version: "v2.0.0"}}
	u := NewUpdater("v1.0.0")
	_, err := u.SelectCandidate(candidates, "v9.9.9")
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

// ---------------------------------------------------------------------------
// compareVersion edge cases
// ---------------------------------------------------------------------------

func TestCompareVersionEdgeCases(t *testing.T) {
	t.Parallel()

	// Equal versions.
	if compareVersion("v1.2.3", "v1.2.3") != 0 {
		t.Fatal("equal versions should compare as 0")
	}
	// patch difference.
	if compareVersion("v1.2.4", "v1.2.3") <= 0 {
		t.Fatal("expected v1.2.4 > v1.2.3")
	}
	// major difference.
	if compareVersion("v2.0.0", "v1.9.9") <= 0 {
		t.Fatal("expected v2.0.0 > v1.9.9")
	}
	// rc vs rc.
	if compareVersion("v1.0.0-rc2", "v1.0.0-rc1") <= 0 {
		t.Fatal("expected rc2 > rc1")
	}
	// both invalid – falls back to string compare.
	result := compareVersion("bad", "bad")
	if result != 0 {
		t.Fatalf("identical invalid versions should compare as 0, got: %d", result)
	}
	// minor difference.
	if compareVersion("v1.3.0", "v1.2.9") <= 0 {
		t.Fatal("expected v1.3.0 > v1.2.9")
	}
}

// ---------------------------------------------------------------------------
// ApplyAll
// ---------------------------------------------------------------------------

func TestApplyAll(t *testing.T) {
	t.Parallel()

	artifactBytes := []byte("syncd-binary")
	sum := sha256.Sum256(artifactBytes)
	checksum := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(artifactBytes)
	}))
	defer server.Close()

	manifest := Manifest{
		Version: "v2.0.0",
		Channel: "stable",
		Artifacts: []Artifact{
			{
				Component: "syncd",
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
				URL:       server.URL + "/syncd",
				SHA256:    checksum,
			},
		},
	}

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "syncd")
	if err := os.WriteFile(targetPath, []byte("old"), 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}

	u := NewUpdater("v1.0.0")
	results := u.ApplyAll(context.Background(), manifest, map[string]string{"syncd": targetPath}, "v2.0.0")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("ApplyAll component error: %v", results[0].Err)
	}
	if results[0].Version != "v2.0.0" {
		t.Fatalf("unexpected version: %s", results[0].Version)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if string(got) != "syncd-binary" {
		t.Fatalf("unexpected content: %q", string(got))
	}
}

func TestApplyAllSkipsMissingComponent(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Version: "v2.0.0",
		Channel: "stable",
		Artifacts: []Artifact{
			{Component: "syncd", OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "http://example/syncd", SHA256: strings.Repeat("a", 64)},
		},
	}

	u := NewUpdater("v1.0.0")
	// Request a component not in the manifest – should be silently skipped.
	results := u.ApplyAll(context.Background(), manifest, map[string]string{"syncctl": "/tmp/syncctl"}, "v2.0.0")
	if len(results) != 0 {
		t.Fatalf("expected no results for missing component, got: %+v", results)
	}
}

// ---------------------------------------------------------------------------
// FetchManifest error paths
// ---------------------------------------------------------------------------

func TestFetchManifestNonOKStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := FetchManifest(context.Background(), server.URL+"/manifest.json")
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
}

func TestFetchManifestMissingFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"version": ""})
	}))
	defer server.Close()

	_, err := FetchManifest(context.Background(), server.URL+"/manifest.json")
	if err == nil {
		t.Fatal("expected error for manifest missing required fields")
	}
}

// ---------------------------------------------------------------------------
// FilterNewer – empty current version
// ---------------------------------------------------------------------------

func TestFilterNewerEmptyCurrentVersion(t *testing.T) {
	t.Parallel()

	candidates := []ReleaseCandidate{{Version: "v1.0.0"}, {Version: "v2.0.0"}}
	u := NewUpdater("")
	got := u.FilterNewer(candidates)
	if len(got) != len(candidates) {
		t.Fatalf("expected all candidates returned when current version is empty, got %d", len(got))
	}
}
