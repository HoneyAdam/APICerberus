package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/billing"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// ============================================================================
// Billing Deduct Benchmarks
// ============================================================================

func setupBillingEngine(b *testing.B) (*billing.Engine, *store.Store, func()) {
	b.Helper()

	tmpFile, err := os.CreateTemp("", "bench-billing-*.db")
	if err != nil {
		b.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        tmpFile.Name(),
			BusyTimeout: 5 * time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
	}

	s, err := store.Open(cfg)
	if err != nil {
		os.Remove(tmpFile.Name())
		b.Fatalf("open store: %v", err)
	}

	engine := billing.NewEngine(s, config.BillingConfig{
		Enabled:     true,
		DefaultCost: 1,
	})

	cleanup := func() {
		s.Close()
		os.Remove(tmpFile.Name())
		os.Remove(tmpFile.Name() + "-shm")
		os.Remove(tmpFile.Name() + "-wal")
	}

	return engine, s, cleanup
}

func createBenchUser(b *testing.B, s *store.Store, balance int, suffix string) *store.User {
	b.Helper()
	user := &store.User{
		Email:         fmt.Sprintf("bench-%s-%d@example.com", suffix, time.Now().UnixNano()),
		Name:          "Bench User",
		Role:          "user",
		Status:        "active",
		CreditBalance: int64(balance),
	}
	if err := s.Users().Create(user); err != nil {
		b.Fatalf("create user: %v", err)
	}
	return user
}

// makePreCheck runs PreCheck to produce a valid PreCheckResult for Deduct.
func makePreCheck(engine *billing.Engine, userID string) (*billing.PreCheckResult, error) {
	return engine.PreCheck(billing.RequestMeta{
		Consumer:  &config.Consumer{ID: userID},
		Route:     &config.Route{ID: "route-1"},
		Method:    "GET",
		RawAPIKey: "ck_live_test",
	})
}

// BenchmarkBillingDeduct benchmarks the full PreCheck+Deduct path:
// PreCheck (read) → BeginTx → UPDATE users RETURNING + INSERT credit_transactions → Commit
// Baseline target: >500 deducts/sec (2 SQL statements per tx)
func BenchmarkBillingDeduct(b *testing.B) {
	engine, s, cleanup := setupBillingEngine(b)
	defer cleanup()

	user := createBenchUser(b, s, b.N+10000, "deduct")
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pre, _ := makePreCheck(engine, user.ID)
		if pre != nil && pre.ShouldDeduct {
			_, _ = engine.Deduct(ctx, pre, fmt.Sprintf("req-%d", i), "route-1")
		}
	}
}

// BenchmarkBillingDeduct_Parallel benchmarks concurrent PreCheck+Deduct across multiple users.
// Measures SQLite write contention under parallel billing operations.
// Baseline target: >300 deducts/sec with 8 goroutines
func BenchmarkBillingDeduct_Parallel(b *testing.B) {
	engine, s, cleanup := setupBillingEngine(b)
	defer cleanup()

	userIDs := make([]string, 10)
	for i := range userIDs {
		user := createBenchUser(b, s, b.N*200+10000, fmt.Sprintf("par-%d", i))
		userIDs[i] = user.ID
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			userID := userIDs[i%len(userIDs)]
			pre, _ := makePreCheck(engine, userID)
			if pre != nil && pre.ShouldDeduct {
				_, _ = engine.Deduct(ctx, pre, fmt.Sprintf("req-%d", i), "route-1")
			}
			i++
		}
	})
}

// BenchmarkBillingPreCheck benchmarks the read-only balance check before deduction.
// Baseline target: >10,000 checks/sec
func BenchmarkBillingPreCheck(b *testing.B) {
	engine, s, cleanup := setupBillingEngine(b)
	defer cleanup()

	user := createBenchUser(b, s, 1000000, "precheck")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = makePreCheck(engine, user.ID)
	}
}

// ============================================================================
// Audit Batch Insert Benchmarks
// ============================================================================

