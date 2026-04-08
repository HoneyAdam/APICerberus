// Package main provides comprehensive benchmarks for APICerebrus Store components.
package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// ============================================================================
// Store Setup Helper
// ============================================================================

func setupBenchmarkStore(b *testing.B) (*store.Store, func()) {
	b.Helper()

	// Create temporary database file
	tmpFile, err := os.CreateTemp("", "bench-*.db")
	if err != nil {
		b.Fatalf("failed to create temp db: %v", err)
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
		b.Fatalf("failed to open store: %v", err)
	}

	cleanup := func() {
		s.Close()
		os.Remove(tmpFile.Name())
		os.Remove(tmpFile.Name() + "-shm")
		os.Remove(tmpFile.Name() + "-wal")
	}

	return s, cleanup
}

// ============================================================================
// User Repository Benchmarks
// ============================================================================

// BenchmarkUserCreate benchmarks user creation.
// Baseline: ~2,000 inserts/sec
func BenchmarkUserCreate(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		user := &store.User{
			Email:    fmt.Sprintf("user%d@example.com", i),
			Name:     fmt.Sprintf("User %d", i),
			Company:  "Test Corp",
			Role:     "user",
			Status:   "active",
			Metadata: map[string]any{"plan": "premium"},
		}
		_ = repo.Create(user)
	}
}

// BenchmarkUserFindByID benchmarks user lookup by ID.
// Baseline: ~50,000 lookups/sec
func BenchmarkUserFindByID(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test user
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := repo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.FindByID(user.ID)
	}
}

// BenchmarkUserFindByEmail benchmarks user lookup by email.
// Baseline: ~40,000 lookups/sec (slower than ID due to index)
func BenchmarkUserFindByEmail(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test user
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := repo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.FindByEmail("test@example.com")
	}
}

// BenchmarkUserList benchmarks user listing with pagination.
// Baseline: ~5,000 lists/sec for 50 users
func BenchmarkUserList(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test users
	for i := 0; i < 100; i++ {
		user := &store.User{
			Email:  fmt.Sprintf("user%d@example.com", i),
			Name:   fmt.Sprintf("User %d", i),
			Role:   "user",
			Status: "active",
		}
		if err := repo.Create(user); err != nil {
			b.Fatalf("failed to create user: %v", err)
		}
	}

	opts := store.UserListOptions{
		Limit: 50,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.List(opts)
	}
}

// BenchmarkUserUpdate benchmarks user updates.
// Baseline: ~1,500 updates/sec
func BenchmarkUserUpdate(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test user
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := repo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		user.Name = fmt.Sprintf("Updated User %d", i)
		_ = repo.Update(user)
	}
}

// BenchmarkUserUpdateCreditBalance benchmarks atomic credit balance updates.
// Baseline: ~3,000 updates/sec (uses RETURNING clause)
func BenchmarkUserUpdateCreditBalance(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test user with initial balance
	user := &store.User{
		Email:         "test@example.com",
		Name:          "Test User",
		Role:          "user",
		Status:        "active",
		CreditBalance: 10000,
	}
	if err := repo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.UpdateCreditBalance(user.ID, 1)
	}
}

// ============================================================================
// API Key Repository Benchmarks
// ============================================================================

// BenchmarkAPIKeyCreate benchmarks API key creation.
// Baseline: ~1,500 keys/sec (includes hashing)
func BenchmarkAPIKeyCreate(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	// Create a user first
	userRepo := s.Users()
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := userRepo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	repo := s.APIKeys()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = repo.Create(user.ID, fmt.Sprintf("key-%d", i), "live")
	}
}

// BenchmarkAPIKeyFindByHash benchmarks API key lookup by hash.
// Baseline: ~60,000 lookups/sec
func BenchmarkAPIKeyFindByHash(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	// Create a user first
	userRepo := s.Users()
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := userRepo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	repo := s.APIKeys()
	_, key, err := repo.Create(user.ID, "test-key", "live")
	if err != nil {
		b.Fatalf("failed to create key: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.FindByHash(key.KeyHash)
	}
}

// BenchmarkAPIKeyListByUser benchmarks listing keys for a user.
// Baseline: ~10,000 lists/sec
func BenchmarkAPIKeyListByUser(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	// Create a user with multiple keys
	userRepo := s.Users()
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := userRepo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	repo := s.APIKeys()
	for i := 0; i < 10; i++ {
		_, _, _ = repo.Create(user.ID, fmt.Sprintf("key-%d", i), "live")
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.ListByUser(user.ID)
	}
}

// BenchmarkAPIKeyResolveUser benchmarks full key resolution with user lookup.
// Baseline: ~30,000 resolutions/sec (JOIN operation)
func BenchmarkAPIKeyResolveUser(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	// Create a user with a key
	userRepo := s.Users()
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := userRepo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	repo := s.APIKeys()
	fullKey, _, err := repo.Create(user.ID, "test-key", "live")
	if err != nil {
		b.Fatalf("failed to create key: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = repo.ResolveUserByRawKey(fullKey)
	}
}

// ============================================================================
// Batch Operations Benchmarks
// ============================================================================

// BenchmarkBatchInsertUsers benchmarks batch user insertion.
// Baseline: ~500 users/sec in individual transactions
func BenchmarkBatchInsertUsers(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		user := &store.User{
			Email:  fmt.Sprintf("user%d@example.com", i),
			Name:   fmt.Sprintf("User %d", i),
			Role:   "user",
			Status: "active",
		}
		_ = repo.Create(user)
	}
}

// BenchmarkTransactionThroughput benchmarks transaction processing speed.
// Baseline: ~2,000 transactions/sec
func BenchmarkTransactionThroughput(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	userRepo := s.Users()
	user := &store.User{
		Email:         "test@example.com",
		Name:          "Test User",
		Role:          "user",
		Status:        "active",
		CreditBalance: 100000,
	}
	if err := userRepo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Each operation is a transaction
		_, _ = userRepo.UpdateCreditBalance(user.ID, -1)
	}
}

