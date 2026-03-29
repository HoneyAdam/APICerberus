package admin

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
	"github.com/APICerberus/APICerebrus/internal/store"
	"github.com/APICerberus/APICerebrus/internal/version"
)

// Server hosts Admin REST API endpoints.
type Server struct {
	mu      sync.RWMutex
	cfg     *config.Config
	gateway *gateway.Gateway
	mux     *http.ServeMux

	startedAt time.Time
}

// NewServer initializes admin routes.
func NewServer(cfg *config.Config, gw *gateway.Gateway) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	if gw == nil {
		return nil, errors.New("gateway is nil")
	}

	s := &Server{
		cfg:       cfg,
		gateway:   gw,
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.handle("GET /admin/api/v1/status", s.handleStatus)
	s.handle("GET /admin/api/v1/info", s.handleInfo)
	s.handle("POST /admin/api/v1/config/reload", s.handleConfigReload)

	s.handle("GET /admin/api/v1/services", s.listServices)
	s.handle("POST /admin/api/v1/services", s.createService)
	s.handle("GET /admin/api/v1/services/{id}", s.getService)
	s.handle("PUT /admin/api/v1/services/{id}", s.updateService)
	s.handle("DELETE /admin/api/v1/services/{id}", s.deleteService)

	s.handle("GET /admin/api/v1/routes", s.listRoutes)
	s.handle("POST /admin/api/v1/routes", s.createRoute)
	s.handle("GET /admin/api/v1/routes/{id}", s.getRoute)
	s.handle("PUT /admin/api/v1/routes/{id}", s.updateRoute)
	s.handle("DELETE /admin/api/v1/routes/{id}", s.deleteRoute)

	s.handle("GET /admin/api/v1/upstreams", s.listUpstreams)
	s.handle("POST /admin/api/v1/upstreams", s.createUpstream)
	s.handle("GET /admin/api/v1/upstreams/{id}", s.getUpstream)
	s.handle("PUT /admin/api/v1/upstreams/{id}", s.updateUpstream)
	s.handle("DELETE /admin/api/v1/upstreams/{id}", s.deleteUpstream)
	s.handle("POST /admin/api/v1/upstreams/{id}/targets", s.addUpstreamTarget)
	s.handle("DELETE /admin/api/v1/upstreams/{id}/targets/{tid}", s.deleteUpstreamTarget)
	s.handle("GET /admin/api/v1/upstreams/{id}/health", s.getUpstreamHealth)

	s.handle("GET /admin/api/v1/users", s.listUsers)
	s.handle("POST /admin/api/v1/users", s.createUser)
	s.handle("GET /admin/api/v1/users/{id}", s.getUser)
	s.handle("PUT /admin/api/v1/users/{id}", s.updateUser)
	s.handle("DELETE /admin/api/v1/users/{id}", s.deleteUser)
	s.handle("POST /admin/api/v1/users/{id}/suspend", s.suspendUser)
	s.handle("POST /admin/api/v1/users/{id}/activate", s.activateUser)
	s.handle("POST /admin/api/v1/users/{id}/reset-password", s.resetUserPassword)

	s.handle("GET /admin/api/v1/users/{id}/api-keys", s.listUserAPIKeys)
	s.handle("POST /admin/api/v1/users/{id}/api-keys", s.createUserAPIKey)
	s.handle("DELETE /admin/api/v1/users/{id}/api-keys/{keyId}", s.revokeUserAPIKey)

	s.handle("GET /admin/api/v1/users/{id}/permissions", s.listUserPermissions)
	s.handle("POST /admin/api/v1/users/{id}/permissions", s.createUserPermission)
	s.handle("PUT /admin/api/v1/users/{id}/permissions/{pid}", s.updateUserPermission)
	s.handle("DELETE /admin/api/v1/users/{id}/permissions/{pid}", s.deleteUserPermission)
	s.handle("POST /admin/api/v1/users/{id}/permissions/bulk", s.bulkAssignUserPermissions)

	s.handle("GET /admin/api/v1/users/{id}/ip-whitelist", s.listUserIPWhitelist)
	s.handle("POST /admin/api/v1/users/{id}/ip-whitelist", s.addUserIPWhitelist)
	s.handle("DELETE /admin/api/v1/users/{id}/ip-whitelist/{ip}", s.deleteUserIPWhitelist)

	s.handle("GET /admin/api/v1/credits/overview", s.creditOverview)
	s.handle("POST /admin/api/v1/users/{id}/credits/topup", s.topupCredits)
	s.handle("POST /admin/api/v1/users/{id}/credits/deduct", s.deductCredits)
	s.handle("GET /admin/api/v1/users/{id}/credits/balance", s.userCreditBalance)
	s.handle("GET /admin/api/v1/users/{id}/credits/transactions", s.listCreditTransactions)
	s.handle("GET /admin/api/v1/audit-logs", s.searchAuditLogs)
	s.handle("GET /admin/api/v1/audit-logs/{id}", s.getAuditLog)
	s.handle("GET /admin/api/v1/audit-logs/export", s.exportAuditLogs)
	s.handle("GET /admin/api/v1/audit-logs/stats", s.auditLogStats)
	s.handle("DELETE /admin/api/v1/audit-logs/cleanup", s.cleanupAuditLogs)
	s.handle("GET /admin/api/v1/users/{id}/audit-logs", s.searchUserAuditLogs)

	s.handle("GET /admin/api/v1/billing/config", s.getBillingConfig)
	s.handle("PUT /admin/api/v1/billing/config", s.updateBillingConfig)
	s.handle("GET /admin/api/v1/billing/route-costs", s.getBillingRouteCosts)
	s.handle("PUT /admin/api/v1/billing/route-costs", s.updateBillingRouteCosts)
}

