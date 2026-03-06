package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/basmulder03/git-project-sync/internal/core/telemetry"
)

type CacheTarget string

const (
	CacheTargetProviders CacheTarget = "providers"
	CacheTargetBranches  CacheTarget = "branches"
	CacheTargetAll       CacheTarget = "all"
)

type EventRecorder interface {
	RecordEvent(ctx context.Context, event telemetry.Event) error
}

type CacheService struct {
	recorder EventRecorder
	now      func() time.Time
}

func NewCacheService(recorder EventRecorder) *CacheService {
	return &CacheService{recorder: recorder, now: func() time.Time { return time.Now().UTC() }}
}

func (s *CacheService) Refresh(ctx context.Context, target CacheTarget) error {
	return s.record(ctx, target, "refresh")
}

func (s *CacheService) Clear(ctx context.Context, target CacheTarget) error {
	return s.record(ctx, target, "clear")
}

func (s *CacheService) record(ctx context.Context, target CacheTarget, action string) error {
	if s.recorder == nil {
		return fmt.Errorf("event recorder is required")
	}
	now := s.now()
	traceID := fmt.Sprintf("cache-%d", now.UnixNano())

	targets := []CacheTarget{target}
	if target == CacheTargetAll {
		targets = []CacheTarget{CacheTargetProviders, CacheTargetBranches}
	}

	for _, current := range targets {
		reasonCode := fmt.Sprintf("cache_%s_%s", action, current)
		if err := s.recorder.RecordEvent(ctx, telemetry.Event{
			TraceID:    traceID,
			RepoPath:   "cache",
			Level:      "info",
			ReasonCode: reasonCode,
			Message:    fmt.Sprintf("cache %s executed for %s", action, current),
			CreatedAt:  now,
		}); err != nil {
			return err
		}
	}

	return nil
}

func LatestCacheEvents(events []telemetry.Event, target CacheTarget) (time.Time, time.Time) {
	var refreshed time.Time
	var cleared time.Time
	refreshCode := "cache_refresh_" + string(target)
	clearCode := "cache_clear_" + string(target)
	for _, event := range events {
		reasonCode := strings.TrimSpace(event.ReasonCode)
		switch reasonCode {
		case refreshCode:
			if event.CreatedAt.After(refreshed) {
				refreshed = event.CreatedAt
			}
		case clearCode:
			if event.CreatedAt.After(cleared) {
				cleared = event.CreatedAt
			}
		}
	}
	return refreshed, cleared
}
