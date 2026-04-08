package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

// BulkOperationResult represents the result of a bulk operation
type BulkOperationResult struct {
	Success   bool              `json:"success"`
	Created   int               `json:"created"`
	Updated   int               `json:"updated"`
	Deleted   int               `json:"deleted"`
	Failed    int               `json:"failed"`
	Errors    []BulkErrorDetail `json:"errors,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// BulkErrorDetail represents an error for a specific item in a bulk operation
type BulkErrorDetail struct {
	Index   int    `json:"index"`
	ID      string `json:"id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BulkServicesRequest represents a request to create multiple services
type BulkServicesRequest struct {
	Services []config.Service `json:"services"`
}

// BulkRoutesRequest represents a request to create multiple routes
type BulkRoutesRequest struct {
	Routes []config.Route `json:"routes"`
}

// BulkDeleteRequest represents a request to delete multiple resources
type BulkDeleteRequest struct {
	Resources []DeleteResource `json:"resources"`
}

// DeleteResource represents a single resource to delete
type DeleteResource struct {
	Type string `json:"type"` // "service", "route", "upstream", "consumer"
	ID   string `json:"id"`
}

// BulkPluginsRequest represents a request to apply plugins to multiple routes
type BulkPluginsRequest struct {
	RouteIDs []string              `json:"route_ids"`
	Plugins  []config.PluginConfig `json:"plugins"`
	Mode     string                `json:"mode"` // "append", "replace", "merge"
}

// BulkTransaction manages a transactional bulk operation
type BulkTransaction struct {
	srv       *Server
	completed bool
}

// NewBulkTransaction creates a new bulk transaction
func (s *Server) NewBulkTransaction() *BulkTransaction {
	return &BulkTransaction{
		srv:       s,
		completed: false,
	}
}

// Complete marks the transaction as completed successfully
func (tx *BulkTransaction) Complete() {
	tx.completed = true
}

// handleBulkServices handles bulk service creation
func (s *Server) handleBulkServices(w http.ResponseWriter, r *http.Request) {
	var req BulkServicesRequest
	if err := jsonutil.ReadJSON(r, &req, 5<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	if len(req.Services) == 0 {
		writeError(w, http.StatusBadRequest, "empty_request", "No services provided")
		return
	}

	if len(req.Services) > 100 {
		writeError(w, http.StatusBadRequest, "too_many_items", "Maximum 100 services allowed per request")
		return
	}

	result := BulkOperationResult{
		Timestamp: time.Now().UTC(),
	}

	// Validate all services first
	validatedServices := make([]config.Service, 0, len(req.Services))
	for i, svc := range req.Services {
		// Generate ID if not provided
		if strings.TrimSpace(svc.ID) == "" {
			id, err := uuid.NewString()
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					Code:    "id_generation_failed",
					Message: err.Error(),
				})
				continue
			}
			svc.ID = id
		}

		// Set defaults
		if strings.TrimSpace(svc.Protocol) == "" {
			svc.Protocol = "http"
		}

		// Validate
		if err := validateServiceInput(svc); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkErrorDetail{
				Index:   i,
				ID:      svc.ID,
				Code:    "validation_failed",
				Message: err.Error(),
			})
			continue
		}

		validatedServices = append(validatedServices, svc)
	}

	// All-or-nothing transaction
	err := s.mutateConfig(func(cfg *config.Config) error {
		// Check for duplicates and upstream existence
		for _, svc := range validatedServices {
			if serviceByID(cfg, svc.ID) != nil {
				return fmt.Errorf("service with id '%s' already exists", svc.ID)
			}
			if serviceByName(cfg, svc.Name) != nil {
				return fmt.Errorf("service with name '%s' already exists", svc.Name)
			}
			if !upstreamExists(cfg, svc.Upstream) {
				return fmt.Errorf("upstream '%s' does not exist", svc.Upstream)
			}
		}

		// All checks passed, add all services
		cfg.Services = append(cfg.Services, validatedServices...)
		return nil
	})

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, BulkErrorDetail{
			Code:    "transaction_failed",
			Message: err.Error(),
		})
		_ = jsonutil.WriteJSON(w, http.StatusBadRequest, result)
		return
	}

	result.Success = true
	result.Created = len(validatedServices)
	_ = jsonutil.WriteJSON(w, http.StatusCreated, result)
}

