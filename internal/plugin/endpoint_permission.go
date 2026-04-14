package plugin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

const (
	metadataPermissionRateLimitOverride = "permission_rate_limit_override"
	metadataPermissionCreditCost        = "permission_credit_cost"
)

// EndpointPermissionError is returned when endpoint access is denied.
type EndpointPermissionError struct {
	PluginError
}

type EndpointPermissionRecord struct {
	ID           string
	UserID       string
	RouteID      string
	Methods      []string
	Allowed      bool
	RateLimits   map[string]any
	CreditCost   *int64
	ValidFrom    *time.Time
	ValidUntil   *time.Time
	AllowedDays  []int
	AllowedHours []string
}

type EndpointPermissionLookupFunc func(userID, routeID string) (*EndpointPermissionRecord, error)

type EndpointPermission struct {
	lookup EndpointPermissionLookupFunc
	now    func() time.Time
}

func NewEndpointPermission(lookup EndpointPermissionLookupFunc) *EndpointPermission {
	return &EndpointPermission{
		lookup: lookup,
		now:    time.Now,
	}
}

func (p *EndpointPermission) Name() string  { return "endpoint-permission" }
func (p *EndpointPermission) Phase() Phase  { return PhasePreProxy }
func (p *EndpointPermission) Priority() int { return 15 }

func (p *EndpointPermission) Evaluate(ctx *PipelineContext) error {
	if p == nil || p.lookup == nil || ctx == nil || ctx.Request == nil || ctx.Route == nil || ctx.Consumer == nil {
		return nil
	}
	userID := strings.TrimSpace(ctx.Consumer.ID)
	if userID == "" {
		// Deny access when consumer identity cannot be determined (CWE-285)
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_denied",
				Message: "Endpoint access requires an authenticated consumer",
				Status:  http.StatusForbidden,
			},
		}
	}
	routeID := permissionRouteID(ctx.Route)
	if routeID == "" {
		return nil
	}

	permission, err := p.lookup(userID, routeID)
	if err != nil {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_backend_error",
				Message: "Permission check backend unavailable",
				Status:  http.StatusInternalServerError,
			},
		}
	}
	if permission == nil {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_denied",
				Message: "Endpoint access is not granted for this user",
				Status:  http.StatusForbidden,
			},
		}
	}
	if !permission.Allowed {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_denied",
				Message: "Endpoint access is denied for this user",
				Status:  http.StatusForbidden,
			},
		}
	}

	method := strings.ToUpper(strings.TrimSpace(ctx.Request.Method))
	if !isMethodAllowed(permission.Methods, method) {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_method_denied",
				Message: "This HTTP method is not allowed for the endpoint",
				Status:  http.StatusForbidden,
			},
		}
	}

	now := p.now()
	if permission.ValidFrom != nil && now.Before(*permission.ValidFrom) {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_not_yet_valid",
				Message: "Endpoint permission is not active yet",
				Status:  http.StatusForbidden,
			},
		}
	}
	if permission.ValidUntil != nil && now.After(*permission.ValidUntil) {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_expired",
				Message: "Endpoint permission has expired",
				Status:  http.StatusForbidden,
			},
		}
	}

	if len(permission.AllowedDays) > 0 {
		if !containsDay(permission.AllowedDays, int(now.Weekday())) {
			return &EndpointPermissionError{
				PluginError: PluginError{
					Code:    "permission_day_denied",
					Message: "Endpoint permission does not allow access today",
					Status:  http.StatusForbidden,
				},
			}
		}
	}
	if len(permission.AllowedHours) > 0 && !isCurrentHourAllowed(now, permission.AllowedHours) {
		return &EndpointPermissionError{
			PluginError: PluginError{
				Code:    "permission_hour_denied",
				Message: "Endpoint permission does not allow access at this time",
				Status:  http.StatusForbidden,
			},
		}
	}

	if ctx.Metadata == nil {
		ctx.Metadata = map[string]any{}
	}
	if len(permission.RateLimits) > 0 {
		ctx.Metadata[metadataPermissionRateLimitOverride] = permission.RateLimits
	}
	if permission.CreditCost != nil {
		ctx.Metadata[metadataPermissionCreditCost] = *permission.CreditCost
	}

	return nil
}

func permissionRouteID(route *config.Route) string {
	if route == nil {
		return ""
	}
	if value := strings.TrimSpace(route.ID); value != "" {
		return value
	}
	return strings.TrimSpace(route.Name)
}

func isMethodAllowed(allowedMethods []string, method string) bool {
	if len(allowedMethods) == 0 {
		return true
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, item := range allowedMethods {
		current := strings.ToUpper(strings.TrimSpace(item))
		if current == "" {
			continue
		}
		if current == "*" || current == method {
			return true
		}
	}
	return false
}

func containsDay(days []int, day int) bool {
	for _, value := range days {
		if value == day {
			return true
		}
	}
	return false
}

func isCurrentHourAllowed(now time.Time, ranges []string) bool {
	minuteOfDay := now.Hour()*60 + now.Minute()
	for _, raw := range ranges {
		start, end, err := parseTimeRange(raw)
		if err != nil {
			continue
		}
		if minuteOfDay >= start && minuteOfDay < end {
			return true
		}
	}
	return false
}

func parseTimeRange(raw string) (int, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, 0, fmt.Errorf("empty range")
	}
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q", raw)
	}
	start, err := parseMinute(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseMinute(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if end <= start {
		return 0, 0, fmt.Errorf("invalid range order %q", raw)
	}
	return start, end, nil
}

func parseMinute(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("empty time")
	}
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid time %q", raw)
		}
		hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, err
		}
		minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, err
		}
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return 0, fmt.Errorf("time out of range %q", raw)
		}
		return hour*60 + minute, nil
	}
	hour, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if hour < 0 || hour > 24 {
		return 0, fmt.Errorf("hour out of range %q", raw)
	}
	return hour * 60, nil
}
