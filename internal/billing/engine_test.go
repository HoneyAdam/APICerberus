package billing

import (
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestEngineCalculateCost(t *testing.T) {
	t.Parallel()

	e := &Engine{
		cfg: config.BillingConfig{
			Enabled:     true,
			DefaultCost: 1,
			RouteCosts: map[string]int64{
				"route-1": 5,
			},
			MethodMultipliers: map[string]float64{
				"POST": 2.0,
			},
		},
	}

	getCost := e.CalculateCost(RequestMeta{
		Route:  &config.Route{ID: "route-1"},
		Method: "GET",
	})
	if getCost != 5 {
		t.Fatalf("expected GET cost=5 got %d", getCost)
	}

	postCost := e.CalculateCost(RequestMeta{
		Route:  &config.Route{ID: "route-1"},
		Method: "POST",
	})
	if postCost != 10 {
		t.Fatalf("expected POST cost=10 got %d", postCost)
	}

	defaultCost := e.CalculateCost(RequestMeta{
		Route:  &config.Route{ID: "route-x"},
		Method: "GET",
	})
	if defaultCost != 1 {
		t.Fatalf("expected default route cost=1 got %d", defaultCost)
	}
}

func TestEnginePreCheckZeroBalanceAndTestKeyBypass(t *testing.T) {
	t.Parallel()

	st := openBillingStore(t)
	defer st.Close()

	user := createBillingUser(t, st, "low-balance@example.com", 3)
	consumer := &config.Consumer{ID: user.ID, Name: user.Name}

	rejectEngine := NewEngine(st, config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	})
	_, err := rejectEngine.PreCheck(RequestMeta{
		Consumer: consumer,
		Route:    &config.Route{ID: "route-1"},
		Method:   "GET",
	})
	if err != store.ErrInsufficientCredits {
		t.Fatalf("expected ErrInsufficientCredits got %v", err)
	}

	allowEngine := NewEngine(st, config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "allow_with_flag",
		TestModeEnabled:   true,
	})
	result, err := allowEngine.PreCheck(RequestMeta{
		Consumer: consumer,
		Route:    &config.Route{ID: "route-1"},
		Method:   "GET",
	})
	if err != nil {
		t.Fatalf("PreCheck allow_with_flag error: %v", err)
	}
	if !result.ZeroBalance || result.ShouldDeduct {
		t.Fatalf("expected zero-balance allow result, got %#v", result)
	}

	bypass, err := allowEngine.PreCheck(RequestMeta{
		Consumer:  consumer,
		Route:     &config.Route{ID: "route-1"},
		Method:    "GET",
		RawAPIKey: "ck_test_abc123",
	})
	if err != nil {
		t.Fatalf("PreCheck test key bypass error: %v", err)
	}
	if bypass.Cost != 0 || bypass.ShouldDeduct {
		t.Fatalf("expected bypass result with zero deduction, got %#v", bypass)
	}
}

func TestEngineDeductCreatesTransaction(t *testing.T) {
	t.Parallel()

	st := openBillingStore(t)
	defer st.Close()

	user := createBillingUser(t, st, "deduct@example.com", 20)
	engine := NewEngine(st, config.BillingConfig{
		Enabled:           true,
		DefaultCost:       4,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	})
	consumer := &config.Consumer{ID: user.ID, Name: user.Name}

	check, err := engine.PreCheck(RequestMeta{
		Consumer: consumer,
		Route:    &config.Route{ID: "route-deduct"},
		Method:   "GET",
	})
	if err != nil {
		t.Fatalf("PreCheck error: %v", err)
	}
	if !check.ShouldDeduct || check.Cost != 4 {
		t.Fatalf("unexpected precheck result: %#v", check)
	}

	newBalance, err := engine.Deduct(check, "req-1", "route-deduct")
	if err != nil {
		t.Fatalf("Deduct error: %v", err)
	}
	if newBalance != 16 {
		t.Fatalf("expected new balance 16 got %d", newBalance)
	}

	list, err := st.Credits().ListByUser(user.ID, store.CreditListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListByUser credit tx error: %v", err)
	}
	if list.Total != 1 || len(list.Transactions) != 1 {
		t.Fatalf("expected one credit transaction, got total=%d len=%d", list.Total, len(list.Transactions))
	}
	tx := list.Transactions[0]
	if tx.Amount != -4 || tx.BalanceAfter != 16 || tx.RequestID != "req-1" {
		t.Fatalf("unexpected credit transaction: %#v", tx)
	}
}

func openBillingStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(&config.Config{
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
	})
	if err != nil {
		t.Fatalf("open store error: %v", err)
	}
	return st
}

func createBillingUser(t *testing.T, st *store.Store, email string, balance int64) *store.User {
	t.Helper()
	pw, err := store.HashPassword("pw")
	if err != nil {
		t.Fatalf("hash password error: %v", err)
	}
	user := &store.User{
		Email:         email,
		Name:          "Billing User",
		PasswordHash:  pw,
		Role:          "user",
		Status:        "active",
		CreditBalance: balance,
	}
	if err := st.Users().Create(user); err != nil {
		t.Fatalf("create billing user error: %v", err)
	}
	return user
}

