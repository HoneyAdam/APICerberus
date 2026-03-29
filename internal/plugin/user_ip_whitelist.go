package plugin

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// UserIPWhitelistError indicates request was blocked by user-level IP whitelist.
type UserIPWhitelistError struct {
	Code    string
	Message string
	Status  int
}

func (e *UserIPWhitelistError) Error() string { return e.Message }

// UserIPWhitelist enforces user-specific IP whitelist from consumer metadata.
type UserIPWhitelist struct{}

func NewUserIPWhitelist() *UserIPWhitelist {
	return &UserIPWhitelist{}
}

func (p *UserIPWhitelist) Name() string  { return "user-ip-whitelist" }
func (p *UserIPWhitelist) Phase() Phase  { return PhasePreProxy }
func (p *UserIPWhitelist) Priority() int { return 12 }

func (p *UserIPWhitelist) Evaluate(ctx *PipelineContext) error {
	if p == nil || ctx == nil || ctx.Request == nil || ctx.Consumer == nil {
		return nil
	}
	rules := extractUserWhitelistRules(ctx.Consumer)
	if len(rules) == 0 {
		return nil
	}

	exact, cidrs, err := parseIPRules(rules)
	if err != nil {
		return &UserIPWhitelistError{
			Code:    "ip_whitelist_invalid",
			Message: "User IP whitelist is invalid",
			Status:  http.StatusForbidden,
		}
	}
	clientIP := net.ParseIP(requestIP(ctx.Request))
	if clientIP == nil {
		return &UserIPWhitelistError{
			Code:    "ip_not_allowed",
			Message: "IP not allowed",
			Status:  http.StatusForbidden,
		}
	}
	if matchesIPWhitelist(clientIP, exact, cidrs) {
		return nil
	}
	return &UserIPWhitelistError{
		Code:    "ip_not_allowed",
		Message: "IP not allowed",
		Status:  http.StatusForbidden,
	}
}

func extractUserWhitelistRules(consumer *config.Consumer) []string {
	if consumer == nil || len(consumer.Metadata) == 0 {
		return nil
	}
	raw, ok := consumer.Metadata["ip_whitelist"]
	if !ok {
		return nil
	}
	return normalizeIPRuleList(raw)
}

func matchesIPWhitelist(ip net.IP, exact map[string]struct{}, cidrs []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	if _, ok := exact[ip.String()]; ok {
		return true
	}
	for _, network := range cidrs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func normalizeIPRuleList(raw any) []string {
	switch v := raw.(type) {
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
			value := strings.TrimSpace(fmt.Sprint(item))
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