// BenchmarkAuditBatchInsert benchmarks AuditRepo.BatchInsert for batch size 100.
// Measures the SQLite write path: Begin → Prepare → loop Exec → Commit.
func BenchmarkAuditBatchInsert(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Audits()

	entries := make([]store.AuditEntry, 100)
	for i := range entries {
		entries[i] = store.AuditEntry{
			ID:         fmt.Sprintf("audit-%d", i),
			RequestID:  fmt.Sprintf("req-%d", i),
			UserID:     "user-1",
			Method:     "GET",
			Path:       "/api/v1/test",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
			LatencyMS:  12,
			CreatedAt:  time.Now(),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = repo.BatchInsert(entries)
	}
}

// BenchmarkAuditBatchInsert_Sizes benchmarks batch inserts at different sizes.
func BenchmarkAuditBatchInsert_Sizes(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Audits()

	for _, size := range []int{1, 10, 50, 100, 500} {
		b.Run(fmt.Sprintf("batch_%d", size), func(b *testing.B) {
			entries := make([]store.AuditEntry, size)
			for i := range entries {
				entries[i] = store.AuditEntry{
					ID:         fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), i),
					RequestID:  fmt.Sprintf("req-%d", i),
					UserID:     "user-1",
					Method:     "GET",
					Path:       "/api/v1/test",
					StatusCode: 200,
					ClientIP:   "127.0.0.1",
					LatencyMS:  12,
					CreatedAt:  time.Now(),
				}
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for j := range entries {
					entries[j].ID = fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), j)
				}
				_ = repo.BatchInsert(entries)
			}
		})
	}
}

// ============================================================================
// Mixed Concurrent Write Benchmarks
// ============================================================================

// BenchmarkMixedConcurrentWrites benchmarks concurrent billing deductions + audit writes.
// Simulates the real-world scenario where both systems write to SQLite simultaneously.
// This is the primary contention benchmark for WAL mode.
func BenchmarkMixedConcurrentWrites(b *testing.B) {
	engine, s, cleanup := setupBillingEngine(b)
	defer cleanup()

	repo := s.Audits()

	userIDs := make([]string, 5)
	for i := range userIDs {
		user := createBenchUser(b, s, b.N*1000+10000, fmt.Sprintf("mixed-%d", i))
		userIDs[i] = user.ID
	}

	ctx := context.Background()
	var auditOps atomic.Int64
	var billingOps atomic.Int64

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				userID := userIDs[i%len(userIDs)]
				pre, _ := makePreCheck(engine, userID)
				if pre != nil && pre.ShouldDeduct {
					_, _ = engine.Deduct(ctx, pre, fmt.Sprintf("req-%d", i), "route-1")
				}
				billingOps.Add(1)
			} else {
				entry := store.AuditEntry{
					ID:         fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), i),
					RequestID:  fmt.Sprintf("req-%d", i),
					UserID:     userIDs[i%len(userIDs)],
					Method:     "GET",
					Path:       "/api/v1/test",
					StatusCode: 200,
					ClientIP:   "127.0.0.1",
					LatencyMS:  12,
					CreatedAt:  time.Now(),
				}
				_ = repo.BatchInsert([]store.AuditEntry{entry})
				auditOps.Add(1)
			}
			i++
		}
	})

	b.Logf("billing ops: %d, audit ops: %d", billingOps.Load(), auditOps.Load())
}

// BenchmarkMixedWrites_WithBatching benchmarks mixed writes with larger audit batches.
// Simulates more realistic scenario: billing per-request, audit in batches of 50.
func BenchmarkMixedWrites_WithBatching(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping mixed write benchmark in short mode")
	}

	engine, s, cleanup := setupBillingEngine(b)
	defer cleanup()

	repo := s.Audits()

	userIDs := make([]string, 5)
	for i := range userIDs {
		user := createBenchUser(b, s, b.N*1000+10000, fmt.Sprintf("batch-%d", i))
		userIDs[i] = user.ID
	}

	ctx := context.Background()

	auditBatch := make([]store.AuditEntry, 50)
	for i := range auditBatch {
		auditBatch[i] = store.AuditEntry{
			RequestID:  fmt.Sprintf("req-%d", i),
			UserID:     "user-1",
			Method:     "GET",
			Path:       "/api/v1/test",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
			LatencyMS:  12,
			CreatedAt:  time.Now(),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if i%50 == 0 && i > 0 {
			for j := range auditBatch {
				auditBatch[j].ID = fmt.Sprintf("audit-%d-%d", time.Now().UnixNano(), j)
			}
			_ = repo.BatchInsert(auditBatch)
		}

		userID := userIDs[i%len(userIDs)]
		pre, _ := makePreCheck(engine, userID)
		if pre != nil && pre.ShouldDeduct {
			_, _ = engine.Deduct(ctx, pre, fmt.Sprintf("req-%d", i), "route-1")
		}
	}
}

// ============================================================================
// WAL Contention Benchmarks
// ============================================================================

// BenchmarkSingleUserContention benchmarks write throughput when all goroutines
// update the same user's credits. Worst-case SQLite contention scenario.
func BenchmarkSingleUserContention(b *testing.B) {
	engine, s, cleanup := setupBillingEngine(b)
	defer cleanup()

	user := createBenchUser(b, s, b.N*2+100000, "contention")
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pre, _ := makePreCheck(engine, user.ID)
			if pre != nil && pre.ShouldDeduct {
				_, _ = engine.Deduct(ctx, pre, "req", "route-1")
			}
		}
	})
}
