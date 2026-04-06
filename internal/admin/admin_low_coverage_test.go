package admin

import (
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// Test for isRateLimited function (35.7% coverage)
func TestIsRateLimited_Coverage(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{
			APIKey: "test-key",
		},
	}
	gw, _ := gateway.New(cfg)
	server, _ := NewServer(cfg, gw)

	t.Run("IP not in map", func(t *testing.T) {
		result := server.isRateLimited("1.2.3.4")
		if result {
			t.Error("Non-existent IP should not be rate limited")
		}
	})

	t.Run("blocked IP within timeout", func(t *testing.T) {
		server.rlAttempts["2.3.4.5"] = &adminAuthAttempts{
			count:     5,
			blocked:   true,
			firstSeen: time.Now(),
			lastSeen:  time.Now(),
		}
		result := server.isRateLimited("2.3.4.5")
		if !result {
			t.Error("Blocked IP should be rate limited")
		}
		delete(server.rlAttempts, "2.3.4.5")
	})

	t.Run("blocked IP after timeout", func(t *testing.T) {
		server.rlAttempts["3.4.5.6"] = &adminAuthAttempts{
			count:     5,
			blocked:   true,
			firstSeen: time.Now().Add(-40 * time.Minute),
			lastSeen:  time.Now().Add(-40 * time.Minute),
		}
		result := server.isRateLimited("3.4.5.6")
		if result {
			t.Error("Expired block should not be rate limited")
		}
		delete(server.rlAttempts, "3.4.5.6")
	})

	t.Run("within window exceeding threshold", func(t *testing.T) {
		server.rlAttempts["4.5.6.7"] = &adminAuthAttempts{
			count:     5,
			blocked:   false,
			firstSeen: time.Now().Add(-5 * time.Minute),
			lastSeen:  time.Now(),
		}
		result := server.isRateLimited("4.5.6.7")
		if !result {
			t.Error("IP with 5+ attempts within window should be rate limited")
		}
		delete(server.rlAttempts, "4.5.6.7")
	})

	t.Run("outside window", func(t *testing.T) {
		server.rlAttempts["5.6.7.8"] = &adminAuthAttempts{
			count:     5,
			blocked:   false,
			firstSeen: time.Now().Add(-20 * time.Minute),
			lastSeen:  time.Now().Add(-20 * time.Minute),
		}
		result := server.isRateLimited("5.6.7.8")
		if result {
			t.Error("IP with attempts outside window should not be rate limited")
		}
		delete(server.rlAttempts, "5.6.7.8")
	})
}

// Test for recordFailedAuth function
func TestRecordFailedAuth_Coverage(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{
			APIKey: "test-key",
		},
	}
	gw, _ := gateway.New(cfg)
	server, _ := NewServer(cfg, gw)

	t.Run("new entry", func(t *testing.T) {
		server.recordFailedAuth("1.2.3.4")
		attempts, exists := server.rlAttempts["1.2.3.4"]
		if !exists {
			t.Fatal("Failed auth should create an entry")
		}
		if attempts.count != 1 {
			t.Errorf("count = %d, want 1", attempts.count)
		}
		delete(server.rlAttempts, "1.2.3.4")
	})

	t.Run("increment existing", func(t *testing.T) {
		server.rlAttempts["2.3.4.5"] = &adminAuthAttempts{
			count:     2,
			blocked:   false,
			firstSeen: time.Now(),
			lastSeen:  time.Now(),
		}
		server.recordFailedAuth("2.3.4.5")
		attempts := server.rlAttempts["2.3.4.5"]
		if attempts.count != 3 {
			t.Errorf("count = %d, want 3", attempts.count)
		}
		delete(server.rlAttempts, "2.3.4.5")
	})

	t.Run("block at threshold", func(t *testing.T) {
		server.rlAttempts["3.4.5.6"] = &adminAuthAttempts{
			count:     4,
			blocked:   false,
			firstSeen: time.Now(),
			lastSeen:  time.Now(),
		}
		server.recordFailedAuth("3.4.5.6")
		attempts := server.rlAttempts["3.4.5.6"]
		if !attempts.blocked {
			t.Error("5th attempt should block the IP")
		}
		delete(server.rlAttempts, "3.4.5.6")
	})

	t.Run("reset expired entry", func(t *testing.T) {
		server.rlAttempts["4.5.6.7"] = &adminAuthAttempts{
			count:     10,
			blocked:   false,
			firstSeen: time.Now().Add(-20 * time.Minute),
			lastSeen:  time.Now().Add(-20 * time.Minute),
		}
		server.recordFailedAuth("4.5.6.7")
		attempts := server.rlAttempts["4.5.6.7"]
		if attempts.count != 1 {
			t.Errorf("count = %d, want 1 (should reset)", attempts.count)
		}
		delete(server.rlAttempts, "4.5.6.7")
	})
}

