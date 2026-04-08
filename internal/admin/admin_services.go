package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

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
			return errServiceNotFound
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
		if errors.Is(err, errServiceNotFound) {
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
			return errServiceNotFound
		}
		svc := cfg.Services[idx]
		for _, rt := range cfg.Routes {
			if rt.Service == svc.ID || rt.Service == svc.Name {
				return errServiceInUse
			}
		}
		cfg.Services = append(cfg.Services[:idx], cfg.Services[idx+1:]...)
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, errServiceNotFound):
			writeError(w, http.StatusNotFound, "service_not_found", err.Error())
		case errors.Is(err, errServiceInUse):
			writeError(w, http.StatusConflict, "service_in_use", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "delete_service_failed", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
