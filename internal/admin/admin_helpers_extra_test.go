package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// --- writeErrorWithID Tests ---

func TestWriteErrorWithID_IncludesRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "req-abc-123")
	w := httptest.NewRecorder()

	writeErrorWithID(req, w, http.StatusBadRequest, "bad_input", "Invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "req-abc-123") {
		t.Errorf("expected request_id in response, got: %s", body)
	}
	if !strings.Contains(body, "bad_input") {
		t.Errorf("expected error code in response, got: %s", body)
	}
}

func TestWriteErrorWithID_EmptyRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	writeErrorWithID(req, w, http.StatusInternalServerError, "internal", "Server error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"request_id":""`) {
		t.Errorf("expected empty request_id in response, got: %s", body)
	}
}

// --- remoteAddrIP Tests ---

func TestRemoteAddrIP_StripsPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"IPv4 with port", "192.168.1.1:8080", "192.168.1.1"},
		{"IPv4 no port", "10.0.0.1", "10.0.0.1"},
		{"IPv6 with port", "[::1]:9876", "::1"},
		{"IPv6 no port", "2001:db8::1", "2001:db8:"},
		{"localhost", "localhost:8080", "localhost"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := remoteAddrIP(tt.input)
			if result != tt.expected {
				t.Errorf("remoteAddrIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- isAllowedIP Tests ---

func TestIsAllowedIP_NoRules(t *testing.T) {
	if !isAllowedIP("1.2.3.4", nil) {
		t.Error("expected true when no allowed IPs configured")
	}
	if !isAllowedIP("1.2.3.4", []string{}) {
		t.Error("expected true when empty allowed IPs")
	}
}

func TestIsAllowedIP_ExactMatch(t *testing.T) {
	if !isAllowedIP("10.0.0.1", []string{"10.0.0.1"}) {
		t.Error("expected exact IP match")
	}
	if isAllowedIP("10.0.0.2", []string{"10.0.0.1"}) {
		t.Error("expected non-matching IP to be denied")
	}
}

func TestIsAllowedIP_CIDR(t *testing.T) {
	allowed := []string{"10.0.0.0/8"}
	if !isAllowedIP("10.1.2.3", allowed) {
		t.Error("expected IP within CIDR to be allowed")
	}
	if isAllowedIP("192.168.1.1", allowed) {
		t.Error("expected IP outside CIDR to be denied")
	}
}

func TestIsAllowedIP_MultipleRules(t *testing.T) {
	allowed := []string{"10.0.0.1", "192.168.0.0/16"}
	if !isAllowedIP("10.0.0.1", allowed) {
		t.Error("expected first rule match")
	}
	if !isAllowedIP("192.168.1.100", allowed) {
		t.Error("expected second rule match")
	}
	if isAllowedIP("172.16.0.1", allowed) {
		t.Error("expected IP matching no rules to be denied")
	}
}

func TestIsAllowedIP_InvalidClientIP(t *testing.T) {
	if isAllowedIP("not-an-ip", []string{"10.0.0.0/8"}) {
		t.Error("expected invalid client IP to be denied")
	}
}

func TestIsAllowedIP_InvalidRule(t *testing.T) {
	allowed := []string{"invalid-cidr", "10.0.0.1"}
	if !isAllowedIP("10.0.0.1", allowed) {
		t.Error("expected valid rule to match despite invalid rule")
	}
}

func TestIsAllowedIP_EmptyRule(t *testing.T) {
	allowed := []string{"", "10.0.0.1"}
	if !isAllowedIP("10.0.0.1", allowed) {
		t.Error("expected empty rule to be skipped")
	}
}

func TestIsAllowedIP_InvalidCIDR(t *testing.T) {
	allowed := []string{"10.0.0.0/33"} // invalid mask
	if isAllowedIP("10.0.0.1", allowed) {
		t.Error("expected invalid CIDR to not match")
	}
}

// --- validateServiceInput Tests ---

func TestValidateServiceInput_MissingName(t *testing.T) {
	err := validateServiceInput(config.Service{Name: "", Upstream: "up1", Protocol: "http"})
	if err == nil {
		t.Error("expected error for missing service name")
	}
}

func TestValidateServiceInput_MissingUpstream(t *testing.T) {
	err := validateServiceInput(config.Service{Name: "svc1", Upstream: "", Protocol: "http"})
	if err == nil {
		t.Error("expected error for missing service upstream")
	}
}

func TestValidateServiceInput_InvalidProtocol(t *testing.T) {
	err := validateServiceInput(config.Service{Name: "svc1", Upstream: "up1", Protocol: "tcp"})
	if err == nil {
		t.Error("expected error for invalid protocol")
	}
}

func TestValidateServiceInput_Valid(t *testing.T) {
	for _, proto := range []string{"http", "grpc", "graphql", "HTTP", "GRPC", "GRAPHQL"} {
		err := validateServiceInput(config.Service{Name: "svc1", Upstream: "up1", Protocol: proto})
		if err != nil {
			t.Errorf("expected valid protocol %q, got error: %v", proto, err)
		}
	}
}

// --- validateRouteInput Tests ---

func TestValidateRouteInput_MissingName(t *testing.T) {
	err := validateRouteInput(config.Route{Name: "", Service: "svc1", Paths: []string{"/api"}})
	if err == nil {
		t.Error("expected error for missing route name")
	}
}

func TestValidateRouteInput_MissingService(t *testing.T) {
	err := validateRouteInput(config.Route{Name: "r1", Service: "", Paths: []string{"/api"}})
	if err == nil {
		t.Error("expected error for missing route service")
	}
}

func TestValidateRouteInput_EmptyPaths(t *testing.T) {
	err := validateRouteInput(config.Route{Name: "r1", Service: "svc1", Paths: []string{}})
	if err == nil {
		t.Error("expected error for empty route paths")
	}
}

func TestValidateRouteInput_Valid(t *testing.T) {
	err := validateRouteInput(config.Route{Name: "r1", Service: "svc1", Paths: []string{"/api"}})
	if err != nil {
		t.Errorf("expected valid route, got error: %v", err)
	}
}

// --- validateUpstreamInput Tests ---

func TestValidateUpstreamInput_MissingName(t *testing.T) {
	err := validateUpstreamInput(config.Upstream{Name: "", Targets: []config.UpstreamTarget{{ID: "t1", Address: "localhost:8080", Weight: 1}}})
	if err == nil {
		t.Error("expected error for missing upstream name")
	}
}

func TestValidateUpstreamInput_EmptyTargets(t *testing.T) {
	err := validateUpstreamInput(config.Upstream{Name: "up1", Targets: []config.UpstreamTarget{}})
	if err == nil {
		t.Error("expected error for empty upstream targets")
	}
}

func TestValidateUpstreamInput_MissingTargetID(t *testing.T) {
	err := validateUpstreamInput(config.Upstream{Name: "up1", Targets: []config.UpstreamTarget{{ID: "", Address: "localhost:8080", Weight: 1}}})
	if err == nil {
		t.Error("expected error for missing target ID")
	}
}

func TestValidateUpstreamInput_MissingTargetAddress(t *testing.T) {
	err := validateUpstreamInput(config.Upstream{Name: "up1", Targets: []config.UpstreamTarget{{ID: "t1", Address: "", Weight: 1}}})
	if err == nil {
		t.Error("expected error for missing target address")
	}
}

func TestValidateUpstreamInput_ZeroWeight(t *testing.T) {
	err := validateUpstreamInput(config.Upstream{Name: "up1", Targets: []config.UpstreamTarget{{ID: "t1", Address: "localhost:8080", Weight: 0}}})
	if err == nil {
		t.Error("expected error for zero target weight")
	}
}

func TestValidateUpstreamInput_Valid(t *testing.T) {
	err := validateUpstreamInput(config.Upstream{Name: "up1", Targets: []config.UpstreamTarget{{ID: "t1", Address: "localhost:8080", Weight: 1}}})
	if err != nil {
		t.Errorf("expected valid upstream, got error: %v", err)
	}
}

// --- service/route/upstream lookup helpers ---

func TestLookupByIDNotFound(t *testing.T) {
	cfg := &config.Config{}

	if serviceByID(cfg, "nonexistent") != nil {
		t.Error("expected nil for unknown service ID")
	}
	if routeByID(cfg, "nonexistent") != nil {
		t.Error("expected nil for unknown route ID")
	}
	if upstreamByID(cfg, "nonexistent") != nil {
		t.Error("expected nil for unknown upstream ID")
	}
	if serviceIndexByID(cfg, "nonexistent") != -1 {
		t.Error("expected -1 for unknown service index")
	}
	if routeIndexByID(cfg, "nonexistent") != -1 {
		t.Error("expected -1 for unknown route index")
	}
	if upstreamIndexByID(cfg, "nonexistent") != -1 {
		t.Error("expected -1 for unknown upstream index")
	}
}

func TestLookupByName(t *testing.T) {
	cfg := &config.Config{}

	if serviceByName(cfg, "nonexistent") != nil {
		t.Error("expected nil for unknown service name")
	}
	if routeByName(cfg, "nonexistent") != nil {
		t.Error("expected nil for unknown route name")
	}
	if upstreamByName(cfg, "nonexistent") != nil {
		t.Error("expected nil for unknown upstream name")
	}
}

func TestExistsFunctions(t *testing.T) {
	cfg := &config.Config{}

	if serviceExists(cfg, "nonexistent") {
		t.Error("expected false for unknown service")
	}
	if upstreamExists(cfg, "nonexistent") {
		t.Error("expected false for unknown upstream")
	}
}

// --- cloneConfig nil ---

func TestCloneConfigNil(t *testing.T) {
	cloned := cloneConfig(nil)
	if cloned == nil {
		t.Error("expected non-nil cloned config for nil input")
	}
}
