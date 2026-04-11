package portal

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func (s *Server) listMyLogs(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	filters, err := parsePortalLogFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filters", err.Error())
		return
	}
	filters.UserID = user.ID
	result, err := s.store.Audits().Search(filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_logs_failed", "failed to list logs")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": result.Entries,
		"total":   result.Total,
	})
}

func (s *Server) getMyLogDetail(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_log", "log id is required")
		return
	}
	entry, err := s.store.Audits().FindByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get_log_failed", "failed to get log")
		return
	}
	if entry == nil || strings.TrimSpace(entry.UserID) != strings.TrimSpace(user.ID) {
		writeError(w, http.StatusNotFound, "log_not_found", "log entry not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, entry)
}

func (s *Server) exportMyLogs(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	filters, err := parsePortalLogFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filters", err.Error())
		return
	}
	filters.UserID = user.ID
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "jsonl"
	}
	var buf bytes.Buffer
	if err := s.store.Audits().Export(filters, format, &buf); err != nil {
		writeError(w, http.StatusInternalServerError, "export_logs_failed", "failed to export logs")
		return
	}
	w.Header().Set("Content-Type", portalExportContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="portal-logs.%s"`, portalExportExtension(format)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) myBalance(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id": user.ID,
		"balance": user.CreditBalance,
	})
}

func (s *Server) myTransactions(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	query := r.URL.Query()
	limit := asInt(query.Get("limit"), 50)
	offset := asInt(query.Get("offset"), 0)
	txnType := strings.TrimSpace(query.Get("type"))
	result, err := s.store.Credits().ListByUser(user.ID, store.CreditListOptions{
		Type:   txnType,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_transactions_failed", "failed to list credit transactions")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"transactions": result.Transactions,
		"total":        result.Total,
	})
}

func (s *Server) myForecast(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	result, err := s.store.Credits().ListByUser(user.ID, store.CreditListOptions{
		Limit:  500,
		Offset: 0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "forecast_failed", "failed to compute forecast")
		return
	}
	byDay := map[string]int64{}
	for _, txn := range result.Transactions {
		if txn.Amount >= 0 {
			continue
		}
		day := txn.CreatedAt.UTC().Format("2006-01-02")
		byDay[day] += -txn.Amount
	}
	avgDaily := 0.0
	if len(byDay) > 0 {
		var sum int64
		for _, value := range byDay {
			sum += value
		}
		avgDaily = float64(sum) / float64(len(byDay))
	}
	daysRemaining := 0.0
	if avgDaily > 0 {
		daysRemaining = float64(user.CreditBalance) / avgDaily
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"balance":                     user.CreditBalance,
		"average_daily_consumption":   avgDaily,
		"projected_days_remaining":    daysRemaining,
		"consumption_days_considered": len(byDay),
	})
}

func (s *Server) purchaseCredits(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "session_required", "valid session is required")
		return
	}
	payload := map[string]any{}
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	amount := coerce.AsInt64(payload["amount"], 0)
	if amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_amount", "purchase amount must be greater than zero")
		return
	}
	description := strings.TrimSpace(coerce.AsString(payload["description"]))
	if description == "" {
		description = "self purchase"
	}

	newBalance, err := s.store.Users().UpdateCreditBalance(user.ID, amount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "purchase_failed", "failed to apply credit purchase")
		return
	}
	before := newBalance - amount
	if err := s.store.Credits().Create(&store.CreditTransaction{
		UserID:        user.ID,
		Type:          "purchase",
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  newBalance,
		Description:   description,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "purchase_failed", "failed to record purchase transaction")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"purchased":   amount,
		"new_balance": newBalance,
	})
}
