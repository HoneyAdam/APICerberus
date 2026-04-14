package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// maxCreditOperation is the maximum credits that can be added/deducted in a single operation.
// This prevents integer overflow and abuse.
const maxCreditOperation = 1_000_000_000_000 // 1 trillion

func (s *Server) creditOverview(w http.ResponseWriter, _ *http.Request) {
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	stats, err := st.Credits().OverviewStats()
	if err != nil {
		writeError(w, http.StatusBadRequest, "credit_overview_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, stats)
}

func (s *Server) topupCredits(w http.ResponseWriter, r *http.Request) {
	s.adjustCredits(w, r, true)
}

func (s *Server) deductCredits(w http.ResponseWriter, r *http.Request) {
	s.adjustCredits(w, r, false)
}

func (s *Server) adjustCredits(w http.ResponseWriter, r *http.Request, topup bool) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	amount := int64(coerce.AsInt(payload["amount"], 0))
	if amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "amount must be greater than zero")
		return
	}
	if amount > maxCreditOperation {
		writeError(w, http.StatusBadRequest, "invalid_amount", fmt.Sprintf("amount exceeds maximum allowed operation of %d", maxCreditOperation))
		return
	}
	delta := amount
	txnType := "topup"
	if !topup {
		delta = -amount
		txnType = "admin_adjust"
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	// Use atomic transaction to ensure balance update and transaction log are consistent
	tx, err := st.DB().BeginTx(context.Background(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "adjust_credits_failed", "failed to begin transaction")
		return
	}
	var commitErr error
	defer func() {
		if commitErr == nil {
			return
		}
		_ = tx.Rollback()
	}()

	newBalance, err := st.Users().UpdateCreditBalanceTx(tx, userID, delta)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		case errors.Is(err, store.ErrInsufficientCredits):
			writeError(w, http.StatusPaymentRequired, "insufficient_credits", "Insufficient credits")
		default:
			writeError(w, http.StatusBadRequest, "adjust_credits_failed", "balance update failed")
		}
		return
	}

	before := newBalance - delta
	if err := st.Credits().CreateTx(tx, &store.CreditTransaction{
		UserID:        userID,
		Type:          txnType,
		Amount:        delta,
		BalanceBefore: before,
		BalanceAfter:  newBalance,
		Description:   strings.TrimSpace(coerce.AsString(payload["reason"])),
		RequestID:     strings.TrimSpace(coerce.AsString(payload["request_id"])),
		RouteID:       strings.TrimSpace(coerce.AsString(payload["route_id"])),
	}); err != nil {
		writeError(w, http.StatusBadRequest, "record_credit_transaction_failed", "failed to record transaction")
		return
	}

	commitErr = tx.Commit()
	if commitErr != nil {
		writeError(w, http.StatusInternalServerError, "adjust_credits_failed", "failed to commit transaction")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":     userID,
		"delta":       delta,
		"new_balance": newBalance,
	})
}

