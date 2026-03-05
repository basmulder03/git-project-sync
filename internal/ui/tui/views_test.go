package tui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderReposView(t *testing.T) {
	t.Parallel()

	b := &strings.Builder{}
	RenderReposView(b, []RepoRow{{Path: "/repos/a", LastStatus: "success", LastSyncAt: time.Now().UTC()}}, "all")
	if !strings.Contains(b.String(), "/repos/a") {
		t.Fatalf("repos view output missing repo path: %s", b.String())
	}
}

func TestRenderCacheView(t *testing.T) {
	t.Parallel()

	b := &strings.Builder{}
	RenderCacheView(b, []CacheRow{{Name: "providers", Age: 10 * time.Second, TTL: 60 * time.Second}})
	if !strings.Contains(b.String(), "providers") {
		t.Fatalf("cache view output missing cache row: %s", b.String())
	}
}

func TestRenderLogsView(t *testing.T) {
	t.Parallel()

	b := &strings.Builder{}
	RenderLogsView(b, []EventRow{{Time: time.Now().UTC(), TraceID: "t1", Level: "warn", ReasonCode: "repo_dirty", Message: "skipped"}}, []string{"boom"}, "warn", "reason=repo_dirty occurrences=1")
	if !strings.Contains(b.String(), "trace=t1") {
		t.Fatalf("logs view output missing trace id: %s", b.String())
	}
}

func TestRenderReposViewShowsFilterEmptyState(t *testing.T) {
	t.Parallel()

	b := &strings.Builder{}
	RenderReposView(b, nil, "error")
	if !strings.Contains(b.String(), "match filter=error") {
		t.Fatalf("expected filter-aware empty state: %s", b.String())
	}
}
