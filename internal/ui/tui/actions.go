package tui

import "context"

type ActionType string

const (
	ActionSyncAll        ActionType = "sync_all"
	ActionCacheRefresh   ActionType = "cache_refresh"
	ActionTraceDrilldown ActionType = "trace_drilldown"
)

type ActionRequest struct {
	Type     ActionType
	RepoPath string
	TraceID  string
}

type ActionExecutor interface {
	Execute(ctx context.Context, request ActionRequest) (string, error)
}

func KeyToAction(key string, status DashboardStatus) (ActionRequest, bool) {
	switch key {
	case "s":
		return ActionRequest{Type: ActionSyncAll}, true
	case "c":
		return ActionRequest{Type: ActionCacheRefresh}, true
	case "t":
		traceID := ""
		if len(status.Events) > 0 {
			traceID = status.Events[0].TraceID
		}
		return ActionRequest{Type: ActionTraceDrilldown, TraceID: traceID}, true
	default:
		return ActionRequest{}, false
	}
}
