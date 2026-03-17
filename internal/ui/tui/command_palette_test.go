package tui

import (
	"strings"
	"testing"
)

func TestCommandPaletteExecuteFlow(t *testing.T) {
	p := NewCommandPalette()
	p.Open()

	for _, key := range []string{"s", "y", "n", "c", " ", "a", "l", "l"} {
		if _, consumed, execute := p.HandleKey(key); !consumed || execute {
			t.Fatalf("expected typing key %q to be consumed without execute", key)
		}
	}

	command, consumed, execute := p.HandleKey("enter")
	if !consumed || !execute {
		t.Fatalf("expected enter to trigger execute, consumed=%t execute=%t", consumed, execute)
	}
	if command != "sync all" {
		t.Fatalf("unexpected command %q", command)
	}
	if p.Active() {
		t.Fatal("palette should close after execute")
	}
}

func TestCommandPaletteEscapeCloses(t *testing.T) {
	p := NewCommandPalette()
	p.Open()
	if _, consumed, execute := p.HandleKey("esc"); !consumed || execute {
		t.Fatalf("expected esc to close palette, consumed=%t execute=%t", consumed, execute)
	}
	if p.Active() {
		t.Fatal("palette should be closed after esc")
	}
}

func TestCommandPaletteTabAutocompleteUsesSelection(t *testing.T) {
	t.Parallel()

	p := NewCommandPalette()
	p.Open()
	_, consumed, execute := p.HandleKey("d")
	if !consumed || execute {
		t.Fatal("expected typing to be consumed")
	}
	_, consumed, execute = p.HandleKey("tab")
	if !consumed || execute {
		t.Fatal("expected tab autocomplete to be consumed")
	}
	command, consumed, execute := p.HandleKey("enter")
	if !consumed || !execute {
		t.Fatal("expected enter to execute selected command")
	}
	if command == "" {
		t.Fatal("expected non-empty selected command")
	}
}

func TestCommandPaletteHasCoreCommandGroups(t *testing.T) {
	t.Parallel()

	entries := defaultPaletteEntries()
	have := make(map[string]bool)
	for _, entry := range entries {
		parts := strings.Fields(entry.Command)
		if len(parts) == 0 {
			continue
		}
		have[parts[0]] = true
	}

	for _, group := range []string{
		"doctor", "source", "repo", "workspace", "sync", "discover",
		"daemon", "config", "auth", "cache", "stats", "events",
		"trace", "state", "maintenance", "update", "install", "uninstall", "service",
	} {
		if !have[group] {
			t.Fatalf("palette missing command group %q", group)
		}
	}
}
