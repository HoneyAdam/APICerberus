package audit

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	archiveEnabled     bool
	archiveDir         string
	archiveCompress    bool
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
	if strings.TrimSpace(cfg.ArchiveDir) == "" {
		cfg.ArchiveDir = "audit-archive"
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
		archiveEnabled:     cfg.ArchiveEnabled,
		archiveDir:         strings.TrimSpace(cfg.ArchiveDir),
		archiveCompress:    cfg.ArchiveCompress,
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
		for route := range s.routeRetentionDays {
			overrideRoutes = append(overrideRoutes, route)
		}
		sort.Strings(overrideRoutes)

		for _, route := range overrideRoutes {
			days := s.routeRetentionDays[route]
			routeCutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
			deleted, err := s.deleteWithOptionalArchive("route-"+route, routeCutoff, func(cutoff time.Time, batchSize int) (int64, error) {
				return s.repo.DeleteOlderThanForRoute(route, cutoff, batchSize)
			}, func(cutoff time.Time, limit int) ([]store.AuditEntry, error) {
				return s.repo.ListOlderThanForRoute(route, cutoff, limit)
			})
			if err != nil {
				return deletedTotal, err
			}
			deletedTotal += deleted
		}

		defaultCutoff := now.Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
		deleted, err := s.deleteWithOptionalArchive("default", defaultCutoff, func(cutoff time.Time, batchSize int) (int64, error) {
			return s.repo.DeleteOlderThanExcludingRoutes(cutoff, batchSize, overrideRoutes)
		}, func(cutoff time.Time, limit int) ([]store.AuditEntry, error) {
			return s.repo.ListOlderThanExcludingRoutes(cutoff, limit, overrideRoutes)
		})
		if err != nil {
			return deletedTotal, err
		}
		deletedTotal += deleted
		return deletedTotal, nil
	}

	cutoff := now.Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	return s.deleteWithOptionalArchive("default", cutoff, s.repo.DeleteOlderThan, s.repo.ListOlderThan)
}

func normalizeRouteKey(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}
	return strings.ToLower(route)
}

type deleteBatchFn func(cutoff time.Time, batchSize int) (int64, error)
type listBatchFn func(cutoff time.Time, limit int) ([]store.AuditEntry, error)

func (s *RetentionScheduler) deleteWithOptionalArchive(scope string, cutoff time.Time, deleter deleteBatchFn, lister listBatchFn) (int64, error) {
	if !s.archiveEnabled {
		return deleter(cutoff, s.batchSize)
	}
	return s.archiveAndDelete(scope, cutoff, lister)
}

func (s *RetentionScheduler) archiveAndDelete(scope string, cutoff time.Time, lister listBatchFn) (int64, error) {
	var deletedTotal int64
	for {
		entries, err := lister(cutoff, s.batchSize)
		if err != nil {
			return deletedTotal, err
		}
		if len(entries) == 0 {
			return deletedTotal, nil
		}

		if err := s.archiveEntries(scope, entries); err != nil {
			return deletedTotal, err
		}

		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			if strings.TrimSpace(entry.ID) != "" {
				ids = append(ids, entry.ID)
			}
		}
		if len(ids) == 0 {
			return deletedTotal, fmt.Errorf("archive cleanup scope %q produced entries without ids", scope)
		}

		deleted, err := s.repo.DeleteByIDs(ids)
		if err != nil {
			return deletedTotal, err
		}
		deletedTotal += deleted
	}
}

func (s *RetentionScheduler) archiveEntries(scope string, entries []store.AuditEntry) error {
	path, err := s.archiveFilePath(scope)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create audit archive directory: %w", err)
	}

	// #nosec G304 -- path is within the administrator-configured audit archive directory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open audit archive file: %w", err)
	}
	defer file.Close()

	if s.archiveCompress {
		gz := gzip.NewWriter(file)
		enc := json.NewEncoder(gz)
		for _, entry := range entries {
			if err := enc.Encode(entry); err != nil {
				_ = gz.Close()
				return fmt.Errorf("write gzip audit archive entry: %w", err)
			}
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("close gzip audit archive writer: %w", err)
		}
		return nil
	}

	enc := json.NewEncoder(file)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("write audit archive entry: %w", err)
		}
	}
	return nil
}

func (s *RetentionScheduler) archiveFilePath(scope string) (string, error) {
	archiveDir := strings.TrimSpace(s.archiveDir)
	if archiveDir == "" {
		return "", fmt.Errorf("audit archive directory is empty")
	}
	now := s.now().UTC()
	dayDir := filepath.Join(archiveDir, now.Format("2006"), now.Format("01"), now.Format("02"))
	scope = sanitizeArchiveScope(scope)
	fileName := fmt.Sprintf("audit-%s-%s.jsonl", scope, now.Format("20060102"))
	if s.archiveCompress {
		fileName += ".gz"
	}
	return filepath.Join(dayDir, fileName), nil
}

func sanitizeArchiveScope(scope string) string {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" {
		return "default"
	}
	var b strings.Builder
	for i := 0; i < len(scope); i++ {
		ch := scope[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteByte(ch)
			continue
		}
		b.WriteByte('-')
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "default"
	}
	return result
}