// adjustCreditsUnified handles POST /users/{id}/credits — determines topup vs deduct
// from the sign of the amount or an explicit "action" field.
func (s *Server) adjustCreditsUnified(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	amount := int64(coerce.AsInt(payload["amount"], 0))
	if amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "amount must be greater than zero")
		return
	}
	if amount > maxCreditOperation {
		writeError(w, http.StatusBadRequest, "invalid_amount", fmt.Sprintf("amount exceeds maximum allowed operation of %d", maxCreditOperation))
		return
	}

	reason := strings.TrimSpace(coerce.AsString(payload["reason"]))
	if reason == "" {
		writeError(w, http.StatusBadRequest, "invalid_reason", "reason is required")
		return
	}

	delta := amount
	txnType := "topup"
	if action := strings.TrimSpace(coerce.AsString(payload["action"])); action != "" {
		switch strings.ToLower(action) {
		case "deduct":
			delta = -amount
			txnType = "deduct"
		}
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	// Use atomic transaction to ensure balance update and transaction log are consistent
	tx, err := st.DB().BeginTx(context.Background(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "adjust_credits_failed", "failed to begin transaction")
		return
	}
	var commitErr error
	defer func() {
		if commitErr == nil {
			return
		}
		_ = tx.Rollback()
	}()

	newBalance, err := st.Users().UpdateCreditBalanceTx(tx, userID, delta)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		case errors.Is(err, store.ErrInsufficientCredits):
			writeError(w, http.StatusPaymentRequired, "insufficient_credits", "Insufficient credits")
		default:
			writeError(w, http.StatusBadRequest, "adjust_credits_failed", "balance update failed")
		}
		return
	}

	before := newBalance - delta
	if err := st.Credits().CreateTx(tx, &store.CreditTransaction{
		UserID:        userID,
		Type:          txnType,
		Amount:        delta,
		BalanceBefore: before,
		BalanceAfter:  newBalance,
		Description:   reason,
		RequestID:     strings.TrimSpace(coerce.AsString(payload["request_id"])),
		RouteID:       strings.TrimSpace(coerce.AsString(payload["route_id"])),
	}); err != nil {
		writeError(w, http.StatusBadRequest, "record_credit_transaction_failed", "failed to record transaction")
		return
	}

	commitErr = tx.Commit()
	if commitErr != nil {
		writeError(w, http.StatusInternalServerError, "adjust_credits_failed", "failed to commit transaction")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":     userID,
		"delta":       delta,
		"new_balance": newBalance,
	})
}

