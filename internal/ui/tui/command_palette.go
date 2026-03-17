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
	active   bool
	input    string
	selected int
	entries  []PaletteEntry
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
	p.selected = 0
}

func (p *CommandPalette) Close() {
	p.active = false
	p.input = ""
	p.selected = 0
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
		matches := p.matches()
		command = strings.TrimSpace(p.input)
		if len(matches) > 0 {
			if command == "" || !hasExactCommand(matches, command) {
				if p.selected < 0 {
					p.selected = 0
				}
				if p.selected >= len(matches) {
					p.selected = len(matches) - 1
				}
				command = matches[p.selected].Command
			}
		}
		p.Close()
		if command == "" {
			return "", true, false
		}
		return command, true, true
	case "up", "k":
		matches := p.matches()
		if len(matches) == 0 {
			return "", true, false
		}
		if p.selected > 0 {
			p.selected--
		}
		return "", true, false
	case "down", "j":
		matches := p.matches()
		if len(matches) == 0 {
			return "", true, false
		}
		if p.selected < len(matches)-1 {
			p.selected++
		}
		return "", true, false
	case "tab":
		matches := p.matches()
		if len(matches) == 0 {
			return "", true, false
		}
		if p.selected < 0 {
			p.selected = 0
		}
		if p.selected >= len(matches) {
			p.selected = len(matches) - 1
		}
		p.input = matches[p.selected].Command
		return "", true, false
	case "backspace":
		if len(p.input) > 0 {
			p.input = p.input[:len(p.input)-1]
		}
		p.selected = 0
		return "", true, false
	case "space":
		p.input += " "
		p.selected = 0
		return "", true, false
	}

	if len(key) == 1 {
		p.input += key
		p.selected = 0
		return "", true, false
	}

	return "", false, false
}

func (p *CommandPalette) Render(width, height int) string {
	if !p.active {
		return ""
	}
	if width <= 0 {
		width = 110
	}
	if height <= 0 {
		height = 36
	}

	inner := width - 24
	if inner > 84 {
		inner = 84
	}
	if inner < 44 {
		inner = 44
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "\n%s\n", bold(accent("Command Palette")))
	fmt.Fprintf(b, "%s\n", muted("Type to search, up/down to select, tab to autocomplete, enter to run"))
	fmt.Fprintf(b, "> %s\n", bold(p.input))

	matches := p.matches()
	if len(matches) == 0 {
		fmt.Fprintf(b, "  %s\n", muted("no matches"))
		return b.String()
	}

	limit := 9
	if len(matches) < limit {
		limit = len(matches)
	}
	for i := 0; i < limit; i++ {
		entry := matches[i]
		prefix := "  "
		cmd := clip(entry.Command, 28)
		desc := clip(entry.Description, 56)
		if i == p.selected {
			prefix = accent("> ")
			cmd = bold(accent(cmd))
		}
		fmt.Fprintf(b, "%s%-30s %s\n", prefix, cmd, muted(desc))
	}

	overlay := box("Command Palette", splitLines(b.String()), inner)
	indent := (width - inner) / 2
	if indent < 0 {
		indent = 0
	}
	topPad := height / 6
	if topPad < 1 {
		topPad = 1
	}
	if topPad > 8 {
		topPad = 8
	}
	prefix := strings.Repeat(" ", indent)
	cb := &strings.Builder{}
	cb.WriteString("\n")
	for i := 0; i < topPad; i++ {
		cb.WriteString("\n")
	}
	for _, line := range splitLines(overlay) {
		cb.WriteString(prefix)
		cb.WriteString(line)
		cb.WriteString("\n")
	}
	return cb.String()
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

func hasExactCommand(entries []PaletteEntry, command string) bool {
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.Command), strings.TrimSpace(command)) {
			return true
		}
	}
	return false
}

func defaultPaletteEntries() []PaletteEntry {
	return []PaletteEntry{
		{Command: "doctor", Description: "run a lightweight health snapshot"},
		{Command: "discover", Description: "scan workspace and summarize discovered repos"},
		{Command: "auth status", Description: "show source auth and SSH enablement"},
		{Command: "source list", Description: "list configured sources"},
		{Command: "repo list", Description: "list configured repositories"},
		{Command: "sync all", Description: "run sync for all repos"},
		{Command: "daemon status", Description: "show daemon health snapshot"},
		{Command: "maintenance status", Description: "show current maintenance-window state"},
		{Command: "cache refresh all", Description: "refresh provider and branch cache"},
		{Command: "cache clear all", Description: "clear provider and branch cache"},
		{Command: "cache show", Description: "show cache TTL settings"},
		{Command: "stats show", Description: "show runtime metrics summary"},
		{Command: "events list", Description: "show recent event history"},
		{Command: "trace show latest", Description: "show newest trace details"},
		{Command: "config show", Description: "show resolved configuration"},
		{Command: "workspace show", Description: "show workspace settings"},
		{Command: "install --user", Description: "install daemon service in user mode"},
		{Command: "uninstall --user", Description: "unregister daemon service in user mode"},
		{Command: "service register --user", Description: "register and start service in user mode"},
		{Command: "service unregister --user", Description: "unregister and stop service in user mode"},
		{Command: "update status", Description: "show update channel and automation settings"},
		{Command: "state check", Description: "check sqlite integrity"},
	}
}

// PaletteCatalog returns a copy of the command palette catalog.
func PaletteCatalog() []PaletteEntry {
	entries := defaultPaletteEntries()
	out := make([]PaletteEntry, len(entries))
	copy(out, entries)
	return out
}
