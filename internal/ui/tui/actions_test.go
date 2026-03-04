package tui

import "testing"

func TestKeyToActionMappings(t *testing.T) {
	t.Parallel()

	status := DashboardStatus{Events: []EventRow{{TraceID: "trace-1"}}}

	if req, ok := KeyToAction("s", status); !ok || req.Type != ActionSyncAll {
		t.Fatalf("expected sync-all action, got ok=%t req=%+v", ok, req)
	}
	if req, ok := KeyToAction("c", status); !ok || req.Type != ActionCacheRefresh {
		t.Fatalf("expected cache-refresh action, got ok=%t req=%+v", ok, req)
	}
	if req, ok := KeyToAction("t", status); !ok || req.Type != ActionTraceDrilldown || req.TraceID != "trace-1" {
		t.Fatalf("expected trace action, got ok=%t req=%+v", ok, req)
	}
}
