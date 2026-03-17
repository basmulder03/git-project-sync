package tui

import (
	"fmt"
	"strings"
	"time"
)

func RenderCacheView(b *strings.Builder, cacheRows []CacheRow) {
	fmt.Fprintf(b, "%s\n", bold("Cache Health"))
	if len(cacheRows) == 0 {
		fmt.Fprintf(b, "  %s\n", muted("No cache metrics available."))
		return
	}

	fmt.Fprintf(b, "%s\n", muted("NAME         AGE      TTL      FRESHNESS"))
	fmt.Fprintf(b, "%s\n", muted(strings.Repeat("-", 70)))
	for _, row := range cacheRows {
		freshness := "stale"
		freshnessStyled := bad("STALE")
		ratio := 1.0
		if row.TTL > 0 && row.Age <= row.TTL {
			freshness = "fresh"
			freshnessStyled = good("FRESH")
			ratio = float64(row.Age) / float64(row.TTL)
		} else if row.TTL > 0 {
			ratio = 1.0
		}
		if freshness == "stale" && row.TTL > 0 {
			ratio = 1
		}
		fmt.Fprintf(b, "%-12s %-8s %-8s %-8s %s\n",
			clip(row.Name, 12),
			row.Age.Truncate(time.Second),
			row.TTL.Truncate(time.Second),
			freshnessStyled,
			bar(24, ratio),
		)
	}
}

func blankIf(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
