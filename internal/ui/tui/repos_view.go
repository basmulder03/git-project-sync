package tui

import (
	"fmt"
	"strings"
)

func RenderReposView(b *strings.Builder, repos []RepoRow, filter string) {
	fmt.Fprintf(b, "%s %s\n", bold("Repositories"), muted("(filter="+blankIf(filter, "all")+")"))
	fmt.Fprintf(b, "%s\n", muted("STATUS        LAST SYNC              PATH"))
	fmt.Fprintf(b, "%s\n", muted(strings.Repeat("-", 78)))

	if len(repos) == 0 {
		if strings.TrimSpace(filter) == "" || filter == "all" {
			fmt.Fprintf(b, "  %s\n", muted("No repository states yet."))
		} else {
			fmt.Fprintf(b, "  %s\n", muted("No repository states match filter="+filter+"."))
		}
		return
	}

	maxRows := len(repos)
	if maxRows > 15 {
		maxRows = 15
	}

	for i := 0; i < maxRows; i++ {
		repo := repos[i]
		syncAt := "never"
		if !repo.LastSyncAt.IsZero() {
			syncAt = repo.LastSyncAt.UTC().Format("2006-01-02 15:04:05")
		}

		line := fmt.Sprintf("%-13s %-22s %s", statusBadge(repo.LastStatus), syncAt, clip(repo.Path, 52))
		fmt.Fprintln(b, line)
		if strings.TrimSpace(repo.LastError) != "" {
			fmt.Fprintf(b, "               %s %s\n", bad("error:"), clip(strings.TrimSpace(repo.LastError), 84))
		}
	}

	if len(repos) > maxRows {
		fmt.Fprintf(b, "\n  %s\n", muted(fmt.Sprintf("... and %d more repositories", len(repos)-maxRows)))
	}
}