func (s *Server) listCreditTransactions(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	query := r.URL.Query()
	limit, _ := strconv.Atoi(strings.TrimSpace(query.Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(query.Get("offset")))

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	result, err := st.Credits().ListByUser(userID, store.CreditListOptions{
		Type:   strings.TrimSpace(query.Get("type")),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_credit_transactions_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) userCreditBalance(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credit_balance_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id": user.ID,
		"balance": user.CreditBalance,
	})
}

// userCreditOverview returns per-user credit summary with transactions.
func (s *Server) userCreditOverview(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", "internal server error")
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credit_overview_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	txs, err := st.Credits().ListByUser(userID, store.CreditListOptions{
		Limit: 50,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "credit_overview_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":      user.ID,
		"balance":      user.CreditBalance,
		"transactions": txs,
	})
}

func (s *Server) getBillingConfig(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	billing := config.CloneBillingConfig(s.cfg.Billing)
	s.mu.RUnlock()
	_ = jsonutil.WriteJSON(w, http.StatusOK, billing)
}

func (s *Server) updateBillingConfig(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	var updated config.BillingConfig
	if err := s.mutateConfig(func(cfg *config.Config) error {
		next := config.CloneBillingConfig(cfg.Billing)

		if value, ok := payload["enabled"]; ok {
			next.Enabled = coerce.AsBool(value, next.Enabled)
		}
		if value, ok := payload["default_cost"]; ok {
			next.DefaultCost = coerce.AsInt64(value, next.DefaultCost)
		}
		if value, ok := payload["zero_balance_action"]; ok {
			next.ZeroBalanceAction = strings.ToLower(strings.TrimSpace(coerce.AsString(value)))
		}
		if value, ok := payload["test_mode_enabled"]; ok {
			next.TestModeEnabled = coerce.AsBool(value, next.TestModeEnabled)
		}
		if value, ok := payload["route_costs"]; ok {
			routeCosts, err := parseBillingRouteCosts(value)
			if err != nil {
				return err
			}
			next.RouteCosts = routeCosts
		}
		if value, ok := payload["method_multipliers"]; ok {
			multipliers, err := parseBillingMethodMultipliers(value)
			if err != nil {
				return err
			}
			next.MethodMultipliers = multipliers
		}
		if err := validateBillingConfig(next); err != nil {
			return err
		}
		cfg.Billing = next
		updated = config.CloneBillingConfig(next)
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "update_billing_config_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, updated)
}

func (s *Server) getBillingRouteCosts(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	routeCosts := config.CloneInt64Map(s.cfg.Billing.RouteCosts)
	s.mu.RUnlock()
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"route_costs": routeCosts,
	})
}

func (s *Server) updateBillingRouteCosts(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	var updated map[string]int64
	if err := s.mutateConfig(func(cfg *config.Config) error {
		next := config.CloneBillingConfig(cfg.Billing)
		if value, ok := payload["route_costs"]; ok {
			routeCosts, err := parseBillingRouteCosts(value)
			if err != nil {
				return err
			}
			next.RouteCosts = routeCosts
		} else {
			routeID := strings.TrimSpace(coerce.AsString(payload["route_id"]))
			if routeID == "" {
				return errors.New("route_id is required when route_costs is omitted")
			}
			cost := coerce.AsInt64(payload["cost"], -1)
			if cost < 0 {
				return errors.New("cost must be greater than or equal to zero")
			}
			if next.RouteCosts == nil {
				next.RouteCosts = map[string]int64{}
			}
			next.RouteCosts[routeID] = cost
		}
		if err := validateBillingConfig(next); err != nil {
			return err
		}
		cfg.Billing = next
		updated = config.CloneInt64Map(next.RouteCosts)
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "update_route_costs_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"route_costs": updated,
	})
}

func validateBillingConfig(cfg config.BillingConfig) error {
	if cfg.DefaultCost < 0 {
		return errors.New("default_cost cannot be negative")
	}
	for routeID, cost := range cfg.RouteCosts {
		if strings.TrimSpace(routeID) == "" {
			return errors.New("route_costs keys cannot be empty")
		}
		if cost < 0 {
			return fmt.Errorf("route_costs[%q] cannot be negative", routeID)
		}
	}
	for method, multiplier := range cfg.MethodMultipliers {
		if strings.TrimSpace(method) == "" {
			return errors.New("method_multipliers keys cannot be empty")
		}
		if multiplier <= 0 {
			return fmt.Errorf("method_multipliers[%q] must be greater than zero", method)
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ZeroBalanceAction)) {
	case "reject", "allow_with_flag":
	default:
		return errors.New("zero_balance_action must be one of: reject, allow_with_flag")
	}
	return nil
}

func parseBillingRouteCosts(value any) (map[string]int64, error) {
	switch v := value.(type) {
	case map[string]int64:
		return config.CloneInt64Map(v), nil
	case map[string]any:
		out := make(map[string]int64, len(v))
		for rawKey, rawCost := range v {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				return nil, errors.New("route_costs keys cannot be empty")
			}
			cost := coerce.AsInt64(rawCost, -1)
			if cost < 0 {
				return nil, fmt.Errorf("route_costs[%q] cannot be negative", key)
			}
			out[key] = cost
		}
		return out, nil
	default:
		return nil, errors.New("route_costs must be an object")
	}
}

func parseBillingMethodMultipliers(value any) (map[string]float64, error) {
	switch v := value.(type) {
	case map[string]float64:
		return config.CloneFloat64Map(v), nil
	case map[string]any:
		out := make(map[string]float64, len(v))
		for rawKey, rawValue := range v {
			key := strings.ToUpper(strings.TrimSpace(rawKey))
			if key == "" {
				return nil, errors.New("method_multipliers keys cannot be empty")
			}
			multiplier, ok := coerce.AsFloat64(rawValue, 0)
			if !ok {
				return nil, fmt.Errorf("method_multipliers[%q] must be numeric", key)
			}
			if multiplier <= 0 {
				return nil, fmt.Errorf("method_multipliers[%q] must be greater than zero", key)
			}
			out[key] = multiplier
		}
		return out, nil
	default:
		return nil, errors.New("method_multipliers must be an object")
	}
}
