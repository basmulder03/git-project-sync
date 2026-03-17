package tui

import (
	"fmt"
	"strings"
)

func RenderLogsView(b *strings.Builder, events []EventRow, recentErrors []string, levelFilter, reasonDetail string) {
	fmt.Fprintf(b, "%s %s\n", bold("Event Stream"), muted("(filter="+blankIf(levelFilter, "all")+")"))
	fmt.Fprintf(b, "%s\n", muted("TIME                 LEVEL      REASON              TRACE        MESSAGE"))
	fmt.Fprintf(b, "%s\n", muted(strings.Repeat("-", 98)))
	if len(events) == 0 {
		fmt.Fprintf(b, "  %s\n", muted("No events in this filter (press 'f' to cycle)."))
	} else {
		maxRows := len(events)
		if maxRows > 14 {
			maxRows = 14
		}
		for i := 0; i < maxRows; i++ {
			event := events[i]
			timestamp := "-"
			if !event.Time.IsZero() {
				timestamp = event.Time.UTC().Format("2006-01-02 15:04:05")
			}
			level := strings.ToLower(blankIf(event.Level, "info"))
			levelStyled := muted(strings.ToUpper(level))
			switch level {
			case "error":
				levelStyled = bad("ERROR")
			case "warn", "warning":
				levelStyled = warn("WARN")
			case "info":
				levelStyled = accent("INFO")
			}
			fmt.Fprintf(b, "%-20s %-10s %-18s %-12s %s\n",
				timestamp,
				levelStyled,
				clip(blankIf(event.ReasonCode, "-"), 18),
				clip(blankIf(event.TraceID, "-"), 12),
				clip(event.Message, 36),
			)
		}
		if len(events) > maxRows {
			fmt.Fprintf(b, "  %s\n", muted(fmt.Sprintf("... and %d older events", len(events)-maxRows)))
		}
	}

	if strings.TrimSpace(reasonDetail) != "" {
		fmt.Fprintf(b, "\n%s\n", bold("Reason drill-down"))
		fmt.Fprintf(b, "  %s\n", accent(reasonDetail))
	}

	fmt.Fprintf(b, "\n%s\n", bold("Recent Errors"))
	if len(recentErrors) == 0 {
		fmt.Fprintf(b, "  %s\n", good("none"))
		return
	}

	for i, entry := range recentErrors {
		if i >= 6 {
			fmt.Fprintf(b, "  %s\n", muted(fmt.Sprintf("... and %d more", len(recentErrors)-i)))
			break
		}
		fmt.Fprintf(b, "  %s %s\n", bad("!"), clip(strings.TrimSpace(entry), 96))
	}
}
