package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

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
			return errRouteNotFound
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
		if errors.Is(err, errRouteNotFound) {
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
			return errRouteNotFound
		}
		cfg.Routes = append(cfg.Routes[:idx], cfg.Routes[idx+1:]...)
		return nil
	}); err != nil {
		if errors.Is(err, errRouteNotFound) {
			writeError(w, http.StatusNotFound, "route_not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "delete_route_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
