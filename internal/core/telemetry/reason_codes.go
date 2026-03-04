package telemetry

import "strings"

const (
	ReasonUnknown              = "unknown"
	ReasonRepoDirty            = "repo_dirty"
	ReasonRepoConflicts        = "repo_conflicts"
	ReasonRepoStagedChanges    = "repo_staged_changes"
	ReasonRepoUnstagedChanges  = "repo_unstaged_changes"
	ReasonRepoUntrackedFiles   = "repo_untracked_files"
	ReasonNonFastForward       = "non_fast_forward"
	ReasonUpstreamMissing      = "upstream_missing"
	ReasonSourceMissing        = "source_missing"
	ReasonRetryBudgetExceeded  = "retry_budget_exceeded"
	ReasonProviderRateLimited  = "provider_rate_limited"
	ReasonPermanentError       = "permanent_error"
	ReasonNetworkError         = "network_error"
	ReasonTimeout              = "timeout"
	ReasonUpdateStarted        = "update_started"
	ReasonUpdateSucceeded      = "update_succeeded"
	ReasonUpdateFailed         = "update_failed"
	ReasonUpdateRollback       = "update_rollback"
	ReasonCleanupNotApplicable = "cleanup_not_applicable"
	ReasonCleanupNotMerged     = "cleanup_branch_not_merged"
	ReasonCleanupUniqueCommits = "cleanup_unique_commits_present"
)

func EnsureReasonCode(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ReasonUnknown
	}
	return trimmed
}