// Test for clearFailedAuth function
func TestClearFailedAuth_Coverage(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{
			APIKey: "test-key",
		},
	}
	gw, _ := gateway.New(cfg)
	server, _ := NewServer(cfg, gw)

	server.rlAttempts["1.2.3.4"] = &adminAuthAttempts{
		count:     5,
		blocked:   true,
		firstSeen: time.Now(),
		lastSeen:  time.Now(),
	}
	server.clearFailedAuth("1.2.3.4")
	_, exists := server.rlAttempts["1.2.3.4"]
	if exists {
		t.Error("clearFailedAuth should delete the entry")
	}
}

// Test for cleanupOldRateLimitEntries function (0.0% coverage)
func TestCleanupOldRateLimitEntries_Coverage(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{
			APIKey: "test-key",
		},
	}
	gw, _ := gateway.New(cfg)
	server, _ := NewServer(cfg, gw)

	// Create old entry (older than 30 minutes)
	server.rlAttempts["1.2.3.4"] = &adminAuthAttempts{
		count:     5,
		blocked:   true,
		firstSeen: time.Now().Add(-40 * time.Minute),
		lastSeen:  time.Now().Add(-40 * time.Minute),
	}

	// Create recent entry
	server.rlAttempts["2.3.4.5"] = &adminAuthAttempts{
		count:     1,
		blocked:   false,
		firstSeen: time.Now(),
		lastSeen:  time.Now(),
	}

	server.cleanupOldRateLimitEntries()

	_, oldExists := server.rlAttempts["1.2.3.4"]
	_, recentExists := server.rlAttempts["2.3.4.5"]

	if oldExists {
		t.Error("Old entry should be cleaned up")
	}
	if !recentExists {
		t.Error("Recent entry should not be cleaned up")
	}

	// Cleanup
	delete(server.rlAttempts, "2.3.4.5")
}

// Test for startRateLimitCleanup function
func TestStartRateLimitCleanup_Coverage(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{
			APIKey: "test-key",
		},
	}
	gw, _ := gateway.New(cfg)
	server, _ := NewServer(cfg, gw)

	// Start cleanup - should not panic
	server.startRateLimitCleanup()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)
}

// Test for decodePermissionPayload function (29.6% coverage)
func TestDecodePermissionPayload_Coverage(t *testing.T) {
	t.Run("nil payload", func(t *testing.T) {
		_, err := decodePermissionPayload(nil)
		if err == nil {
			t.Error("decodePermissionPayload(nil) should return error")
		}
	})

	t.Run("missing route_id", func(t *testing.T) {
		payload := map[string]any{
			"methods": []string{"GET"},
		}
		_, err := decodePermissionPayload(payload)
		if err == nil {
			t.Error("decodePermissionPayload without route_id should return error")
		}
	})

	t.Run("invalid credit_cost", func(t *testing.T) {
		payload := map[string]any{
			"route_id":    "test-route",
			"credit_cost": "not-a-number",
		}
		_, err := decodePermissionPayload(payload)
		if err == nil {
			t.Error("decodePermissionPayload with invalid credit_cost should return error")
		}
	})

	t.Run("invalid valid_from", func(t *testing.T) {
		payload := map[string]any{
			"route_id":   "test-route",
			"valid_from": "not-a-valid-time",
		}
		_, err := decodePermissionPayload(payload)
		if err == nil {
			t.Error("decodePermissionPayload with invalid valid_from should return error")
		}
	})

	t.Run("invalid valid_until", func(t *testing.T) {
		payload := map[string]any{
			"route_id":    "test-route",
			"valid_until": "not-a-valid-time",
		}
		_, err := decodePermissionPayload(payload)
		if err == nil {
			t.Error("decodePermissionPayload with invalid valid_until should return error")
		}
	})

	t.Run("valid payload", func(t *testing.T) {
		now := time.Now()
		payload := map[string]any{
			"id":            "perm-1",
			"route_id":      "test-route",
			"methods":       []string{"GET", "POST"},
			"allowed":       true,
			"credit_cost":   "100",
			"valid_from":    now.Format(time.RFC3339),
			"valid_until":   now.Add(24 * time.Hour).Format(time.RFC3339),
			"rate_limits":   map[string]any{"rps": 10},
			"allowed_days":  []int{1, 2, 3},
			"allowed_hours": []string{"09:00-17:00"},
		}
		perm, err := decodePermissionPayload(payload)
		if err != nil {
			t.Errorf("decodePermissionPayload valid payload error: %v", err)
		}
		if perm.RouteID != "test-route" {
			t.Errorf("RouteID = %q, want %q", perm.RouteID, "test-route")
		}
	})
}

// Test for clonePluginConfigs function (15.4% coverage)
func TestClonePluginConfigs_Coverage(t *testing.T) {
	t.Run("with configs", func(t *testing.T) {
		enabled := true
		configs := []config.PluginConfig{
			{
				Enabled: &enabled,
				Config:  map[string]any{"key": "value"},
			},
		}
		cloned := clonePluginConfigs(configs)
		if len(cloned) != len(configs) {
			t.Errorf("len(cloned) = %d, want %d", len(cloned), len(configs))
		}
	})

	t.Run("nil input", func(t *testing.T) {
		cloned := clonePluginConfigs(nil)
		if cloned != nil {
			t.Error("clonePluginConfigs(nil) should return nil")
		}
	})
}
