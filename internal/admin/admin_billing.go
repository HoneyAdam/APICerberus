package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func (s *Server) creditOverview(w http.ResponseWriter, _ *http.Request) {
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
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
	amount := int64(asInt(payload["amount"], 0))
	if amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "amount must be greater than zero")
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
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	newBalance, err := st.Users().UpdateCreditBalance(userID, delta)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		case errors.Is(err, store.ErrInsufficientCredits):
			writeError(w, http.StatusPaymentRequired, "insufficient_credits", "Insufficient credits")
		default:
			writeError(w, http.StatusBadRequest, "adjust_credits_failed", err.Error())
		}
		return
	}

	before := newBalance - delta
	if err := st.Credits().Create(&store.CreditTransaction{
		UserID:        userID,
		Type:          txnType,
		Amount:        delta,
		BalanceBefore: before,
		BalanceAfter:  newBalance,
		Description:   strings.TrimSpace(asString(payload["reason"])),
		RequestID:     strings.TrimSpace(asString(payload["request_id"])),
		RouteID:       strings.TrimSpace(asString(payload["route_id"])),
	}); err != nil {
		writeError(w, http.StatusBadRequest, "record_credit_transaction_failed", err.Error())
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
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
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
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
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


func (s *Server) getBillingConfig(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	billing := cloneBillingConfig(s.cfg.Billing)
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
		next := cloneBillingConfig(cfg.Billing)

		if value, ok := payload["enabled"]; ok {
			next.Enabled = asBool(value, next.Enabled)
		}
		if value, ok := payload["default_cost"]; ok {
			next.DefaultCost = asInt64(value, next.DefaultCost)
		}
		if value, ok := payload["zero_balance_action"]; ok {
			next.ZeroBalanceAction = strings.ToLower(strings.TrimSpace(asString(value)))
		}
		if value, ok := payload["test_mode_enabled"]; ok {
			next.TestModeEnabled = asBool(value, next.TestModeEnabled)
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
		updated = cloneBillingConfig(next)
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "update_billing_config_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, updated)
}

func (s *Server) getBillingRouteCosts(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	routeCosts := cloneBillingRouteCosts(s.cfg.Billing.RouteCosts)
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
		next := cloneBillingConfig(cfg.Billing)
		if value, ok := payload["route_costs"]; ok {
			routeCosts, err := parseBillingRouteCosts(value)
			if err != nil {
				return err
			}
			next.RouteCosts = routeCosts
		} else {
			routeID := strings.TrimSpace(asString(payload["route_id"]))
			if routeID == "" {
				return errors.New("route_id is required when route_costs is omitted")
			}
			cost := asInt64(payload["cost"], -1)
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
		updated = cloneBillingRouteCosts(next.RouteCosts)
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
		return cloneBillingRouteCosts(v), nil
	case map[string]any:
		out := make(map[string]int64, len(v))
		for rawKey, rawCost := range v {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				return nil, errors.New("route_costs keys cannot be empty")
			}
			cost := asInt64(rawCost, -1)
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
		return cloneBillingMethodMultipliers(v), nil
	case map[string]any:
		out := make(map[string]float64, len(v))
		for rawKey, rawValue := range v {
			key := strings.ToUpper(strings.TrimSpace(rawKey))
			if key == "" {
				return nil, errors.New("method_multipliers keys cannot be empty")
			}
			multiplier, ok := asFloat64(rawValue)
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


func cloneBillingConfig(in config.BillingConfig) config.BillingConfig {
	out := in
	out.RouteCosts = cloneBillingRouteCosts(in.RouteCosts)
	out.MethodMultipliers = cloneBillingMethodMultipliers(in.MethodMultipliers)
	return out
}

func cloneBillingRouteCosts(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return map[string]int64{}
	}
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBillingMethodMultipliers(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
