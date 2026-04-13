package billing

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

type Engine struct {
	st      *store.Store
	users   *store.UserRepo
	credits *store.CreditRepo
	cfg     config.BillingConfig
}

type RequestMeta struct {
	Consumer     *config.Consumer
	Route        *config.Route
	Method       string
	RawAPIKey    string
	CostOverride *int64
}

type PreCheckResult struct {
	UserID       string
	Cost         int64
	ShouldDeduct bool
	ZeroBalance  bool
	Balance      int64
}

func NewEngine(st *store.Store, cfg config.BillingConfig) *Engine {
	if st == nil {
		return nil
	}
	return &Engine{
		st:      st,
		users:   st.Users(),
		credits: st.Credits(),
		cfg:     cfg,
	}
}

func (e *Engine) Enabled() bool {
	return e != nil && e.cfg.Enabled
}

func (e *Engine) CalculateCost(in RequestMeta) int64 {
	if e == nil || !e.cfg.Enabled {
		return 0
	}
	if in.CostOverride != nil {
		if *in.CostOverride < 0 {
			return 0
		}
		return *in.CostOverride
	}

	base := e.cfg.DefaultCost
	if base < 0 {
		base = 0
	}
	if routeID := routeID(in.Route); routeID != "" {
		if cost, ok := e.cfg.RouteCosts[routeID]; ok {
			base = cost
		}
	}
	if base <= 0 {
		return 0
	}

	multiplier := 1.0
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method != "" {
		if m, ok := e.cfg.MethodMultipliers[method]; ok && m > 0 {
			multiplier = m
		}
	}

	value := int64(math.Round(float64(base) * multiplier))
	if value < 0 {
		return 0
	}
	return value
}

func (e *Engine) PreCheck(in RequestMeta) (*PreCheckResult, error) {
	result := &PreCheckResult{}
	if e == nil || !e.cfg.Enabled {
		return result, nil
	}

	if in.Consumer == nil {
		return result, nil
	}
	userID := strings.TrimSpace(in.Consumer.ID)
	if userID == "" {
		return result, nil
	}
	result.UserID = userID

	if e.cfg.TestModeEnabled && isTestAPIKey(in.RawAPIKey) {
		return result, nil
	}

	cost := e.CalculateCost(in)
	result.Cost = cost
	if cost <= 0 {
		return result, nil
	}

	user, err := e.users.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, sql.ErrNoRows
	}
	result.Balance = user.CreditBalance
	if user.CreditBalance < cost {
		if strings.EqualFold(strings.TrimSpace(e.cfg.ZeroBalanceAction), "allow_with_flag") {
			result.ZeroBalance = true
			return result, nil
		}
		return nil, store.ErrInsufficientCredits
	}

	result.ShouldDeduct = true
	return result, nil
}

func (e *Engine) Deduct(result *PreCheckResult, requestID, routeID string) (int64, error) {
	if e == nil || !e.cfg.Enabled || result == nil || !result.ShouldDeduct {
		if result == nil {
			return 0, nil
		}
		return result.Balance, nil
	}
	if strings.TrimSpace(result.UserID) == "" || result.Cost <= 0 {
		return result.Balance, nil
	}

	// Use atomic transaction to ensure balance update and transaction log are consistent
	tx, err := e.st.DB().BeginTx(context.Background(), nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	var commitErr error
	defer func() {
		if commitErr == nil {
			return
		}
		_ = tx.Rollback()
	}()

	newBalance, err := e.users.UpdateCreditBalanceTx(tx, result.UserID, -result.Cost)
	if err != nil {
		return 0, err
	}

	if e.credits != nil {
		if err := e.credits.CreateTx(tx, &store.CreditTransaction{
			UserID:        result.UserID,
			Type:          "consume",
			Amount:        -result.Cost,
			BalanceBefore: newBalance + result.Cost,
			BalanceAfter:  newBalance,
			Description:   "request charge",
			RequestID:     strings.TrimSpace(requestID),
			RouteID:       strings.TrimSpace(routeID),
		}); err != nil {
			return newBalance, fmt.Errorf("create credit transaction: %w", err)
		}
	}

	commitErr = tx.Commit()
	if commitErr != nil {
		return 0, fmt.Errorf("commit transaction: %w", commitErr)
	}

	return newBalance, nil
}

func routeID(route *config.Route) string {
	if route == nil {
		return ""
	}
	if value := strings.TrimSpace(route.ID); value != "" {
		return value
	}
	return strings.TrimSpace(route.Name)
}

func isTestAPIKey(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), "ck_test_")
}
