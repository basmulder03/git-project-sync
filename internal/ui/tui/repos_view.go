package tui

import (
	"fmt"
	"strings"
	"time"
)

func RenderReposView(b *strings.Builder, repos []RepoRow) {
	if len(repos) == 0 {
		fmt.Fprintf(b, "No repository states yet.\n")
		return
	}

	fmt.Fprintf(b, "Repo states:\n")
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
