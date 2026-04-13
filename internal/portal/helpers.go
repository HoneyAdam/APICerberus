package portal

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	"github.com/APICerberus/APICerebrus/internal/store"
)

type portalConfigView struct {
	Routes      []config.Route
	Services    []config.Service
	Billing     config.BillingConfig
	GatewayAddr string
}

func (s *Server) configSnapshot() portalConfigView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	view := portalConfigView{
		Routes:      append([]config.Route(nil), s.cfg.Routes...),
		Services:    append([]config.Service(nil), s.cfg.Services...),
		Billing:     s.cfg.Billing,
		GatewayAddr: s.cfg.Gateway.HTTPAddr,
	}
	view.Billing.RouteCosts = config.CloneInt64Map(s.cfg.Billing.RouteCosts)
	view.Billing.MethodMultipliers = config.CloneFloat64Map(s.cfg.Billing.MethodMultipliers)
	for i := range view.Routes {
		view.Routes[i].Methods = append([]string(nil), s.cfg.Routes[i].Methods...)
		view.Routes[i].Paths = append([]string(nil), s.cfg.Routes[i].Paths...)
		view.Routes[i].Hosts = append([]string(nil), s.cfg.Routes[i].Hosts...)
	}
	return view
}

func buildAPIList(snapshot portalConfigView, permissions []store.EndpointPermission) []map[string]any {
	permsByRoute := map[string]*store.EndpointPermission{}
	for i := range permissions {
		perm := permissions[i]
		permsByRoute[strings.TrimSpace(perm.RouteID)] = &perm
	}

	servicesByID := map[string]config.Service{}
	servicesByName := map[string]config.Service{}
	for _, service := range snapshot.Services {
		servicesByID[strings.TrimSpace(service.ID)] = service
		servicesByName[strings.TrimSpace(service.Name)] = service
	}

	items := make([]map[string]any, 0, len(snapshot.Routes))
	for i := range snapshot.Routes {
		route := snapshot.Routes[i]
		perm := findPermissionForRoute(permsByRoute, &route)
		if len(permissions) > 0 && (perm == nil || !perm.Allowed) {
			continue
		}

		service := servicesByID[route.Service]
		if strings.TrimSpace(service.Name) == "" {
			service = servicesByName[route.Service]
		}
		items = append(items, map[string]any{
			"route_id":     route.ID,
			"route_name":   route.Name,
			"service_name": service.Name,
			"methods":      route.Methods,
			"paths":        route.Paths,
			"hosts":        route.Hosts,
			"credit_cost":  resolveRouteCreditCost(snapshot.Billing, &route, perm),
			"allowed":      true,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return coerce.AsString(items[i]["route_name"]) < coerce.AsString(items[j]["route_name"])
	})
	return items
}

func findAPIDetail(snapshot portalConfigView, permissions []store.EndpointPermission, routeID string) (*config.Route, *config.Service, *store.EndpointPermission) {
	permsByRoute := map[string]*store.EndpointPermission{}
	for i := range permissions {
		perm := permissions[i]
		permsByRoute[strings.TrimSpace(perm.RouteID)] = &perm
	}
	routeID = strings.TrimSpace(routeID)
	for i := range snapshot.Routes {
		route := &snapshot.Routes[i]
		if route.ID != routeID && !strings.EqualFold(strings.TrimSpace(route.Name), routeID) {
			continue
		}
		perm := findPermissionForRoute(permsByRoute, route)
		for j := range snapshot.Services {
			service := &snapshot.Services[j]
			if service.ID == route.Service || strings.EqualFold(service.Name, route.Service) {
				return route, service, perm
			}
		}
		return route, &config.Service{}, perm
	}
	return nil, nil, nil
}

func findPermissionForRoute(permsByRoute map[string]*store.EndpointPermission, route *config.Route) *store.EndpointPermission {
	if route == nil {
		return nil
	}
	if route.ID != "" {
		if perm, ok := permsByRoute[route.ID]; ok {
			return perm
		}
	}
	if route.Name != "" {
		if perm, ok := permsByRoute[route.Name]; ok {
			return perm
		}
	}
	return nil
}

func resolveRouteCreditCost(billing config.BillingConfig, route *config.Route, permission *store.EndpointPermission) int64 {
	if permission != nil && permission.CreditCost != nil {
		return *permission.CreditCost
	}
	if route != nil {
		if cost, ok := billing.RouteCosts[route.ID]; ok {
			return cost
		}
		if cost, ok := billing.RouteCosts[route.Name]; ok {
			return cost
		}
	}
	if billing.DefaultCost > 0 {
		return billing.DefaultCost
	}
	return 0
}

func resolveGatewayBaseURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "http://127.0.0.1:8080"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimSuffix(addr, "/")
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	return "http://" + strings.TrimSuffix(addr, "/")
}

func parsePortalTimeRange(query url.Values) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	to := now
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("to must be RFC3339")
		}
		to = value.UTC()
	}
	window := 24 * time.Hour
	if raw := strings.TrimSpace(query.Get("window")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("window must be a valid duration")
		}
		if parsed > 0 {
			window = parsed
		}
	}
	from := to.Add(-window)
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("from must be RFC3339")
		}
		from = value.UTC()
	}
	if from.After(to) {
		from, to = to, from
	}
	return from, to, nil
}

func parsePortalGranularity(query url.Values) (time.Duration, error) {
	raw := strings.TrimSpace(query.Get("granularity"))
	if raw == "" {
		return time.Hour, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("granularity must be a valid duration")
	}
	if value <= 0 {
		return 0, fmt.Errorf("granularity must be greater than zero")
	}
	if value < time.Minute {
		value = time.Minute
	}
	return value, nil
}

func parsePortalLogFilters(query url.Values) (store.AuditSearchFilters, error) {
	filters := store.AuditSearchFilters{
		Route:    strings.TrimSpace(query.Get("route")),
		Method:   strings.TrimSpace(query.Get("method")),
		ClientIP: strings.TrimSpace(query.Get("client_ip")),
		FullText: strings.TrimSpace(query.Get("q")),
		Limit:    asInt(query.Get("limit"), 50),
		Offset:   asInt(query.Get("offset"), 0),
	}
	if raw := strings.TrimSpace(query.Get("status_min")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return filters, fmt.Errorf("status_min must be numeric")
		}
		filters.StatusMin = v
	}
	if raw := strings.TrimSpace(query.Get("status_max")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return filters, fmt.Errorf("status_max must be numeric")
		}
		filters.StatusMax = v
	}
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return filters, fmt.Errorf("from must be RFC3339")
		}
		v := value.UTC()
		filters.DateFrom = &v
	}
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return filters, fmt.Errorf("to must be RFC3339")
		}
		v := value.UTC()
		filters.DateTo = &v
	}
	return filters, nil
}

func portalExportContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "text/csv; charset=utf-8"
	case "json":
		return "application/json; charset=utf-8"
	default:
		return "application/x-ndjson; charset=utf-8"
	}
}

func portalExportExtension(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		return "csv"
	case "json":
		return "json"
	default:
		return "jsonl"
	}
}

func asInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