func (s *Server) handle(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, s.withAdminAuth(handler))
}

func (s *Server) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		expected := s.cfg.Admin.APIKey
		s.mu.RUnlock()

		provided := r.Header.Get("X-Admin-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			writeError(w, http.StatusUnauthorized, "admin_unauthorized", "Invalid admin key")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	startedAt := s.startedAt
	s.mu.RUnlock()

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"version":    version.Version,
		"commit":     version.Commit,
		"build_time": version.BuildTime,
		"uptime_sec": int(time.Since(startedAt).Seconds()),
		"summary": map[string]any{
			"services":  len(cfg.Services),
			"routes":    len(cfg.Routes),
			"upstreams": len(cfg.Upstreams),
		},
	})
}

func (s *Server) handleConfigReload(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	next := cloneConfig(s.cfg)
	s.mu.RUnlock()

	if err := s.gateway.Reload(next); err != nil {
		writeError(w, http.StatusBadRequest, "config_reload_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"reloaded": true})
}

func (s *Server) listServices(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_ = jsonutil.WriteJSON(w, http.StatusOK, s.cfg.Services)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var in config.Service
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		in.ID = id
	}
	if strings.TrimSpace(in.Protocol) == "" {
		in.Protocol = "http"
	}
	if err := validateServiceInput(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_service", err.Error())
		return
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		if serviceByID(cfg, in.ID) != nil {
			return errors.New("service id already exists")
		}
		if serviceByName(cfg, in.Name) != nil {
			return errors.New("service name already exists")
		}
		if !upstreamExists(cfg, in.Upstream) {
			return errors.New("referenced upstream does not exist")
		}
		cfg.Services = append(cfg.Services, in)
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "create_service_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, in)
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	defer s.mu.RUnlock()
	svc := serviceByID(s.cfg, id)
	if svc == nil {
		writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, svc)
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in config.Service
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		in.ID = id
	}
	if in.ID != id {
		writeError(w, http.StatusBadRequest, "invalid_service", "path id and payload id must match")
		return
	}
	if strings.TrimSpace(in.Protocol) == "" {
		in.Protocol = "http"
	}
	if err := validateServiceInput(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_service", err.Error())
		return
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := serviceIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("service not found")
		}
		if !upstreamExists(cfg, in.Upstream) {
			return errors.New("referenced upstream does not exist")
		}
		for i := range cfg.Services {
			if i != idx && strings.EqualFold(cfg.Services[i].Name, in.Name) {
				return errors.New("service name already exists")
			}
		}
		cfg.Services[idx] = in
		return nil
	}); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "service not found" {
			status = http.StatusNotFound
		}
		writeError(w, status, "update_service_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, in)
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := serviceIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("service not found")
		}
		svc := cfg.Services[idx]
		for _, rt := range cfg.Routes {
			if rt.Service == svc.ID || rt.Service == svc.Name {
				return errors.New("service is referenced by route")
			}
		}
		cfg.Services = append(cfg.Services[:idx], cfg.Services[idx+1:]...)
		return nil
	}); err != nil {
		switch err.Error() {
		case "service not found":
			writeError(w, http.StatusNotFound, "service_not_found", err.Error())
		case "service is referenced by route":
			writeError(w, http.StatusConflict, "service_in_use", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "delete_service_failed", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRoutes(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_ = jsonutil.WriteJSON(w, http.StatusOK, s.cfg.Routes)
}

func (s *Server) createRoute(w http.ResponseWriter, r *http.Request) {
	var in config.Route
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		in.ID = id
	}
	if len(in.Methods) == 0 {
		in.Methods = []string{http.MethodGet}
	}
	if err := validateRouteInput(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_route", err.Error())
		return
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		if routeByID(cfg, in.ID) != nil {
			return errors.New("route id already exists")
		}
		if routeByName(cfg, in.Name) != nil {
			return errors.New("route name already exists")
		}
		if !serviceExists(cfg, in.Service) {
			return errors.New("referenced service does not exist")
		}
		cfg.Routes = append(cfg.Routes, in)
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "create_route_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, in)
}

func (s *Server) getRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	defer s.mu.RUnlock()
	route := routeByID(s.cfg, id)
	if route == nil {
		writeError(w, http.StatusNotFound, "route_not_found", "Route not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, route)
}

func (s *Server) updateRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in config.Route
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		in.ID = id
	}
	if in.ID != id {
		writeError(w, http.StatusBadRequest, "invalid_route", "path id and payload id must match")
		return
	}
	if len(in.Methods) == 0 {
		in.Methods = []string{http.MethodGet}
	}
	if err := validateRouteInput(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_route", err.Error())
		return
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := routeIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("route not found")
		}
		if !serviceExists(cfg, in.Service) {
			return errors.New("referenced service does not exist")
		}
		for i := range cfg.Routes {
			if i != idx && strings.EqualFold(cfg.Routes[i].Name, in.Name) {
				return errors.New("route name already exists")
			}
		}
		cfg.Routes[idx] = in
		return nil
	}); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "route not found" {
			status = http.StatusNotFound
		}
		writeError(w, status, "update_route_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, in)
}

func (s *Server) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := routeIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("route not found")
		}
		cfg.Routes = append(cfg.Routes[:idx], cfg.Routes[idx+1:]...)
		return nil
	}); err != nil {
		if err.Error() == "route not found" {
			writeError(w, http.StatusNotFound, "route_not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "delete_route_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listUpstreams(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_ = jsonutil.WriteJSON(w, http.StatusOK, s.cfg.Upstreams)
}

func (s *Server) createUpstream(w http.ResponseWriter, r *http.Request) {
	var in config.Upstream
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		in.ID = id
	}
	if strings.TrimSpace(in.Algorithm) == "" {
		in.Algorithm = "round_robin"
	}
	if err := validateUpstreamInput(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upstream", err.Error())
		return
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		if upstreamByID(cfg, in.ID) != nil {
			return errors.New("upstream id already exists")
		}
		if upstreamByName(cfg, in.Name) != nil {
			return errors.New("upstream name already exists")
		}
		cfg.Upstreams = append(cfg.Upstreams, in)
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "create_upstream_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, in)
}

func (s *Server) getUpstream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	defer s.mu.RUnlock()
	up := upstreamByID(s.cfg, id)
	if up == nil {
		writeError(w, http.StatusNotFound, "upstream_not_found", "Upstream not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, up)
}

func (s *Server) updateUpstream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in config.Upstream
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		in.ID = id
	}
	if in.ID != id {
		writeError(w, http.StatusBadRequest, "invalid_upstream", "path id and payload id must match")
		return
	}
	if strings.TrimSpace(in.Algorithm) == "" {
		in.Algorithm = "round_robin"
	}
	if err := validateUpstreamInput(in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upstream", err.Error())
		return
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := upstreamIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("upstream not found")
		}
		for i := range cfg.Upstreams {
			if i != idx && strings.EqualFold(cfg.Upstreams[i].Name, in.Name) {
				return errors.New("upstream name already exists")
			}
		}
		cfg.Upstreams[idx] = in
		return nil
	}); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "upstream not found" {
			status = http.StatusNotFound
		}
		writeError(w, status, "update_upstream_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, in)
}

func (s *Server) deleteUpstream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := upstreamIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("upstream not found")
		}
		up := cfg.Upstreams[idx]
		for _, svc := range cfg.Services {
			if svc.Upstream == up.ID || svc.Upstream == up.Name {
				return errors.New("upstream is referenced by service")
			}
		}
		cfg.Upstreams = append(cfg.Upstreams[:idx], cfg.Upstreams[idx+1:]...)
		return nil
	}); err != nil {
		switch err.Error() {
		case "upstream not found":
			writeError(w, http.StatusNotFound, "upstream_not_found", err.Error())
		case "upstream is referenced by service":
			writeError(w, http.StatusConflict, "upstream_in_use", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "delete_upstream_failed", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addUpstreamTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in config.UpstreamTarget
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		generated, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		in.ID = generated
	}
	if strings.TrimSpace(in.Address) == "" {
		writeError(w, http.StatusBadRequest, "invalid_target", "target address is required")
		return
	}
	if in.Weight <= 0 {
		in.Weight = 100
	}

	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := upstreamIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("upstream not found")
		}
		for _, t := range cfg.Upstreams[idx].Targets {
			if t.ID == in.ID {
				return errors.New("target id already exists")
			}
		}
		cfg.Upstreams[idx].Targets = append(cfg.Upstreams[idx].Targets, in)
		return nil
	}); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "upstream not found" {
			status = http.StatusNotFound
		}
		writeError(w, status, "add_target_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, in)
}

