package tui

import "testing"

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
