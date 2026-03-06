package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestSourceAddListShowRemoveFlow(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	if output, err := executeSyncctl("--config", configPath, "source", "add", "github", "gh-personal", "--account", "jane-doe"); err != nil {
		t.Fatalf("source add failed: %v, output=%s", err, output)
	}

	listOutput, err := executeSyncctl("--config", configPath, "source", "list")
	if err != nil {
		t.Fatalf("source list failed: %v", err)
	}
	if !strings.Contains(listOutput, "gh-personal") {
		t.Fatalf("list output missing source id: %s", listOutput)
	}

	showOutput, err := executeSyncctl("--config", configPath, "source", "show", "gh-personal")
	if err != nil {
		t.Fatalf("source show failed: %v", err)
	}
	if !strings.Contains(showOutput, "account: jane-doe") {
		t.Fatalf("show output missing account: %s", showOutput)
	}

	if output, err := executeSyncctl("--config", configPath, "source", "remove", "gh-personal"); err != nil {
		t.Fatalf("source remove failed: %v, output=%s", err, output)
	}
}

func TestDefaultHost(t *testing.T) {
	t.Parallel()

	if got := defaultHost("github"); got != "github.com" {
		t.Fatalf("defaultHost(github) = %q, want github.com", got)
	}
	if got := defaultHost("azure"); got != "dev.azure.com" {
		t.Fatalf("defaultHost(azure) = %q, want dev.azure.com", got)
	}
}

func TestSourceAddCreatesConfigWhenMissing(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "new-config.yaml")

	output, err := executeSyncctl("--config", configPath, "source", "add", "github", "gh-personal", "--account", "jane-doe")
	if err != nil {
		t.Fatalf("source add failed: %v, output=%s", err, output)
	}

	if _, err := config.Load(configPath); err != nil {
		t.Fatalf("expected config to be created and valid: %v", err)
	}
}

func executeSyncctl(args ...string) (string, error) {
	cmd := newRootCommand()
	buffer := &bytes.Buffer{}
	cmd.SetOut(buffer)
	cmd.SetErr(buffer)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil && buffer.Len() == 0 {
		buffer.WriteString(formatCLIError(err))
	}
	return buffer.String(), err
}
