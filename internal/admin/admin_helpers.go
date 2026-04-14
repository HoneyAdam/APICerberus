package admin

import (
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
)

func normalizeDefault(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	return value
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

func writeError(w http.ResponseWriter, status int, code, message string) {
	_ = jsonutil.WriteJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// writeInternalError logs the actual error and sends a generic message to the client.
// This prevents internal error details (database errors, file paths, etc.) from leaking.
func writeInternalError(w http.ResponseWriter, code string, err error) {
	if err != nil {
		log.Printf("[ERROR] admin: %s: %v", code, err)
	}
	writeError(w, http.StatusInternalServerError, code, "An internal error occurred. Please try again later.")
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

// extractClientIP extracts the client IP from the request, considering X-Forwarded-For header.
func extractClientIP(r *http.Request) string {
	return netutil.ExtractClientIP(r)
}

// SetTrustedProxies configures which proxy IPs are trusted for X-Forwarded-For parsing.
func SetTrustedProxies(proxies []string) {
	netutil.SetTrustedProxies(proxies)
}

// isRateLimited checks if a client IP has exceeded the rate limit for failed auth attempts
func (s *Server) isRateLimited(clientIP string) bool {
	s.rlMu.RLock()
	defer s.rlMu.RUnlock()

	attempts, exists := s.rlAttempts[clientIP]
	if !exists {
		return false
	}

	// If already blocked, check if block duration has passed (30 minutes)
	if attempts.blocked {
		if time.Since(attempts.lastSeen) > 30*time.Minute {
			// Unblock after 30 minutes of no activity
			return false
		}
		return true
	}

	// Check if within rate limit window (15 minutes) and exceeded threshold (5 attempts)
	if time.Since(attempts.firstSeen) <= 15*time.Minute && attempts.count >= 5 {
		return true
	}

	// Reset if outside the window
	if time.Since(attempts.firstSeen) > 15*time.Minute {
		return false
	}

	return false
}

// recordFailedAuth records a failed authentication attempt for a client IP
func (s *Server) recordFailedAuth(clientIP string) {
	s.rlMu.Lock()
	defer s.rlMu.Unlock()

	attempts, exists := s.rlAttempts[clientIP]
	if !exists || time.Since(attempts.firstSeen) > 15*time.Minute {
		// New entry or expired entry - reset
		s.rlAttempts[clientIP] = &adminAuthAttempts{
			count:     1,
			firstSeen: time.Now(),
			lastSeen:  time.Now(),
			blocked:   false,
		}
		return
	}

	// Update existing entry
	attempts.count++
	attempts.lastSeen = time.Now()

	// Block if threshold exceeded
	if attempts.count >= 5 {
		attempts.blocked = true
	}
}

// clearFailedAuth clears failed authentication attempts for a client IP (on successful auth)
func (s *Server) clearFailedAuth(clientIP string) {
	s.rlMu.Lock()
	defer s.rlMu.Unlock()

	delete(s.rlAttempts, clientIP)
}

// startRateLimitCleanup starts the background goroutine for cleaning up old rate limit entries
func (s *Server) startRateLimitCleanup() {
	s.rlCleanupTicker = time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-s.rlCleanupTicker.C:
				s.cleanupOldRateLimitEntries()
			case <-s.rlStopCh:
				s.rlCleanupTicker.Stop()
				return
			}
		}
	}()
}

// cleanupOldRateLimitEntries removes rate limit entries older than 30 minutes
func (s *Server) cleanupOldRateLimitEntries() {
	s.rlMu.Lock()
	defer s.rlMu.Unlock()

	now := time.Now()
	for ip, attempts := range s.rlAttempts {
		if now.Sub(attempts.lastSeen) > 30*time.Minute {
			delete(s.rlAttempts, ip)
		}
	}
}

// isAllowedIP checks whether clientIP matches any entry in allowedIPs (supports CIDR).
func isAllowedIP(clientIP string, allowedIPs []string) bool {
	if len(allowedIPs) == 0 {
		return true
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, rule := range allowedIPs {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if strings.Contains(rule, "/") {
			_, network, err := net.ParseCIDR(rule)
			if err == nil && network.Contains(ip) {
				return true
			}
			continue
		}
		if net.ParseIP(rule).Equal(ip) {
			return true
		}
	}
	return false
}
