package tui

import (
	"fmt"
	"strings"
	"time"
)

func RenderReposView(b *strings.Builder, repos []RepoRow, filter string) {
	if len(repos) == 0 {
		if strings.TrimSpace(filter) == "" || filter == "all" {
			fmt.Fprintf(b, "No repository states yet.\n")
		} else {
			fmt.Fprintf(b, "No repository states match filter=%s.\n", filter)
		}
		return
	}

	fmt.Fprintf(b, "Repo states (filter=%s):\n", blankIf(filter, "all"))
	for _, repo := range repos {
		syncAt := "never"
		if !repo.LastSyncAt.IsZero() {
			syncAt = repo.LastSyncAt.UTC().Format(time.RFC3339)
		}
		line := fmt.Sprintf("- %s | status=%s | sync=%s", repo.Path, blankIf(repo.LastStatus, "unknown"), syncAt)
		if repo.LastError != "" {
			line += " | error=" + repo.LastError
		}
		fmt.Fprintln(b, line)
	}
}
