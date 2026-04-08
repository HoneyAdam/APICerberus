package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

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
			return errUpstreamNotFound
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
		if errors.Is(err, errUpstreamNotFound) {
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
			return errUpstreamNotFound
		}
		up := cfg.Upstreams[idx]
		for _, svc := range cfg.Services {
			if svc.Upstream == up.ID || svc.Upstream == up.Name {
				return errUpstreamInUse
			}
		}
		cfg.Upstreams = append(cfg.Upstreams[:idx], cfg.Upstreams[idx+1:]...)
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, errUpstreamNotFound):
			writeError(w, http.StatusNotFound, "upstream_not_found", err.Error())
		case errors.Is(err, errUpstreamInUse):
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
			return errUpstreamNotFound
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
		if errors.Is(err, errUpstreamNotFound) {
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
			return errUpstreamNotFound
		}
		targets := cfg.Upstreams[idx].Targets
		for i := range targets {
			if targets[i].ID == targetID {
				cfg.Upstreams[idx].Targets = append(targets[:i], targets[i+1:]...)
				return nil
			}
		}
		return errTargetNotFound
	}); err != nil {
		switch {
		case errors.Is(err, errUpstreamNotFound):
			writeError(w, http.StatusNotFound, "upstream_not_found", err.Error())
		case errors.Is(err, errTargetNotFound):
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
