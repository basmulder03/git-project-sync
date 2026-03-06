package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestCompareVersion(t *testing.T) {
	t.Parallel()
	if compareVersion("v1.2.0", "v1.1.9") <= 0 {
		t.Fatal("expected v1.2.0 > v1.1.9")
	}
	if compareVersion("v1.2.0", "v1.2.0-rc1") <= 0 {
		t.Fatal("expected stable > rc")
	}
}

func TestListCandidatesAndFilterNewer(t *testing.T) {
	t.Parallel()

	manifest := Manifest{Version: "v1.2.0", Channel: "stable", Artifacts: []Artifact{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "https://example/art", SHA256: strings.Repeat("a", 64)}}}
	manifestBytes, _ := json.Marshal(manifest)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/me/repo/releases"):
			_, _ = w.Write([]byte(`[{
				"tag_name":"v1.2.0",
				"prerelease":false,
				"assets":[{"name":"manifest.json","browser_download_url":"` + server.URL + `/manifest.json"}]
			}]`))
		case r.URL.Path == "/manifest.json":
			_, _ = w.Write(manifestBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updater := NewUpdater("v1.0.0")
	updater.Client = server.Client()
	prevBaseURL := githubAPIBaseURL
	githubAPIBaseURL = server.URL
	t.Cleanup(func() {
		githubAPIBaseURL = prevBaseURL
	})

	candidates, err := updater.ListCandidates(context.Background(), "me/repo", "stable", false)
	if err != nil {
		t.Fatalf("ListCandidates failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}

	newer := updater.FilterNewer(candidates)
	if len(newer) != 1 {
		t.Fatalf("expected one newer candidate, got %d", len(newer))
	}
}
