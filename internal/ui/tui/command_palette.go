package tui

import (
	"fmt"
	"sort"
	"strings"
)

type PaletteEntry struct {
	Command     string
	Description string
}

type CommandPalette struct {
	active  bool
	input   string
	entries []PaletteEntry
}

func NewCommandPalette() *CommandPalette {
	return &CommandPalette{entries: defaultPaletteEntries()}
}

func (p *CommandPalette) Active() bool {
	return p.active
}

func (p *CommandPalette) Open() {
	p.active = true
	p.input = ""
}

func (p *CommandPalette) Close() {
	p.active = false
	p.input = ""
}

func (p *CommandPalette) HandleKey(key string) (command string, consumed bool, execute bool) {
	if !p.active {
		return "", false, false
	}

	switch key {
	case "esc":
		p.Close()
		return "", true, false
	case "enter":
		command = strings.TrimSpace(p.input)
		p.Close()
		if command == "" {
			return "", true, false
		}
		return command, true, true
	case "backspace":
		if len(p.input) > 0 {
			p.input = p.input[:len(p.input)-1]
		}
		return "", true, false
	}

	if len(key) == 1 {
		p.input += key
		return "", true, false
	}

	return "", false, false
}

func (p *CommandPalette) Render() string {
	if !p.active {
		return ""
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "\nCommand Palette (/ to open, esc to close, enter to run)\n")
	fmt.Fprintf(b, "> %s\n", p.input)

	matches := p.matches()
	if len(matches) == 0 {
		fmt.Fprintf(b, "- no matches\n")
		return b.String()
	}

	limit := 7
	if len(matches) < limit {
		limit = len(matches)
	}
	for i := 0; i < limit; i++ {
		entry := matches[i]
		fmt.Fprintf(b, "- %s | %s\n", entry.Command, entry.Description)
	}

	return b.String()
}

func (p *CommandPalette) matches() []PaletteEntry {
	needle := strings.ToLower(strings.TrimSpace(p.input))
	if needle == "" {
		return p.entries
	}

	out := make([]PaletteEntry, 0)
	for _, entry := range p.entries {
		combined := strings.ToLower(entry.Command + " " + entry.Description)
		if strings.Contains(combined, needle) {
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Command) < len(out[j].Command)
	})
	return out
}

func defaultPaletteEntries() []PaletteEntry {
	return []PaletteEntry{
		{Command: "sync all", Description: "run sync for all repos"},
		{Command: "cache refresh all", Description: "refresh provider and branch cache"},
		{Command: "cache clear all", Description: "clear provider and branch cache"},
		{Command: "stats show", Description: "show runtime metrics summary"},
		{Command: "events list", Description: "show recent event history"},
		{Command: "trace show latest", Description: "show newest trace details"},
		{Command: "repo list", Description: "list configured repositories"},
		{Command: "source list", Description: "list configured sources"},
		{Command: "config show", Description: "show resolved configuration"},
		{Command: "workspace show", Description: "show workspace settings"},
		{Command: "daemon status", Description: "show daemon health snapshot"},
		{Command: "state check", Description: "check sqlite integrity"},
	}
}
