package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Updater struct {
	CurrentVersion string
	Client         *http.Client
}

type CheckResult struct {
	Available bool
	Manifest  Manifest
	Artifact  Artifact
}

func NewUpdater(currentVersion string) *Updater {
	return &Updater{CurrentVersion: currentVersion, Client: http.DefaultClient}
}

func (u *Updater) Check(ctx context.Context, manifestURL, channel string) (CheckResult, error) {
	manifest, err := FetchManifest(ctx, manifestURL)
	if err != nil {
		return CheckResult{}, err
	}

	if channel != "" && !strings.EqualFold(channel, manifest.Channel) {
		return CheckResult{Available: false, Manifest: manifest}, nil
	}

	artifact, ok := matchArtifact(manifest.Artifacts, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return CheckResult{}, fmt.Errorf("no artifact for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	available := u.CurrentVersion == "" || manifest.Version != u.CurrentVersion
	return CheckResult{Available: available, Manifest: manifest, Artifact: artifact}, nil
}

func (u *Updater) DownloadAndVerify(ctx context.Context, artifact Artifact, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return err
	}

	resp, err := u.Client.Do(req)
	if err != nil {
		return fmt.Errorf("download update artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download artifact returned status %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	if _, err := io.Copy(writer, resp.Body); err != nil {
		return fmt.Errorf("copy artifact bytes: %w", err)
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, artifact.SHA256) {
		return fmt.Errorf("checksum verification failed: expected %s got %s", artifact.SHA256, actual)
	}

	return nil
}

func (u *Updater) Apply(ctx context.Context, artifact Artifact, targetBinaryPath string, version string) (ApplyResult, error) {
	tmp, err := os.CreateTemp(filepath.Dir(targetBinaryPath), "sync-update-*")
	if err != nil {
		return ApplyResult{}, fmt.Errorf("create temp artifact file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := u.DownloadAndVerify(ctx, artifact, tmpPath); err != nil {
		return ApplyResult{}, err
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("mark downloaded artifact executable: %w", err)
	}

	if err := ReplaceBinaryWithRollback(targetBinaryPath, tmpPath); err != nil {
		return ApplyResult{}, err
	}

	return ApplyResult{TargetPath: targetBinaryPath, Version: version}, nil
}

func matchArtifact(artifacts []Artifact, osName, arch string) (Artifact, bool) {
	for _, artifact := range artifacts {
		if strings.EqualFold(artifact.OS, osName) && strings.EqualFold(artifact.Arch, arch) {
			return artifact, true
		}
	}
	return Artifact{}, false
}
