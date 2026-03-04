package tui

import (
	"fmt"
	"strings"
	"time"
)

func RenderLogsView(b *strings.Builder, events []EventRow, recentErrors []string) {
	fmt.Fprintf(b, "Recent events:\n")
	if len(events) == 0 {
		fmt.Fprintf(b, "- none\n")
	} else {
		for _, event := range events {
			timestamp := "-"
			if !event.Time.IsZero() {
				timestamp = event.Time.UTC().Format(time.RFC3339)
			}
			fmt.Fprintf(b, "- %s | trace=%s | %s/%s | %s\n", timestamp, blankIf(event.TraceID, "-"), blankIf(event.Level, "info"), blankIf(event.ReasonCode, "-"), event.Message)
		}
	}

	fmt.Fprintf(b, "\nRecent errors:\n")
	if len(recentErrors) == 0 {
		fmt.Fprintf(b, "- none\n")
		return
	}

	for _, entry := range recentErrors {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(entry))
	}
}