// handleBulkRoutes handles bulk route creation
func (s *Server) handleBulkRoutes(w http.ResponseWriter, r *http.Request) {
	var req BulkRoutesRequest
	if err := jsonutil.ReadJSON(r, &req, 5<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	if len(req.Routes) == 0 {
		writeError(w, http.StatusBadRequest, "empty_request", "No routes provided")
		return
	}

	if len(req.Routes) > 100 {
		writeError(w, http.StatusBadRequest, "too_many_items", "Maximum 100 routes allowed per request")
		return
	}

	result := BulkOperationResult{
		Timestamp: time.Now().UTC(),
	}

	// Validate all routes first
	validatedRoutes := make([]config.Route, 0, len(req.Routes))
	for i, route := range req.Routes {
		// Generate ID if not provided
		if strings.TrimSpace(route.ID) == "" {
			id, err := uuid.NewString()
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					Code:    "id_generation_failed",
					Message: err.Error(),
				})
				continue
			}
			route.ID = id
		}

		// Validate
		if err := validateRouteInput(route); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkErrorDetail{
				Index:   i,
				ID:      route.ID,
				Code:    "validation_failed",
				Message: err.Error(),
			})
			continue
		}

		validatedRoutes = append(validatedRoutes, route)
	}

	// All-or-nothing transaction
	err := s.mutateConfig(func(cfg *config.Config) error {
		// Check for duplicates and service existence
		for _, route := range validatedRoutes {
			if routeByID(cfg, route.ID) != nil {
				return fmt.Errorf("route with id '%s' already exists", route.ID)
			}
			if routeByName(cfg, route.Name) != nil {
				return fmt.Errorf("route with name '%s' already exists", route.Name)
			}
			if !serviceExists(cfg, route.Service) {
				return fmt.Errorf("service '%s' does not exist", route.Service)
			}
		}

		// All checks passed, add all routes
		cfg.Routes = append(cfg.Routes, validatedRoutes...)
		return nil
	})

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, BulkErrorDetail{
			Code:    "transaction_failed",
			Message: err.Error(),
		})
		_ = jsonutil.WriteJSON(w, http.StatusBadRequest, result)
		return
	}

	result.Success = true
	result.Created = len(validatedRoutes)
	_ = jsonutil.WriteJSON(w, http.StatusCreated, result)
}

