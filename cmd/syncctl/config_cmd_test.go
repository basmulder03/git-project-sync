package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigInitShowValidateAndPath(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")

	out, err := executeSyncctl("--config", configPath, "config", "init")
	if err != nil {
		t.Fatalf("config init failed: %v output=%s", err, out)
	}
	if !strings.Contains(out, "config created") {
		t.Fatalf("unexpected init output: %s", out)
	}

	out, err = executeSyncctl("--config", configPath, "config", "show")
	if err != nil {
		t.Fatalf("config show failed: %v output=%s", err, out)
	}
	if !strings.Contains(out, "schema_version") {
		t.Fatalf("expected schema output, got: %s", out)
	}

	out, err = executeSyncctl("--config", configPath, "config", "validate")
	if err != nil {
		t.Fatalf("config validate failed: %v output=%s", err, out)
	}
	if !strings.Contains(out, "config valid") {
		t.Fatalf("unexpected validate output: %s", out)
	}

	out, err = executeSyncctl("--config", configPath, "config", "path")
	if err != nil {
		t.Fatalf("config path failed: %v output=%s", err, out)
	}
	if !strings.Contains(out, configPath) {
		t.Fatalf("expected config path output, got: %s", out)
	}
}

func TestConfigSetAndGet(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if out, err := executeSyncctl("--config", configPath, "config", "init"); err != nil {
		t.Fatalf("config init failed: %v output=%s", err, out)
	}

	if out, err := executeSyncctl("--config", configPath, "config", "set", "daemon.interval_seconds", "45"); err != nil {
		t.Fatalf("config set failed: %v output=%s", err, out)
	}

	out, err := executeSyncctl("--config", configPath, "config", "get", "daemon.interval_seconds")
	if err != nil {
		t.Fatalf("config get failed: %v output=%s", err, out)
	}
	if strings.TrimSpace(out) != "45" {
		t.Fatalf("expected interval 45, got %q", strings.TrimSpace(out))
	}
}