func (s *Server) deleteUpstreamTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	targetID := r.PathValue("tid")

	if err := s.mutateConfig(func(cfg *config.Config) error {
		idx := upstreamIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("upstream not found")
		}
		targets := cfg.Upstreams[idx].Targets
		for i := range targets {
			if targets[i].ID == targetID {
				cfg.Upstreams[idx].Targets = append(targets[:i], targets[i+1:]...)
				return nil
			}
		}
		return errors.New("target not found")
	}); err != nil {
		switch err.Error() {
		case "upstream not found":
			writeError(w, http.StatusNotFound, "upstream_not_found", err.Error())
		case "target not found":
			writeError(w, http.StatusNotFound, "target_not_found", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "delete_target_failed", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getUpstreamHealth(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	up := upstreamByID(s.cfg, id)
	s.mu.RUnlock()
	if up == nil {
		writeError(w, http.StatusNotFound, "upstream_not_found", "Upstream not found")
		return
	}

	health := s.gateway.UpstreamHealth(up.Name)
	targets := make([]map[string]any, 0, len(up.Targets))
	for _, t := range up.Targets {
		targets = append(targets, map[string]any{
			"id":      t.ID,
			"address": t.Address,
			"healthy": health[t.ID],
		})
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"upstream_id":   up.ID,
		"upstream_name": up.Name,
		"targets":       targets,
	})
}

func (s *Server) mutateConfig(mutator func(*config.Config) error) error {
	s.mu.RLock()
	next := cloneConfig(s.cfg)
	s.mu.RUnlock()

	if err := mutator(next); err != nil {
		return err
	}
	if err := s.gateway.Reload(next); err != nil {
		return err
	}

	s.mu.Lock()
	s.cfg = next
	s.mu.Unlock()
	return nil
}

func (s *Server) openStore() (*store.Store, error) {
	s.mu.RLock()
	cfg := cloneConfig(s.cfg)
	s.mu.RUnlock()
	return store.Open(cfg)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	query := r.URL.Query()
	limit, _ := strconv.Atoi(strings.TrimSpace(query.Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(query.Get("offset")))
	sortDesc := strings.EqualFold(strings.TrimSpace(query.Get("sort_desc")), "true")

	result, err := st.Users().List(store.UserListOptions{
		Search:   strings.TrimSpace(query.Get("search")),
		Status:   strings.TrimSpace(query.Get("status")),
		Role:     strings.TrimSpace(query.Get("role")),
		SortBy:   strings.TrimSpace(query.Get("sort_by")),
		SortDesc: sortDesc,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_users_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	password := strings.TrimSpace(asString(payload["password"]))
	if password == "" {
		password = "change-me-user"
	}
	passwordHash, err := store.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user", err.Error())
		return
	}

	user := &store.User{
		Email:         strings.TrimSpace(asString(payload["email"])),
		Name:          strings.TrimSpace(asString(payload["name"])),
		Company:       strings.TrimSpace(asString(payload["company"])),
		PasswordHash:  passwordHash,
		Role:          normalizeDefault(asString(payload["role"]), "user"),
		Status:        normalizeDefault(asString(payload["status"]), "active"),
		CreditBalance: int64(asInt(payload["credit_balance"], asInt(payload["initial_credits"], 0))),
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Users().Create(user); err != nil {
		writeError(w, http.StatusBadRequest, "create_user_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, user)
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "get_user_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, user)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_user_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	if value := strings.TrimSpace(asString(payload["email"])); value != "" {
		user.Email = value
	}
	if value := strings.TrimSpace(asString(payload["name"])); value != "" {
		user.Name = value
	}
	if value := strings.TrimSpace(asString(payload["company"])); value != "" {
		user.Company = value
	}
	if value := strings.TrimSpace(asString(payload["role"])); value != "" {
		user.Role = value
	}
	if value := strings.TrimSpace(asString(payload["status"])); value != "" {
		user.Status = value
	}
	if _, ok := payload["credit_balance"]; ok {
		user.CreditBalance = int64(asInt(payload["credit_balance"], int(user.CreditBalance)))
	}
	if password := strings.TrimSpace(asString(payload["password"])); password != "" {
		hash, err := store.HashPassword(password)
		if err != nil {
			writeError(w, http.StatusBadRequest, "update_user_failed", err.Error())
			return
		}
		user.PasswordHash = hash
	}
	if value, ok := payload["ip_whitelist"]; ok {
		user.IPWhitelist = asStringSlice(value)
	}
	if value, ok := payload["metadata"].(map[string]any); ok {
		user.Metadata = value
	}
	if value, ok := payload["rate_limits"].(map[string]any); ok {
		user.RateLimits = value
	}

	if err := st.Users().Update(user); err != nil {
		if errors.Is(err, store.ErrInsufficientCredits) {
			writeError(w, http.StatusPaymentRequired, "insufficient_credits", err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_user_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, user)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Users().Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "delete_user_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) suspendUser(w http.ResponseWriter, r *http.Request) {
	s.updateUserStatus(w, r, "suspended")
}

func (s *Server) activateUser(w http.ResponseWriter, r *http.Request) {
	s.updateUserStatus(w, r, "active")
}

func (s *Server) updateUserStatus(w http.ResponseWriter, r *http.Request, status string) {
	id := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Users().UpdateStatus(id, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_user_status_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status})
}

func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	password := strings.TrimSpace(asString(payload["password"]))
	if password == "" {
		writeError(w, http.StatusBadRequest, "invalid_password", "password is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	user, err := st.Users().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reset_password_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	hash, err := store.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "reset_password_failed", err.Error())
		return
	}
	user.PasswordHash = hash
	if err := st.Users().Update(user); err != nil {
		writeError(w, http.StatusBadRequest, "reset_password_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "password_reset": true})
}

func (s *Server) listUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	keys, err := st.APIKeys().ListByUser(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_api_keys_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, keys)
}

func (s *Server) createUserAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	name := strings.TrimSpace(asString(payload["name"]))
	mode := strings.TrimSpace(asString(payload["mode"]))
	if mode == "" {
		mode = "live"
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	fullKey, key, err := st.APIKeys().Create(userID, name, mode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeError(w, http.StatusBadRequest, "create_api_key_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"full_key": fullKey,
		"key":      key,
	})
}

func (s *Server) revokeUserAPIKey(w http.ResponseWriter, r *http.Request) {
	keyID := strings.TrimSpace(r.PathValue("keyId"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.APIKeys().Revoke(keyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "api_key_not_found", "API key not found")
			return
		}
		writeError(w, http.StatusBadRequest, "revoke_api_key_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listUserPermissions(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	permissions, err := st.Permissions().ListByUser(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_permissions_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, permissions)
}

func (s *Server) createUserPermission(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	permission, err := decodePermissionPayload(payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_permission", err.Error())
		return
	}
	permission.UserID = userID

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Permissions().Create(permission); err != nil {
		writeError(w, http.StatusBadRequest, "create_permission_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, permission)
}

func (s *Server) updateUserPermission(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	permissionID := strings.TrimSpace(r.PathValue("pid"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	permission, err := decodePermissionPayload(payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_permission", err.Error())
		return
	}
	permission.ID = permissionID
	permission.UserID = userID

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	if err := st.Permissions().Update(permission); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "permission_not_found", "Permission not found")
			return
		}
		writeError(w, http.StatusBadRequest, "update_permission_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, permission)
}

func (s *Server) deleteUserPermission(w http.ResponseWriter, r *http.Request) {
	permissionID := strings.TrimSpace(r.PathValue("pid"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	if err := st.Permissions().Delete(permissionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "permission_not_found", "Permission not found")
			return
		}
		writeError(w, http.StatusBadRequest, "delete_permission_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) bulkAssignUserPermissions(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	rawPermissions, ok := payload["permissions"].([]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_permissions", "permissions array is required")
		return
	}
	permissions := make([]store.EndpointPermission, 0, len(rawPermissions))
	for _, raw := range rawPermissions {
		item, ok := raw.(map[string]any)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_permissions", "permission item must be object")
			return
		}
		permission, err := decodePermissionPayload(item)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_permissions", err.Error())
			return
		}
		permissions = append(permissions, *permission)
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	if err := st.Permissions().BulkAssign(userID, permissions); err != nil {
		writeError(w, http.StatusBadRequest, "bulk_assign_permissions_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"assigned": len(permissions)})
}

func (s *Server) listUserIPWhitelist(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "list_ip_whitelist_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"ip_whitelist": user.IPWhitelist})
}

func (s *Server) addUserIPWhitelist(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	var payload map[string]any
	if err := jsonutil.ReadJSON(r, &payload, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	ips := asStringSlice(payload["ips"])
	if len(ips) == 0 {
		if value := strings.TrimSpace(asString(payload["ip"])); value != "" {
			ips = []string{value}
		}
	}
	if len(ips) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_ip", "ip or ips is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_ip_whitelist_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(user.IPWhitelist)+len(ips))
	for _, item := range user.IPWhitelist {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	for _, item := range ips {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		merged = append(merged, item)
	}
	user.IPWhitelist = merged
	if err := st.Users().Update(user); err != nil {
		writeError(w, http.StatusBadRequest, "update_ip_whitelist_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"ip_whitelist": user.IPWhitelist})
}

func (s *Server) deleteUserIPWhitelist(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	ipValue := strings.TrimSpace(r.PathValue("ip"))
	if decoded, err := url.PathUnescape(ipValue); err == nil {
		ipValue = strings.TrimSpace(decoded)
	}
	if ipValue == "" {
		writeError(w, http.StatusBadRequest, "invalid_ip", "ip is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()
	user, err := st.Users().FindByID(userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "delete_ip_whitelist_failed", err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}
	next := make([]string, 0, len(user.IPWhitelist))
	for _, item := range user.IPWhitelist {
		if strings.TrimSpace(item) == ipValue {
			continue
		}
		next = append(next, item)
	}
	user.IPWhitelist = next
	if err := st.Users().Update(user); err != nil {
		writeError(w, http.StatusBadRequest, "delete_ip_whitelist_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"ip_whitelist": user.IPWhitelist})
}

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

func (s *Server) searchAuditLogs(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	result, err := st.Audits().Search(filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "search_audit_logs_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) searchUserAuditLogs(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}
	filters.UserID = strings.TrimSpace(r.PathValue("id"))

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	result, err := st.Audits().Search(filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "search_user_audit_logs_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) getAuditLog(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_audit_id", "audit log id is required")
		return
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	entry, err := st.Audits().FindByID(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "get_audit_log_failed", err.Error())
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "audit_log_not_found", "Audit log not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, entry)
}

func (s *Server) auditLogStats(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}
	filters.Limit = 0
	filters.Offset = 0

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	stats, err := st.Audits().Stats(filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit_stats_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, stats)
}

func (s *Server) exportAuditLogs(w http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditSearchFilters(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_audit_filters", err.Error())
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "jsonl"
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	var body bytes.Buffer
	if err := st.Audits().Export(filters, format, &body); err != nil {
		writeError(w, http.StatusBadRequest, "export_audit_logs_failed", err.Error())
		return
	}

	fileExt := auditExportFileExtension(format)
	fileName := fmt.Sprintf("audit-logs-%s.%s", time.Now().UTC().Format("20060102-150405"), fileExt)
	w.Header().Set("Content-Type", auditExportContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func (s *Server) cleanupAuditLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cutoff, err := resolveAuditCleanupCutoff(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cleanup_cutoff", err.Error())
		return
	}

	batchSize := 1000
	if raw := strings.TrimSpace(query.Get("batch_size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_batch_size", "batch_size must be numeric")
			return
		}
		if parsed > 0 {
			batchSize = parsed
		}
	}

	st, err := s.openStore()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_open_failed", err.Error())
		return
	}
	defer st.Close()

	deleted, err := st.Audits().DeleteOlderThan(cutoff, batchSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, "audit_cleanup_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":    deleted,
		"cutoff":     cutoff.UTC().Format(time.RFC3339Nano),
		"batch_size": batchSize,
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

func decodePermissionPayload(payload map[string]any) (*store.EndpointPermission, error) {
	if payload == nil {
		return nil, errors.New("permission payload is required")
	}
	permission := &store.EndpointPermission{
		ID:           strings.TrimSpace(asString(payload["id"])),
		RouteID:      strings.TrimSpace(asString(payload["route_id"])),
		Methods:      asStringSlice(payload["methods"]),
		Allowed:      asBool(payload["allowed"], true),
		RateLimits:   asAnyMap(payload["rate_limits"]),
		AllowedDays:  asIntSlice(payload["allowed_days"]),
		AllowedHours: asStringSlice(payload["allowed_hours"]),
	}
	if permission.RouteID == "" {
		return nil, errors.New("route_id is required")
	}
	if value, ok := payload["credit_cost"]; ok {
		raw := strings.TrimSpace(asString(value))
		if raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, errors.New("credit_cost must be numeric")
			}
			permission.CreditCost = &parsed
		}
	}
	if value := strings.TrimSpace(asString(payload["valid_from"])); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, errors.New("valid_from must be RFC3339")
			}
		}
		permission.ValidFrom = &parsed
	}
	if value := strings.TrimSpace(asString(payload["valid_until"])); value != "" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, errors.New("valid_until must be RFC3339")
			}
		}
		permission.ValidUntil = &parsed
	}
	return permission, nil
}

func normalizeDefault(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	return value
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

func parseAuditSearchFilters(query url.Values) (store.AuditSearchFilters, error) {
	filters := store.AuditSearchFilters{
		UserID:       strings.TrimSpace(query.Get("user_id")),
		APIKeyPrefix: strings.TrimSpace(query.Get("api_key_prefix")),
		Route:        strings.TrimSpace(query.Get("route")),
		Method:       strings.TrimSpace(query.Get("method")),
		ClientIP:     strings.TrimSpace(query.Get("client_ip")),
		BlockReason:  strings.TrimSpace(query.Get("block_reason")),
		FullText:     strings.TrimSpace(firstNonEmpty(query.Get("q"), query.Get("search"))),
	}

	if raw := strings.TrimSpace(firstNonEmpty(query.Get("status_min"), query.Get("status_code_min"))); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("status_min must be numeric")
		}
		filters.StatusMin = value
	}
	if raw := strings.TrimSpace(firstNonEmpty(query.Get("status_max"), query.Get("status_code_max"))); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("status_max must be numeric")
		}
		filters.StatusMax = value
	}
	if raw := strings.TrimSpace(query.Get("min_latency_ms")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return filters, errors.New("min_latency_ms must be numeric")
		}
		filters.MinLatencyMS = value
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("limit must be numeric")
		}
		filters.Limit = value
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filters, errors.New("offset must be numeric")
		}
		filters.Offset = value
	}
	if raw := strings.TrimSpace(query.Get("blocked")); raw != "" {
		value, err := parseBoolString(raw)
		if err != nil {
			return filters, errors.New("blocked must be true or false")
		}
		filters.Blocked = &value
	}
	if raw := strings.TrimSpace(query.Get("date_from")); raw != "" {
		value, err := parseAuditTime(raw)
		if err != nil {
			return filters, errors.New("date_from must be RFC3339")
		}
		filters.DateFrom = &value
	}
	if raw := strings.TrimSpace(query.Get("date_to")); raw != "" {
		value, err := parseAuditTime(raw)
		if err != nil {
			return filters, errors.New("date_to must be RFC3339")
		}
		filters.DateTo = &value
	}
	return filters, nil
}

func resolveAuditCleanupCutoff(query url.Values) (time.Time, error) {
	if raw := strings.TrimSpace(query.Get("cutoff")); raw != "" {
		value, err := parseAuditTime(raw)
		if err != nil {
			return time.Time{}, errors.New("cutoff must be RFC3339")
		}
		return value, nil
	}

	days := 30
	if raw := strings.TrimSpace(query.Get("older_than_days")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return time.Time{}, errors.New("older_than_days must be numeric")
		}
		if value > 0 {
			days = value
		}
	}
	return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour), nil
}

func parseAuditTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("empty time value")
	}
	if value, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return value, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func parseBoolString(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("invalid boolean")
	}
}

func auditExportContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "text/csv; charset=utf-8"
	case "json":
		return "application/json; charset=utf-8"
	default:
		return "application/x-ndjson; charset=utf-8"
	}
}

func auditExportFileExtension(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "csv"
	case "json":
		return "json"
	default:
		return "jsonl"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func asAnyMap(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

func asStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			value := strings.TrimSpace(asString(item))
			if value != "" {
				out = append(out, value)
			}
		}
		return out
	case string:
		value := strings.TrimSpace(v)
		if value == "" {
			return nil
		}
		if strings.Contains(value, ",") {
			parts := strings.Split(value, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					out = append(out, part)
				}
			}
			return out
		}
		return []string{value}
	default:
		return nil
	}
}

func asIntSlice(value any) []int {
	switch v := value.(type) {
	case []int:
		return append([]int(nil), v...)
	case []any:
		out := make([]int, 0, len(v))
		for _, item := range v {
			out = append(out, asInt(item, 0))
		}
		return out
	default:
		return nil
	}
}

func asBool(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			return fallback
		}
		return v == "1" || v == "true" || v == "yes" || v == "on"
	default:
		return fallback
	}
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.ReplaceAll(fmt.Sprint(value), "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	return strings.TrimSpace(text)
}

func asInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func asInt64(value any, fallback int64) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func asFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	_ = jsonutil.WriteJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func validateServiceInput(svc config.Service) error {
	if strings.TrimSpace(svc.Name) == "" {
		return errors.New("service name is required")
	}
	if strings.TrimSpace(svc.Upstream) == "" {
		return errors.New("service upstream is required")
	}
	switch strings.ToLower(strings.TrimSpace(svc.Protocol)) {
	case "http", "grpc", "graphql":
	default:
		return errors.New("service protocol must be http, grpc, or graphql")
	}
	return nil
}

func validateRouteInput(route config.Route) error {
	if strings.TrimSpace(route.Name) == "" {
		return errors.New("route name is required")
	}
	if strings.TrimSpace(route.Service) == "" {
		return errors.New("route service is required")
	}
	if len(route.Paths) == 0 {
		return errors.New("route must define at least one path")
	}
	return nil
}

func validateUpstreamInput(up config.Upstream) error {
	if strings.TrimSpace(up.Name) == "" {
		return errors.New("upstream name is required")
	}
	if len(up.Targets) == 0 {
		return errors.New("upstream must include at least one target")
	}
	for _, t := range up.Targets {
		if strings.TrimSpace(t.ID) == "" {
			return errors.New("upstream target id is required")
		}
		if strings.TrimSpace(t.Address) == "" {
			return errors.New("upstream target address is required")
		}
		if t.Weight <= 0 {
			return errors.New("upstream target weight must be greater than zero")
		}
	}
	return nil
}

