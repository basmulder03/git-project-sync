package commands

import (
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestSetAndGetConfigValue(t *testing.T) {
	cfg := config.Default()

	if err := SetConfigValue(&cfg, "daemon.interval_seconds", "42"); err != nil {
		t.Fatalf("set config value: %v", err)
	}

	got, err := GetConfigValue(cfg, "daemon.interval_seconds")
	if err != nil {
		t.Fatalf("get config value: %v", err)
	}
	if got != "42" {
		t.Fatalf("unexpected interval value %q", got)
	}
}

func TestSetConfigValueRejectsBadInput(t *testing.T) {
	cfg := config.Default()
	if err := SetConfigValue(&cfg, "daemon.interval_seconds", "not-a-number"); err == nil {
		t.Fatal("expected integer parse error")
	}
	if err := SetConfigValue(&cfg, "unknown.key", "value"); err == nil {
		t.Fatal("expected unsupported key error")
	}
}