// Helper function for int64 pointer
func int64Ptr(i int64) *int64 {
	return &i
}

// Test Enabled function
func TestEngineEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		engine   *Engine
		expected bool
	}{
		{
			name:     "nil engine",
			engine:   nil,
			expected: false,
		},
		{
			name: "enabled config",
			engine: &Engine{
				cfg: config.BillingConfig{Enabled: true},
			},
			expected: true,
		},
		{
			name: "disabled config",
			engine: &Engine{
				cfg: config.BillingConfig{Enabled: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.engine.Enabled()
			if result != tt.expected {
				t.Errorf("Enabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test NewEngine with nil store
func TestNewEngine_NilStore(t *testing.T) {
	engine := NewEngine(nil, config.BillingConfig{Enabled: true})
	if engine != nil {
		t.Error("NewEngine(nil) should return nil")
	}
}

// Test CalculateCost edge cases
func TestEngineCalculateCost_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		engine   *Engine
		meta     RequestMeta
		expected int64
	}{
		{
			name:     "nil engine",
			engine:   nil,
			meta:     RequestMeta{Route: &config.Route{ID: "route-1"}, Method: "GET"},
			expected: 0,
		},
		{
			name:     "disabled billing",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: false, DefaultCost: 10}},
			meta:     RequestMeta{Route: &config.Route{ID: "route-1"}, Method: "GET"},
			expected: 0,
		},
		{
			name:     "negative cost override",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 10}},
			meta:     RequestMeta{CostOverride: int64Ptr(-5)},
			expected: 0,
		},
		{
			name:     "positive cost override",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 10}},
			meta:     RequestMeta{CostOverride: int64Ptr(25)},
			expected: 25,
		},
		{
			name:     "zero cost override",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 10}},
			meta:     RequestMeta{CostOverride: int64Ptr(0)},
			expected: 0,
		},
		{
			name:     "negative default cost",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: -10}},
			meta:     RequestMeta{Route: &config.Route{ID: "route-1"}, Method: "GET"},
			expected: 0,
		},
		{
			name:     "zero base cost after route check",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 0, RouteCosts: map[string]int64{"route-1": 0}}},
			meta:     RequestMeta{Route: &config.Route{ID: "route-1"}, Method: "GET"},
			expected: 0,
		},
		{
			name:     "nil route",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 5}},
			meta:     RequestMeta{Route: nil, Method: "GET"},
			expected: 5,
		},
		{
			name:     "route by name",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 1, RouteCosts: map[string]int64{"Test Route": 10}}},
			meta:     RequestMeta{Route: &config.Route{ID: "", Name: "Test Route"}, Method: "GET"},
			expected: 10,
		},
		{
			name:     "empty method",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 10, MethodMultipliers: map[string]float64{"POST": 2.0}}},
			meta:     RequestMeta{Route: &config.Route{ID: "route-1"}, Method: ""},
			expected: 10,
		},
		{
			name:     "method with zero multiplier",
			engine:   &Engine{cfg: config.BillingConfig{Enabled: true, DefaultCost: 10, MethodMultipliers: map[string]float64{"GET": 0}}},
			meta:     RequestMeta{Route: &config.Route{ID: "route-1"}, Method: "GET"},
			expected: 10, // multiplier is 0 but base * 1.0 = 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.engine.CalculateCost(tt.meta)
			if result != tt.expected {
				t.Errorf("CalculateCost() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// Test Deduct edge cases
func TestEngineDeduct_EdgeCases(t *testing.T) {
	t.Parallel()

	st := openBillingStore(t)
	defer st.Close()

	engine := NewEngine(st, config.BillingConfig{
		Enabled:     true,
		DefaultCost: 5,
	})

	tests := []struct {
		name        string
		engine      *Engine
		result      *PreCheckResult
		requestID   string
		routeID     string
		wantBalance int64
		wantErr     bool
	}{
		{
			name:        "nil engine",
			engine:      nil,
			result:      &PreCheckResult{UserID: "user-1", Cost: 5, ShouldDeduct: true, Balance: 100},
			wantBalance: 100,
			wantErr:     false,
		},
		{
			name:        "disabled billing",
			engine:      &Engine{cfg: config.BillingConfig{Enabled: false}},
			result:      &PreCheckResult{UserID: "user-1", Cost: 5, ShouldDeduct: true, Balance: 100},
			wantBalance: 100,
			wantErr:     false,
		},
		{
			name:        "nil result",
			engine:      engine,
			result:      nil,
			wantBalance: 0,
			wantErr:     false,
		},
		{
			name:        "should not deduct",
			engine:      engine,
			result:      &PreCheckResult{UserID: "user-1", Cost: 5, ShouldDeduct: false, Balance: 100},
			wantBalance: 100,
			wantErr:     false,
		},
		{
			name:        "empty userID",
			engine:      engine,
			result:      &PreCheckResult{UserID: "", Cost: 5, ShouldDeduct: true, Balance: 100},
			wantBalance: 100,
			wantErr:     false,
		},
		{
			name:        "zero cost",
			engine:      engine,
			result:      &PreCheckResult{UserID: "user-1", Cost: 0, ShouldDeduct: true, Balance: 100},
			wantBalance: 100,
			wantErr:     false,
		},
		{
			name:        "negative cost",
			engine:      engine,
			result:      &PreCheckResult{UserID: "user-1", Cost: -5, ShouldDeduct: true, Balance: 100},
			wantBalance: 100,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance, err := tt.engine.Deduct(tt.result, tt.requestID, tt.routeID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Deduct() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if balance != tt.wantBalance {
				t.Errorf("Deduct() balance = %d, want %d", balance, tt.wantBalance)
			}
		})
	}
}

// Test routeID function
func TestRouteID(t *testing.T) {
	tests := []struct {
		name  string
		route *config.Route
		want  string
	}{
		{
			name:  "nil route",
			route: nil,
			want:  "",
		},
		{
			name:  "route with ID",
			route: &config.Route{ID: "route-1", Name: "Test Route"},
			want:  "route-1",
		},
		{
			name:  "route with empty ID, has Name",
			route: &config.Route{ID: "", Name: "Test Route"},
			want:  "Test Route",
		},
		{
			name:  "route with whitespace ID",
			route: &config.Route{ID: "  ", Name: "Test Route"},
			want:  "Test Route",
		},
		{
			name:  "route with both empty",
			route: &config.Route{ID: "", Name: ""},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeID(tt.route)
			if got != tt.want {
				t.Errorf("routeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test PreCheck edge cases
func TestEnginePreCheck_EdgeCases(t *testing.T) {
	t.Parallel()

	st := openBillingStore(t)
	defer st.Close()

	user := createBillingUser(t, st, "precheck-edge@example.com", 100)
	engine := NewEngine(st, config.BillingConfig{
		Enabled:     true,
		DefaultCost: 5,
	})

	tests := []struct {
		name    string
		engine  *Engine
		meta    RequestMeta
		wantErr bool
		checkFn func(*testing.T, *PreCheckResult)
	}{
		{
			name:    "nil engine",
			engine:  nil,
			meta:    RequestMeta{Consumer: &config.Consumer{ID: user.ID}},
			wantErr: false,
			checkFn: func(t *testing.T, r *PreCheckResult) {
				if r.ShouldDeduct {
					t.Error("nil engine should not deduct")
				}
			},
		},
		{
			name:    "disabled billing",
			engine:  NewEngine(st, config.BillingConfig{Enabled: false}),
			meta:    RequestMeta{Consumer: &config.Consumer{ID: user.ID}},
			wantErr: false,
			checkFn: func(t *testing.T, r *PreCheckResult) {
				if r.ShouldDeduct {
					t.Error("disabled billing should not deduct")
				}
			},
		},
		{
			name:    "nil consumer",
			engine:  engine,
			meta:    RequestMeta{Consumer: nil},
			wantErr: false,
			checkFn: func(t *testing.T, r *PreCheckResult) {
				if r.ShouldDeduct {
					t.Error("nil consumer should not deduct")
				}
			},
		},
		{
			name:    "empty consumer ID",
			engine:  engine,
			meta:    RequestMeta{Consumer: &config.Consumer{ID: "  "}},
			wantErr: false,
			checkFn: func(t *testing.T, r *PreCheckResult) {
				if r.ShouldDeduct {
					t.Error("empty consumer ID should not deduct")
				}
			},
		},
		{
			name:    "test key bypass",
			engine:  NewEngine(st, config.BillingConfig{Enabled: true, TestModeEnabled: true}),
			meta:    RequestMeta{Consumer: &config.Consumer{ID: user.ID}, RawAPIKey: "ck_test_abc123"},
			wantErr: false,
			checkFn: func(t *testing.T, r *PreCheckResult) {
				if r.ShouldDeduct {
					t.Error("test key should bypass deduction")
				}
			},
		},
		{
			name:    "zero cost",
			engine:  NewEngine(st, config.BillingConfig{Enabled: true, DefaultCost: 0}),
			meta:    RequestMeta{Consumer: &config.Consumer{ID: user.ID}},
			wantErr: false,
			checkFn: func(t *testing.T, r *PreCheckResult) {
				if r.ShouldDeduct {
					t.Error("zero cost should not deduct")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.engine.PreCheck(tt.meta)
			if (err != nil) != tt.wantErr {
				t.Errorf("PreCheck() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

// Test Enabled with store integration
func TestEngineEnabled_Integration(t *testing.T) {
	t.Parallel()

	st := openBillingStore(t)
	defer st.Close()

	// Enabled engine
	enabledEngine := NewEngine(st, config.BillingConfig{Enabled: true})
	if !enabledEngine.Enabled() {
		t.Error("Expected enabled engine to return true")
	}

	// Disabled engine
	disabledEngine := NewEngine(st, config.BillingConfig{Enabled: false})
	if disabledEngine.Enabled() {
		t.Error("Expected disabled engine to return false")
	}

	// Nil engine
	var nilEngine *Engine
	if nilEngine.Enabled() {
		t.Error("Expected nil engine to return false")
	}
}