func cloneConfig(src *config.Config) *config.Config {
	if src == nil {
		return &config.Config{}
	}
	out := *src
	out.Billing = cloneBillingConfig(src.Billing)
	out.Services = append([]config.Service(nil), src.Services...)
	out.Routes = append([]config.Route(nil), src.Routes...)
	out.GlobalPlugins = clonePluginConfigs(src.GlobalPlugins)
	for i := range out.Routes {
		out.Routes[i].Plugins = clonePluginConfigs(src.Routes[i].Plugins)
	}

	out.Upstreams = append([]config.Upstream(nil), src.Upstreams...)
	for i := range out.Upstreams {
		out.Upstreams[i].Targets = append([]config.UpstreamTarget(nil), src.Upstreams[i].Targets...)
	}

	out.Consumers = append([]config.Consumer(nil), src.Consumers...)
	for i := range out.Consumers {
		out.Consumers[i].APIKeys = append([]config.ConsumerAPIKey(nil), src.Consumers[i].APIKeys...)
		out.Consumers[i].ACLGroups = append([]string(nil), src.Consumers[i].ACLGroups...)
		if src.Consumers[i].Metadata != nil {
			out.Consumers[i].Metadata = make(map[string]any, len(src.Consumers[i].Metadata))
			for k, v := range src.Consumers[i].Metadata {
				out.Consumers[i].Metadata[k] = v
			}
		}
	}
	out.Auth.APIKey.KeyNames = append([]string(nil), src.Auth.APIKey.KeyNames...)
	out.Auth.APIKey.QueryNames = append([]string(nil), src.Auth.APIKey.QueryNames...)
	out.Auth.APIKey.CookieNames = append([]string(nil), src.Auth.APIKey.CookieNames...)
	return &out
}

