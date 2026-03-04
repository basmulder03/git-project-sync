package telemetry

import "time"

type EventSummary struct {
	ErrorsLastHour int
	WarnsLastHour  int
	TotalLastHour  int
}

func SummarizeRecentEvents(events []Event, now time.Time) EventSummary {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	summary := EventSummary{}
	cutoff := now.Add(-1 * time.Hour)
	for _, event := range events {
		if event.CreatedAt.Before(cutoff) {
			continue
		}
		summary.TotalLastHour++
		switch event.Level {
		case "error":
			summary.ErrorsLastHour++
		case "warn", "warning":
			summary.WarnsLastHour++
		}
	}

	return summary
}

func HealthScore(criticalCount, warningCount int) int {
	score := 100 - (criticalCount * 30) - (warningCount * 10)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}
