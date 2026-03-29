package audit

import (
	"context"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// RetentionScheduler periodically deletes expired audit logs.
type RetentionScheduler struct {
	repo               *store.AuditRepo
	retentionDays      int
	routeRetentionDays map[string]int
	interval           time.Duration
	batchSize          int
	now                func() time.Time
}

func NewRetentionScheduler(repo *store.AuditRepo, cfg config.AuditConfig) *RetentionScheduler {
	if repo == nil || !cfg.Enabled || cfg.RetentionDays <= 0 {
		return nil
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = time.Hour
	}
	if cfg.CleanupBatchSize <= 0 {
		cfg.CleanupBatchSize = 1000
	}
	routeRetention := make(map[string]int, len(cfg.RouteRetentionDays))
	for route, days := range cfg.RouteRetentionDays {
		route = normalizeRouteKey(route)
		if route == "" || days <= 0 {
			continue
		}
		routeRetention[route] = days
	}
	return &RetentionScheduler{
		repo:               repo,
		retentionDays:      cfg.RetentionDays,
		routeRetentionDays: routeRetention,
		interval:           cfg.CleanupInterval,
		batchSize:          cfg.CleanupBatchSize,
		now:                time.Now,
	}
}

func (s *RetentionScheduler) Enabled() bool {
	return s != nil && s.repo != nil && s.retentionDays > 0
}

func (s *RetentionScheduler) Start(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.RunOnce()
		}
	}
}

func (s *RetentionScheduler) RunOnce() (int64, error) {
	if !s.Enabled() {
		return 0, nil
	}
	now := s.now().UTC()
	var deletedTotal int64

	if len(s.routeRetentionDays) > 0 {
		overrideRoutes := make([]string, 0, len(s.routeRetentionDays))
		for route, days := range s.routeRetentionDays {
			routeCutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
			deleted, err := s.repo.DeleteOlderThanForRoute(route, routeCutoff, s.batchSize)
			if err != nil {
				return deletedTotal, err
			}
			deletedTotal += deleted
			overrideRoutes = append(overrideRoutes, route)
		}

		defaultCutoff := now.Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
		deleted, err := s.repo.DeleteOlderThanExcludingRoutes(defaultCutoff, s.batchSize, overrideRoutes)
		if err != nil {
			return deletedTotal, err
		}
		deletedTotal += deleted
		return deletedTotal, nil
	}

	cutoff := now.Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	return s.repo.DeleteOlderThan(cutoff, s.batchSize)
}

func normalizeRouteKey(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}
	return strings.ToLower(route)
}
