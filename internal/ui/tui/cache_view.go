package tui

import (
	"fmt"
	"strings"
)

func RenderCacheView(b *strings.Builder, cacheRows []CacheRow) {
	if len(cacheRows) == 0 {
		fmt.Fprintf(b, "No cache metrics available.\n")
		return
	}

	fmt.Fprintf(b, "Cache:\n")
	for _, row := range cacheRows {
		freshness := "stale"
		if row.TTL > 0 && row.Age <= row.TTL {
			freshness = "fresh"
		}
		fmt.Fprintf(b, "- %s | age=%s | ttl=%s | %s\n", row.Name, row.Age.Truncate(1), row.TTL.Truncate(1), freshness)
	}
}

func blankIf(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
