package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
)

var githubAPIBaseURL = "https://api.github.com"

type ReleaseCandidate struct {
	Version     string
	Prerelease  bool
	ManifestURL string
	Manifest    Manifest
	Artifact    Artifact
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (u *Updater) ListCandidates(ctx context.Context, repo, channel string, includePrerelease bool) ([]ReleaseCandidate, error) {
	if strings.TrimSpace(repo) == "" {
		return nil, fmt.Errorf("repo is required")
	}

	releases, err := u.fetchGitHubReleases(ctx, repo)
	if err != nil {
		return nil, err
	}

	candidates := make([]ReleaseCandidate, 0)
	seen := map[string]struct{}{}
	for _, release := range releases {
		if release.Prerelease && !includePrerelease {
			continue
		}

		manifestURL := ""
		for _, asset := range release.Assets {
			if strings.EqualFold(asset.Name, "manifest.json") {
				manifestURL = strings.TrimSpace(asset.BrowserDownloadURL)
				break
			}
		}
		if manifestURL == "" {
			continue
		}

		manifest, err := FetchManifest(ctx, manifestURL)
		if err != nil {
			continue
		}
		if channel != "" && !strings.EqualFold(channel, manifest.Channel) {
			continue
		}

		artifact, ok := matchArtifact(manifest.Artifacts, runtime.GOOS, runtime.GOARCH)
		if !ok {
			continue
		}

		version := normalizeVersion(manifest.Version)
		if version == "" {
			version = normalizeVersion(release.TagName)
		}
		if version == "" {
			continue
		}
		if _, ok := seen[version]; ok {
			continue
		}
		seen[version] = struct{}{}

		candidates = append(candidates, ReleaseCandidate{
			Version:     version,
			Prerelease:  release.Prerelease,
			ManifestURL: manifestURL,
			Manifest:    manifest,
			Artifact:    artifact,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersion(candidates[i].Version, candidates[j].Version) > 0
	})

	return candidates, nil
}

func (u *Updater) FilterNewer(candidates []ReleaseCandidate) []ReleaseCandidate {
	if normalizeVersion(u.CurrentVersion) == "" {
		return append([]ReleaseCandidate(nil), candidates...)
	}

	out := make([]ReleaseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if compareVersion(candidate.Version, u.CurrentVersion) > 0 {
			out = append(out, candidate)
		}
	}
	return out
}

func (u *Updater) SelectCandidate(candidates []ReleaseCandidate, desiredVersion string) (ReleaseCandidate, error) {
	if len(candidates) == 0 {
		return ReleaseCandidate{}, fmt.Errorf("no release candidates available")
	}

	desiredVersion = normalizeVersion(desiredVersion)
	if desiredVersion == "" {
		return candidates[0], nil
	}

	for _, candidate := range candidates {
		if normalizeVersion(candidate.Version) == desiredVersion {
			return candidate, nil
		}
	}

	return ReleaseCandidate{}, fmt.Errorf("version %q not found in available release candidates", desiredVersion)
}

func (u *Updater) fetchGitHubReleases(ctx context.Context, repo string) ([]githubRelease, error) {
	base, err := url.Parse(strings.TrimSpace(githubAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse github api base url: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + fmt.Sprintf("/repos/%s/releases", strings.TrimSpace(repo))
	q := base.Query()
	q.Set("per_page", "50")
	base.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "git-project-sync-syncctl")

	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch releases returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	return releases, nil
}

func normalizeVersion(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return ""
	}
	return "v" + v
}

func compareVersion(left, right string) int {
	l := parseVersion(left)
	r := parseVersion(right)
	if !l.valid || !r.valid {
		return strings.Compare(normalizeVersion(left), normalizeVersion(right))
	}

	if l.major != r.major {
		if l.major > r.major {
			return 1
		}
		return -1
	}
	if l.minor != r.minor {
		if l.minor > r.minor {
			return 1
		}
		return -1
	}
	if l.patch != r.patch {
		if l.patch > r.patch {
			return 1
		}
		return -1
	}

	if l.hasRC != r.hasRC {
		if l.hasRC {
			return -1
		}
		return 1
	}
	if l.rc != r.rc {
		if l.rc > r.rc {
			return 1
		}
		return -1
	}
	return 0
}

type parsedVersion struct {
	major int
	minor int
	patch int
	rc    int
	hasRC bool
	valid bool
}

func parseVersion(raw string) parsedVersion {
	v := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if v == "" {
		return parsedVersion{}
	}

	main := v
	rcPart := ""
	if idx := strings.Index(v, "-rc"); idx >= 0 {
		main = v[:idx]
		rcPart = v[idx+3:]
	}

	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return parsedVersion{}
	}

	var out parsedVersion
	if _, err := fmt.Sscanf(parts[0], "%d", &out.major); err != nil {
		return parsedVersion{}
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &out.minor); err != nil {
		return parsedVersion{}
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &out.patch); err != nil {
		return parsedVersion{}
	}
	if rcPart != "" {
		out.hasRC = true
		if _, err := fmt.Sscanf(rcPart, "%d", &out.rc); err != nil {
			return parsedVersion{}
		}
	}
	out.valid = true
	return out
}