func clonePluginConfigs(in []config.PluginConfig) []config.PluginConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.PluginConfig, len(in))
	for i := range in {
		out[i] = in[i]
		if in[i].Enabled != nil {
			v := *in[i].Enabled
			out[i].Enabled = &v
		}
		if in[i].Config != nil {
			out[i].Config = make(map[string]any, len(in[i].Config))
			for k, v := range in[i].Config {
				out[i].Config[k] = v
			}
		}
	}
	return out
}

func serviceByID(cfg *config.Config, id string) *config.Service {
	for i := range cfg.Services {
		if cfg.Services[i].ID == id {
			return &cfg.Services[i]
		}
	}
	return nil
}

func serviceByName(cfg *config.Config, name string) *config.Service {
	for i := range cfg.Services {
		if strings.EqualFold(cfg.Services[i].Name, name) {
			return &cfg.Services[i]
		}
	}
	return nil
}

func serviceIndexByID(cfg *config.Config, id string) int {
	for i := range cfg.Services {
		if cfg.Services[i].ID == id {
			return i
		}
	}
	return -1
}

func routeByID(cfg *config.Config, id string) *config.Route {
	for i := range cfg.Routes {
		if cfg.Routes[i].ID == id {
			return &cfg.Routes[i]
		}
	}
	return nil
}

