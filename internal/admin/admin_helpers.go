package admin

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
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
	if len(src.Audit.RouteRetentionDays) > 0 {
		out.Audit.RouteRetentionDays = make(map[string]int, len(src.Audit.RouteRetentionDays))
		for route, days := range src.Audit.RouteRetentionDays {
			out.Audit.RouteRetentionDays[route] = days
		}
	}
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

// extractClientIP extracts the client IP from the request, considering X-Forwarded-For header.
// When trusted_proxies is configured, forwarded headers are only trusted when the direct
// connection comes from a trusted proxy. When trusted_proxies is empty (default), all
// forwarding headers are trusted for backward compatibility.
func extractClientIP(r *http.Request) string {
	remoteIP := remoteAddrIP(r.RemoteAddr)

	// When trusted_proxies is configured, only parse forwarding headers from trusted sources.
	// When empty (default), trust all forwarding headers for backward compatibility.
	trustHeaders := len(trustedProxies) == 0 || trustedProxies[remoteIP]

	if trustHeaders {
		// Check X-Forwarded-For header first (for proxied requests)
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if clientIP != "" {
					return clientIP
				}
			}
		}

		// Fall back to X-Real-Ip header
		xri := r.Header.Get("X-Real-Ip")
		if xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

// remoteAddrIP strips the port from a RemoteAddr and normalizes IPv6 brackets.
func remoteAddrIP(remoteAddr string) string {
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		remoteAddr = remoteAddr[:idx]
	}
	return strings.Trim(remoteAddr, "[]")
}

// trustedProxies holds the set of trusted proxy IPs. When nil/empty, all proxies are trusted.
var trustedProxies map[string]bool

// SetTrustedProxies configures which proxy IPs are trusted for X-Forwarded-For parsing.
// When the list is empty, all forwarding headers are trusted (backward compatible).
func SetTrustedProxies(proxies []string) {
	if len(proxies) == 0 {
		trustedProxies = nil
		return
	}
	trustedProxies = make(map[string]bool, len(proxies))
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if p != "" {
			trustedProxies[p] = true
		}
	}
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