// handleBulkDelete handles bulk deletion of resources
func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	var req BulkDeleteRequest
	if err := jsonutil.ReadJSON(r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	if len(req.Resources) == 0 {
		writeError(w, http.StatusBadRequest, "empty_request", "No resources provided")
		return
	}

	if len(req.Resources) > 100 {
		writeError(w, http.StatusBadRequest, "too_many_items", "Maximum 100 resources allowed per request")
		return
	}

	result := BulkOperationResult{
		Timestamp: time.Now().UTC(),
	}

	// Validate all resources first
	validatedDeletions := make([]DeleteResource, 0, len(req.Resources))
	for i, res := range req.Resources {
		res.Type = strings.ToLower(strings.TrimSpace(res.Type))
		res.ID = strings.TrimSpace(res.ID)

		if res.ID == "" {
			result.Failed++
			result.Errors = append(result.Errors, BulkErrorDetail{
				Index:   i,
				Code:    "missing_id",
				Message: "Resource ID is required",
			})
			continue
		}

		switch res.Type {
		case "service", "route", "upstream", "consumer":
			validatedDeletions = append(validatedDeletions, res)
		default:
			result.Failed++
			result.Errors = append(result.Errors, BulkErrorDetail{
				Index:   i,
				ID:      res.ID,
				Code:    "invalid_type",
				Message: fmt.Sprintf("Invalid resource type: %s", res.Type),
			})
		}
	}

	// All-or-nothing transaction
	err := s.mutateConfig(func(cfg *config.Config) error {
		// Check all resources exist and can be deleted
		for _, res := range validatedDeletions {
			switch res.Type {
			case "service":
				idx := serviceIndexByID(cfg, res.ID)
				if idx < 0 {
					return fmt.Errorf("service '%s' not found", res.ID)
				}
				svc := cfg.Services[idx]
				for _, rt := range cfg.Routes {
					if rt.Service == svc.ID || rt.Service == svc.Name {
						return fmt.Errorf("service '%s' is referenced by route '%s'", res.ID, rt.ID)
					}
				}
			case "route":
				if routeIndexByID(cfg, res.ID) < 0 {
					return fmt.Errorf("route '%s' not found", res.ID)
				}
			case "upstream":
				idx := upstreamIndexByID(cfg, res.ID)
				if idx < 0 {
					return fmt.Errorf("upstream '%s' not found", res.ID)
				}
				up := cfg.Upstreams[idx]
				for _, svc := range cfg.Services {
					if svc.Upstream == up.ID || svc.Upstream == up.Name {
						return fmt.Errorf("upstream '%s' is referenced by service '%s'", res.ID, svc.ID)
					}
				}
			case "consumer":
				found := false
				for i := range cfg.Consumers {
					if cfg.Consumers[i].ID == res.ID {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("consumer '%s' not found", res.ID)
				}
			}
		}

		// All checks passed, delete all resources
		for _, res := range validatedDeletions {
			switch res.Type {
			case "service":
				idx := serviceIndexByID(cfg, res.ID)
				cfg.Services = append(cfg.Services[:idx], cfg.Services[idx+1:]...)
			case "route":
				idx := routeIndexByID(cfg, res.ID)
				cfg.Routes = append(cfg.Routes[:idx], cfg.Routes[idx+1:]...)
			case "upstream":
				idx := upstreamIndexByID(cfg, res.ID)
				cfg.Upstreams = append(cfg.Upstreams[:idx], cfg.Upstreams[idx+1:]...)
			case "consumer":
				for i := range cfg.Consumers {
					if cfg.Consumers[i].ID == res.ID {
						cfg.Consumers = append(cfg.Consumers[:i], cfg.Consumers[i+1:]...)
						break
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, BulkErrorDetail{
			Code:    "transaction_failed",
			Message: err.Error(),
		})
		_ = jsonutil.WriteJSON(w, http.StatusBadRequest, result)
		return
	}

	result.Success = true
	result.Deleted = len(validatedDeletions)
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

// handleBulkPlugins handles bulk plugin application to routes
func (s *Server) handleBulkPlugins(w http.ResponseWriter, r *http.Request) {
	var req BulkPluginsRequest
	if err := jsonutil.ReadJSON(r, &req, 2<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	if len(req.RouteIDs) == 0 {
		writeError(w, http.StatusBadRequest, "empty_routes", "No route IDs provided")
		return
	}

	if len(req.RouteIDs) > 100 {
		writeError(w, http.StatusBadRequest, "too_many_routes", "Maximum 100 routes allowed per request")
		return
	}

	if len(req.Plugins) == 0 {
		writeError(w, http.StatusBadRequest, "empty_plugins", "No plugins provided")
		return
	}

	// Normalize mode
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "append"
	}
	if req.Mode != "append" && req.Mode != "replace" && req.Mode != "merge" {
		writeError(w, http.StatusBadRequest, "invalid_mode", "Mode must be 'append', 'replace', or 'merge'")
		return
	}

	result := BulkOperationResult{
		Timestamp: time.Now().UTC(),
	}

	// All-or-nothing transaction
	err := s.mutateConfig(func(cfg *config.Config) error {
		// Verify all routes exist
		routeIndices := make(map[string]int)
		for _, routeID := range req.RouteIDs {
			idx := routeIndexByID(cfg, routeID)
			if idx < 0 {
				return fmt.Errorf("route '%s' not found", routeID)
			}
			routeIndices[routeID] = idx
		}

		// Apply plugins to all routes
		for routeID, idx := range routeIndices {
			route := &cfg.Routes[idx]

			switch req.Mode {
			case "replace":
				// Replace all plugins
				route.Plugins = clonePluginConfigsBulk(req.Plugins)

			case "append":
				// Append new plugins
				route.Plugins = append(route.Plugins, clonePluginConfigsBulk(req.Plugins)...)

			case "merge":
				// Merge by plugin name (update existing, add new)
				existingPlugins := make(map[string]int)
				for i, p := range route.Plugins {
					existingPlugins[p.Name] = i
				}

				for _, newPlugin := range req.Plugins {
					if existingIdx, exists := existingPlugins[newPlugin.Name]; exists {
						// Update existing plugin
						route.Plugins[existingIdx] = newPlugin
					} else {
						// Add new plugin
						route.Plugins = append(route.Plugins, newPlugin)
					}
				}
			}

			// Track update
			_ = routeID // Use routeID to avoid unused variable warning
		}

		return nil
	})

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, BulkErrorDetail{
			Code:    "transaction_failed",
			Message: err.Error(),
		})
		_ = jsonutil.WriteJSON(w, http.StatusBadRequest, result)
		return
	}

	result.Success = true
	result.Updated = len(req.RouteIDs)
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

// RegisterBulkRoutes registers bulk operation endpoints
func (s *Server) RegisterBulkRoutes() {
	s.handle("POST /admin/api/v1/bulk/services", s.handleBulkServices)
	s.handle("POST /admin/api/v1/bulk/routes", s.handleBulkRoutes)
	s.handle("POST /admin/api/v1/bulk/delete", s.handleBulkDelete)
	s.handle("POST /admin/api/v1/bulk/plugins", s.handleBulkPlugins)
}

// BulkDatabaseOperation provides transactional database operations
type BulkDatabaseOperation struct {
	tx *sql.Tx
	db *sql.DB
}

// NewBulkDatabaseOperation starts a new database transaction for bulk operations
func (s *Server) NewBulkDatabaseOperation() (*BulkDatabaseOperation, error) {
	store := s.gateway.Store()
	if store == nil {
		return nil, errors.New("store not available")
	}

	db := store.DB()
	if db == nil {
		return nil, errors.New("database not available")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	return &BulkDatabaseOperation{
		tx: tx,
		db: db,
	}, nil
}

// Commit commits the transaction
func (op *BulkDatabaseOperation) Commit() error {
	if op.tx == nil {
		return errors.New("transaction already completed")
	}
	err := op.tx.Commit()
	op.tx = nil
	return err
}

// Rollback rolls back the transaction
func (op *BulkDatabaseOperation) Rollback() error {
	if op.tx == nil {
		return nil
	}
	err := op.tx.Rollback()
	op.tx = nil
	return err
}

// Exec executes a query within the transaction
func (op *BulkDatabaseOperation) Exec(query string, args ...any) (sql.Result, error) {
	if op.tx == nil {
		return nil, errors.New("transaction not active")
	}
	return op.tx.Exec(query, args...)
}

// QueryRow executes a query returning a single row within the transaction
func (op *BulkDatabaseOperation) QueryRow(query string, args ...any) *sql.Row {
	if op.tx == nil {
		return nil
	}
	return op.tx.QueryRow(query, args...)
}

// Query executes a query within the transaction
func (op *BulkDatabaseOperation) Query(query string, args ...any) (*sql.Rows, error) {
	if op.tx == nil {
		return nil, errors.New("transaction not active")
	}
	return op.tx.Query(query, args...)
}

// BulkImportResult represents the result of a bulk import operation
type BulkImportResult struct {
	Success    bool              `json:"success"`
	Services   BulkResourceStats `json:"services"`
	Routes     BulkResourceStats `json:"routes"`
	Upstreams  BulkResourceStats `json:"upstreams"`
	Consumers  BulkResourceStats `json:"consumers"`
	Errors     []BulkErrorDetail `json:"errors,omitempty"`
	DurationMs int64             `json:"duration_ms"`
	Timestamp  time.Time         `json:"timestamp"`
}

// BulkResourceStats represents statistics for a specific resource type
type BulkResourceStats struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// handleBulkImport handles importing a complete configuration
func (s *Server) handleBulkImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Services  []config.Service  `json:"services"`
		Routes    []config.Route    `json:"routes"`
		Upstreams []config.Upstream `json:"upstreams"`
		Consumers []config.Consumer `json:"consumers"`
		Mode      string            `json:"mode"` // "create", "upsert", "replace"
	}

	if err := jsonutil.ReadJSON(r, &req, 10<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	// Normalize mode
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "create"
	}

	start := time.Now()
	result := BulkImportResult{
		Timestamp: time.Now().UTC(),
	}

	// All-or-nothing transaction
	err := s.mutateConfig(func(cfg *config.Config) error {
		// Import upstreams first (services depend on them)
		for i, up := range req.Upstreams {
			if strings.TrimSpace(up.ID) == "" {
				id, err := uuid.NewString()
				if err != nil {
					result.Errors = append(result.Errors, BulkErrorDetail{
						Index:   i,
						Code:    "id_generation_failed",
						Message: err.Error(),
					})
					result.Upstreams.Failed++
					continue
				}
				up.ID = id
			}

			if err := validateUpstreamInput(up); err != nil {
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					ID:      up.ID,
					Code:    "validation_failed",
					Message: err.Error(),
				})
				result.Upstreams.Failed++
				continue
			}

			existingIdx := upstreamIndexByID(cfg, up.ID)
			if existingIdx >= 0 {
				switch req.Mode {
				case "upsert":
					cfg.Upstreams[existingIdx] = up
					result.Upstreams.Updated++
				case "replace":
					cfg.Upstreams[existingIdx] = up
					result.Upstreams.Updated++
				default:
					result.Upstreams.Skipped++
				}
			} else {
				cfg.Upstreams = append(cfg.Upstreams, up)
				result.Upstreams.Created++
			}
		}

		// Import services (routes depend on them)
		for i, svc := range req.Services {
			if strings.TrimSpace(svc.ID) == "" {
				id, err := uuid.NewString()
				if err != nil {
					result.Errors = append(result.Errors, BulkErrorDetail{
						Index:   i,
						Code:    "id_generation_failed",
						Message: err.Error(),
					})
					result.Services.Failed++
					continue
				}
				svc.ID = id
			}

			if strings.TrimSpace(svc.Protocol) == "" {
				svc.Protocol = "http"
			}

			if err := validateServiceInput(svc); err != nil {
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					ID:      svc.ID,
					Code:    "validation_failed",
					Message: err.Error(),
				})
				result.Services.Failed++
				continue
			}

			if !upstreamExists(cfg, svc.Upstream) {
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					ID:      svc.ID,
					Code:    "upstream_not_found",
					Message: "upstream '" + svc.Upstream + "' does not exist",
				})
				result.Services.Failed++
				continue
			}

			existingIdx := serviceIndexByID(cfg, svc.ID)
			if existingIdx >= 0 {
				switch req.Mode {
				case "upsert":
					cfg.Services[existingIdx] = svc
					result.Services.Updated++
				case "replace":
					cfg.Services[existingIdx] = svc
					result.Services.Updated++
				default:
					result.Services.Skipped++
				}
			} else {
				cfg.Services = append(cfg.Services, svc)
				result.Services.Created++
			}
		}

		// Import routes
		for i, route := range req.Routes {
			if strings.TrimSpace(route.ID) == "" {
				id, err := uuid.NewString()
				if err != nil {
					result.Errors = append(result.Errors, BulkErrorDetail{
						Index:   i,
						Code:    "id_generation_failed",
						Message: err.Error(),
					})
					result.Routes.Failed++
					continue
				}
				route.ID = id
			}

			if err := validateRouteInput(route); err != nil {
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					ID:      route.ID,
					Code:    "validation_failed",
					Message: err.Error(),
				})
				result.Routes.Failed++
				continue
			}

			if !serviceExists(cfg, route.Service) {
				result.Errors = append(result.Errors, BulkErrorDetail{
					Index:   i,
					ID:      route.ID,
					Code:    "service_not_found",
					Message: "service '" + route.Service + "' does not exist",
				})
				result.Routes.Failed++
				continue
			}

			existingIdx := routeIndexByID(cfg, route.ID)
			if existingIdx >= 0 {
				switch req.Mode {
				case "upsert":
					cfg.Routes[existingIdx] = route
					result.Routes.Updated++
				case "replace":
					cfg.Routes[existingIdx] = route
					result.Routes.Updated++
				default:
					result.Routes.Skipped++
				}
			} else {
				cfg.Routes = append(cfg.Routes, route)
				result.Routes.Created++
			}
		}

		// Import consumers
		for i, consumer := range req.Consumers {
			if strings.TrimSpace(consumer.ID) == "" {
				id, err := uuid.NewString()
				if err != nil {
					result.Errors = append(result.Errors, BulkErrorDetail{
						Index:   i,
						Code:    "id_generation_failed",
						Message: err.Error(),
					})
					result.Consumers.Failed++
					continue
				}
				consumer.ID = id
			}

			found := false
			for j := range cfg.Consumers {
				if cfg.Consumers[j].ID == consumer.ID {
					found = true
					switch req.Mode {
					case "upsert":
						cfg.Consumers[j] = consumer
						result.Consumers.Updated++
					case "replace":
						cfg.Consumers[j] = consumer
						result.Consumers.Updated++
					default:
						result.Consumers.Skipped++
					}
					break
				}
			}

			if !found {
				cfg.Consumers = append(cfg.Consumers, consumer)
				result.Consumers.Created++
			}
		}

		return nil
	})

	result.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, BulkErrorDetail{
			Code:    "transaction_failed",
			Message: err.Error(),
		})
		_ = jsonutil.WriteJSON(w, http.StatusBadRequest, result)
		return
	}

	result.Success = true
	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

// clonePluginConfigsBulk creates a deep copy of plugin configs
func clonePluginConfigsBulk(in []config.PluginConfig) []config.PluginConfig {
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

// RegisterBulkImportRoute registers the bulk import endpoint
func (s *Server) RegisterBulkImportRoute() {
	s.handle("POST /admin/api/v1/bulk/import", s.handleBulkImport)
}