// ============================================================================
// Concurrent Access Benchmarks
// ============================================================================

// BenchmarkConcurrentUserReads benchmarks concurrent read performance.
// Baseline: ~100,000 reads/sec with 8 threads
func BenchmarkConcurrentUserReads(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test user
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := repo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = repo.FindByID(user.ID)
		}
	})
}

// BenchmarkConcurrentCreditUpdates benchmarks concurrent credit updates.
// Baseline: ~5,000 updates/sec with 8 threads (contention)
func BenchmarkConcurrentCreditUpdates(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	userRepo := s.Users()

	// Create multiple users to reduce contention
	userIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		user := &store.User{
			Email:         fmt.Sprintf("user%d@example.com", i),
			Name:          fmt.Sprintf("User %d", i),
			Role:          "user",
			Status:        "active",
			CreditBalance: 1000000,
		}
		if err := userRepo.Create(user); err != nil {
			b.Fatalf("failed to create user: %v", err)
		}
		userIDs[i] = user.ID
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			userID := userIDs[i%len(userIDs)]
			_, _ = userRepo.UpdateCreditBalance(userID, -1)
			i++
		}
	})
}

// ============================================================================
// Search Benchmarks
// ============================================================================

// BenchmarkUserSearch benchmarks user search functionality.
// Baseline: ~1,000 searches/sec across 1,000 users
func BenchmarkUserSearch(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test users
	for i := 0; i < 1000; i++ {
		user := &store.User{
			Email:   fmt.Sprintf("user%d@example.com", i),
			Name:    fmt.Sprintf("Test User %d", i),
			Company: fmt.Sprintf("Company %d", i%10),
			Role:    "user",
			Status:  "active",
		}
		if err := repo.Create(user); err != nil {
			b.Fatalf("failed to create user: %v", err)
		}
	}

	opts := store.UserListOptions{
		Search: "Test User",
		Limit:  50,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.List(opts)
	}
}

// BenchmarkUserFilterByStatus benchmarks filtering users by status.
// Baseline: ~2,000 filters/sec
func BenchmarkUserFilterByStatus(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create test users with mixed status
	for i := 0; i < 1000; i++ {
		status := "active"
		if i%2 == 0 {
			status = "suspended"
		}
		user := &store.User{
			Email:  fmt.Sprintf("user%d@example.com", i),
			Name:   fmt.Sprintf("User %d", i),
			Role:   "user",
			Status: status,
		}
		if err := repo.Create(user); err != nil {
			b.Fatalf("failed to create user: %v", err)
		}
	}

	opts := store.UserListOptions{
		Status: "active",
		Limit:  100,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.List(opts)
	}
}

// ============================================================================
// Raw SQL Benchmarks
// ============================================================================

// BenchmarkRawQuery benchmarks raw SQL query execution.
// Baseline: ~100,000 queries/sec for simple SELECT
func BenchmarkRawQuery(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	db := s.DB()

	// Create test data
	userRepo := s.Users()
	user := &store.User{
		Email:  "test@example.com",
		Name:   "Test User",
		Role:   "user",
		Status: "active",
	}
	if err := userRepo.Create(user); err != nil {
		b.Fatalf("failed to create user: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM users WHERE status = ?", "active").Scan(&count)
	}
}

// BenchmarkRawInsert benchmarks raw SQL insert performance.
// Baseline: ~3,000 inserts/sec
func BenchmarkRawInsert(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	db := s.DB()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = db.Exec(
			"INSERT INTO users(id, email, name, company, password_hash, role, status, credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			fmt.Sprintf("id-%d", i),
			fmt.Sprintf("user%d@example.com", i),
			fmt.Sprintf("User %d", i),
			"Test Corp",
			"hash",
			"user",
			"active",
			0,
			"{}",
			"[]",
			"{}",
			time.Now().Format(time.RFC3339Nano),
			time.Now().Format(time.RFC3339Nano),
		)
	}
}

// ============================================================================
// Memory Usage Benchmarks
// ============================================================================

// BenchmarkLargeDatasetQuery benchmarks query performance with large datasets.
// Baseline: ~500 ms for 10,000 users
func BenchmarkLargeDatasetQuery(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping large dataset benchmark in short mode")
	}

	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()

	repo := s.Users()

	// Create large dataset
	b.Log("Creating 10,000 test users...")
	for i := 0; i < 10000; i++ {
		user := &store.User{
			Email:    fmt.Sprintf("user%d@example.com", i),
			Name:     fmt.Sprintf("User %d", i),
			Company:  fmt.Sprintf("Company %d", i%100),
			Role:     "user",
			Status:   "active",
			Metadata: map[string]any{"index": i},
		}
		if err := repo.Create(user); err != nil {
			b.Fatalf("failed to create user: %v", err)
		}
	}

	opts := store.UserListOptions{
		Limit: 100,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = repo.List(opts)
	}
}
