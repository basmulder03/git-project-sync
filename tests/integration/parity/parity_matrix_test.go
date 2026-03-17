package parity

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/basmulder03/git-project-sync/internal/ui/tui"
	"gopkg.in/yaml.v3"
)

type matrixFile struct {
	Version int `yaml:"version"`
	Groups  []struct {
		Group      string `yaml:"group"`
		CLIExample string `yaml:"cli_example"`
		TUIPalette string `yaml:"tui_palette"`
		TUIGuided  string `yaml:"tui_guided"`
	} `yaml:"groups"`
}

func TestParityMatrixCoversSyncctlTopLevelGroups(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	matrix := loadMatrix(t, filepath.Join(root, "docs", "reference", "cli-tui-parity-matrix.yaml"))
	fromMatrix := map[string]struct{}{}
	for _, g := range matrix.Groups {
		name := strings.TrimSpace(g.Group)
		if name == "" {
			t.Fatal("matrix contains empty group")
		}
		if strings.TrimSpace(g.CLIExample) == "" || strings.TrimSpace(g.TUIPalette) == "" || strings.TrimSpace(g.TUIGuided) == "" {
			t.Fatalf("matrix group %q has empty fields", name)
		}
		fromMatrix[name] = struct{}{}
	}

	fromSyncctl := topLevelGroupsFromSyncctlMain(t, filepath.Join(root, "cmd", "syncctl", "main.go"))
	for _, group := range fromSyncctl {
		if _, ok := fromMatrix[group]; !ok {
			t.Fatalf("parity matrix missing syncctl command group %q", group)
		}
	}
}

func TestParityMatrixPaletteCommandsExist(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	matrix := loadMatrix(t, filepath.Join(root, "docs", "reference", "cli-tui-parity-matrix.yaml"))

	palette := tui.PaletteCatalog()
	have := map[string]struct{}{}
	for _, entry := range palette {
		have[strings.TrimSpace(entry.Command)] = struct{}{}
	}

	for _, group := range matrix.Groups {
		cmd := strings.TrimSpace(group.TUIPalette)
		if _, ok := have[cmd]; !ok {
			t.Fatalf("matrix maps group %q to missing palette command %q", group.Group, cmd)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}

func loadMatrix(t *testing.T, path string) matrixFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	var m matrixFile
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse matrix yaml: %v", err)
	}
	if m.Version != 1 {
		t.Fatalf("unexpected matrix version %d", m.Version)
	}
	if len(m.Groups) == 0 {
		t.Fatal("matrix groups are empty")
	}
	return m
}

func topLevelGroupsFromSyncctlMain(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read syncctl main: %v", err)
	}
	s := string(data)
	start := strings.Index(s, "root.AddCommand(")
	if start < 0 {
		t.Fatal("root.AddCommand block not found")
	}
	end := strings.Index(s[start:], ")\n\n\treturn root")
	if end < 0 {
		t.Fatal("root.AddCommand block end not found")
	}
	block := s[start : start+end]
	re := regexp.MustCompile(`new([A-Za-z0-9]+)Command`)
	matches := re.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		t.Fatal("no top-level commands found in root.AddCommand")
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := strings.ToLower(strings.TrimSpace(m[1]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
