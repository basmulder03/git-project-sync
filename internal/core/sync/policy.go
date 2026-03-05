package sync

import (
	"regexp"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/config"
	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type policyDecision struct {
	allowed    bool
	reasonCode string
	reason     string
}

func evaluatePolicy(now time.Time, source config.SourceConfig, governance config.GovernanceConfig, repo config.RepoConfig) policyDecision {
	policy := governance.DefaultPolicy
	if override, ok := governance.SourcePolicies[source.ID]; ok {
		policy = mergePolicy(policy, override)
	}

	path := normalizeRepoPath(repo.Path)
	if len(policy.IncludeRepoPatterns) > 0 && !matchAnyPattern(path, policy.IncludeRepoPatterns) {
		return policyDecision{allowed: false, reasonCode: telemetry.ReasonPolicyRepoNotIncluded, reason: "repository is not in governance include patterns"}
	}
	if matchAnyPattern(path, policy.ExcludeRepoPatterns) {
		return policyDecision{allowed: false, reasonCode: telemetry.ReasonPolicyRepoExcluded, reason: "repository is excluded by governance policy"}
	}
	if matchAnyPattern(path, policy.ProtectedRepoPatterns) {
		return policyDecision{allowed: false, reasonCode: telemetry.ReasonPolicyRepoProtected, reason: "repository is protected by governance policy"}
	}
	if len(policy.AllowedSyncWindows) > 0 && !withinAnyWindow(now, policy.AllowedSyncWindows) {
		return policyDecision{allowed: false, reasonCode: telemetry.ReasonPolicyOutsideSyncWindow, reason: "current time is outside allowed governance sync windows"}
	}

	return policyDecision{allowed: true}
}

func mergePolicy(base, override config.SyncPolicyConfig) config.SyncPolicyConfig {
	result := base
	if len(override.IncludeRepoPatterns) > 0 {
		result.IncludeRepoPatterns = override.IncludeRepoPatterns
	}
	if len(override.ExcludeRepoPatterns) > 0 {
		result.ExcludeRepoPatterns = append(result.ExcludeRepoPatterns, override.ExcludeRepoPatterns...)
	}
	if len(override.ProtectedRepoPatterns) > 0 {
		result.ProtectedRepoPatterns = append(result.ProtectedRepoPatterns, override.ProtectedRepoPatterns...)
	}
	if len(override.AllowedSyncWindows) > 0 {
		result.AllowedSyncWindows = override.AllowedSyncWindows
	}
	return result
}

func normalizeRepoPath(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}

func matchAnyPattern(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func withinAnyWindow(now time.Time, windows []config.SyncWindowConfig) bool {
	for _, w := range windows {
		if withinWindow(now, w) {
			return true
		}
	}
	return false
}

func withinWindow(now time.Time, w config.SyncWindowConfig) bool {
	weekdayAllowed := false
	for _, day := range w.Days {
		wd, ok := mapDay(day)
		if ok && wd == now.Weekday() {
			weekdayAllowed = true
			break
		}
	}
	if !weekdayAllowed {
		return false
	}
	start, err := time.Parse("15:04", w.Start)
	if err != nil {
		return false
	}
	end, err := time.Parse("15:04", w.End)
	if err != nil {
		return false
	}
	current := time.Duration(now.Hour())*time.Hour + time.Duration(now.Minute())*time.Minute
	startD := time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute
	endD := time.Duration(end.Hour())*time.Hour + time.Duration(end.Minute())*time.Minute

	if startD <= endD {
		return current >= startD && current <= endD
	}
	return current >= startD || current <= endD
}

func mapDay(raw string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}
