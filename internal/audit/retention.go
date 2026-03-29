package audit

import (
	"context"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// RetentionScheduler periodically deletes expired audit logs.
type RetentionScheduler struct {
	repo          *store.AuditRepo
	retentionDays int
	interval      time.Duration
	batchSize     int
	now           func() time.Time
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
	return &RetentionScheduler{
		repo:          repo,
		retentionDays: cfg.RetentionDays,
		interval:      cfg.CleanupInterval,
		batchSize:     cfg.CleanupBatchSize,
		now:           time.Now,
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
	cutoff := s.now().UTC().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	return s.repo.DeleteOlderThan(cutoff, s.batchSize)
}
