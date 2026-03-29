package audit

import (
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestRetentionSchedulerRunOnceDeletesExpiredLogs(t *testing.T) {
	t.Parallel()

	st := openAuditTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{ID: "ret-old-1", CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "ret-old-2", CreatedAt: now.Add(-25 * time.Hour)},
		{ID: "ret-new-1", CreatedAt: now.Add(-2 * time.Hour)},
	}); err != nil {
		t.Fatalf("seed audit logs: %v", err)
	}

	scheduler := NewRetentionScheduler(st.Audits(), config.AuditConfig{
		Enabled:          true,
		RetentionDays:    1,
		CleanupInterval:  time.Minute,
		CleanupBatchSize: 1,
	})
	if scheduler == nil {
		t.Fatalf("expected retention scheduler")
	}
	scheduler.now = func() time.Time { return now }

	deleted, err := scheduler.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected deleted=2 got %d", deleted)
	}

	remaining, err := st.Audits().Search(store.AuditSearchFilters{Limit: 10})
	if err != nil {
		t.Fatalf("search remaining logs: %v", err)
	}
	if remaining.Total != 1 || remaining.Entries[0].ID != "ret-new-1" {
		t.Fatalf("unexpected remaining rows: %+v", remaining)
	}
}

func TestRetentionSchedulerRunOnceRouteOverrides(t *testing.T) {
	t.Parallel()

	st := openAuditTestStore(t)
	defer st.Close()

	now := time.Now().UTC()
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{ID: "route-default-old", RouteID: "default", CreatedAt: now.Add(-40 * 24 * time.Hour)},
		{ID: "route-default-new", RouteID: "default", CreatedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "route-critical-old", RouteID: "critical", CreatedAt: now.Add(-100 * 24 * time.Hour)},
		{ID: "route-critical-mid", RouteID: "critical", CreatedAt: now.Add(-40 * 24 * time.Hour)},
	}); err != nil {
		t.Fatalf("seed audit logs: %v", err)
	}

	scheduler := NewRetentionScheduler(st.Audits(), config.AuditConfig{
		Enabled:            true,
		RetentionDays:      30,
		RouteRetentionDays: map[string]int{"critical": 90},
		CleanupInterval:    time.Minute,
		CleanupBatchSize:   100,
	})
	if scheduler == nil {
		t.Fatalf("expected retention scheduler")
	}
	scheduler.now = func() time.Time { return now }

	deleted, err := scheduler.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected deleted=2 got %d", deleted)
	}

	remaining, err := st.Audits().Search(store.AuditSearchFilters{Limit: 20})
	if err != nil {
		t.Fatalf("search remaining logs: %v", err)
	}
	if remaining.Total != 2 {
		t.Fatalf("expected 2 remaining logs got %d", remaining.Total)
	}
	ids := map[string]bool{}
	for _, entry := range remaining.Entries {
		ids[entry.ID] = true
	}
	if !ids["route-default-new"] || !ids["route-critical-mid"] {
		t.Fatalf("unexpected remaining IDs: %+v", ids)
	}
}