func routeByName(cfg *config.Config, name string) *config.Route {
	for i := range cfg.Routes {
		if strings.EqualFold(cfg.Routes[i].Name, name) {
			return &cfg.Routes[i]
		}
	}
	return nil
}

func routeIndexByID(cfg *config.Config, id string) int {
	for i := range cfg.Routes {
		if cfg.Routes[i].ID == id {
			return i
		}
	}
	return -1
}

func upstreamByID(cfg *config.Config, id string) *config.Upstream {
	for i := range cfg.Upstreams {
		if cfg.Upstreams[i].ID == id {
			return &cfg.Upstreams[i]
		}
	}
	return nil
}

func upstreamByName(cfg *config.Config, name string) *config.Upstream {
	for i := range cfg.Upstreams {
		if strings.EqualFold(cfg.Upstreams[i].Name, name) {
			return &cfg.Upstreams[i]
		}
	}
	return nil
}

func upstreamIndexByID(cfg *config.Config, id string) int {
	for i := range cfg.Upstreams {
		if cfg.Upstreams[i].ID == id {
			return i
		}
	}
	return -1
}

func upstreamExists(cfg *config.Config, nameOrID string) bool {
	return upstreamByID(cfg, nameOrID) != nil || upstreamByName(cfg, nameOrID) != nil
}

func serviceExists(cfg *config.Config, nameOrID string) bool {
	return serviceByID(cfg, nameOrID) != nil || serviceByName(cfg, nameOrID) != nil
}
