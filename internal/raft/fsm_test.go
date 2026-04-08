package raft

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestNewGatewayFSM(t *testing.T) {
	fsm := NewGatewayFSM()
	if fsm == nil {
		t.Fatal("NewGatewayFSM() returned nil")
	}
	if fsm.Routes == nil {
		t.Error("Routes map not initialized")
	}
	if fsm.Services == nil {
		t.Error("Services map not initialized")
	}
	if fsm.Upstreams == nil {
		t.Error("Upstreams map not initialized")
	}
	if fsm.RateLimitCounters == nil {
		t.Error("RateLimitCounters map not initialized")
	}
	if fsm.CreditBalances == nil {
		t.Error("CreditBalances map not initialized")
	}
	if fsm.HealthChecks == nil {
		t.Error("HealthChecks map not initialized")
	}
	if fsm.RequestCounts == nil {
		t.Error("RequestCounts map not initialized")
	}
	if fsm.Certificates == nil {
		t.Error("Certificates map not initialized")
	}
}

func TestGatewayFSM_Apply_AddRoute(t *testing.T) {
	fsm := NewGatewayFSM()

	route := RouteConfig{
		ID:        "route-1",
		Name:      "Test Route",
		ServiceID: "svc-1",
		Hosts:     []string{"example.com"},
		Paths:     []string{"/api"},
		Methods:   []string{"GET"},
	}
	payload, _ := json.Marshal(route)
	cmd := FSMCommand{
		Type:    CmdAddRoute,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	// Verify route was added
	r, ok := fsm.GetRoute("route-1")
	if !ok {
		t.Error("Route not found after Apply")
	}
	if r.Name != "Test Route" {
		t.Errorf("Route name = %v, want Test Route", r.Name)
	}
}

func TestGatewayFSM_Apply_DeleteRoute(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add a route first
	fsm.Routes["route-1"] = &RouteConfig{ID: "route-1", Name: "Test Route"}

	// Delete it
	payload, _ := json.Marshal("route-1")
	cmd := FSMCommand{
		Type:    CmdDeleteRoute,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	// Verify route was deleted
	_, ok := fsm.GetRoute("route-1")
	if ok {
		t.Error("Route still exists after DeleteRoute")
	}
}

func TestGatewayFSM_Apply_AddService(t *testing.T) {
	fsm := NewGatewayFSM()

	service := ServiceConfig{
		ID:       "svc-1",
		Name:     "Test Service",
		Protocol: "http",
		Headers:  map[string]string{"X-Custom": "value"},
	}
	payload, _ := json.Marshal(service)
	cmd := FSMCommand{
		Type:    CmdAddService,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	s, ok := fsm.GetService("svc-1")
	if !ok {
		t.Error("Service not found after Apply")
	}
	if s.Name != "Test Service" {
		t.Errorf("Service name = %v, want Test Service", s.Name)
	}
}

func TestGatewayFSM_Apply_DeleteService(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add a service first
	fsm.Services["svc-1"] = &ServiceConfig{ID: "svc-1", Name: "Test Service"}

	// Delete it
	payload, _ := json.Marshal("svc-1")
	cmd := FSMCommand{
		Type:    CmdDeleteService,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	_, ok := fsm.GetService("svc-1")
	if ok {
		t.Error("Service still exists after DeleteService")
	}
}

func TestGatewayFSM_Apply_AddUpstream(t *testing.T) {
	fsm := NewGatewayFSM()

	upstream := UpstreamConfig{
		ID:        "up-1",
		Name:      "Test Upstream",
		Algorithm: "round_robin",
		Targets: []TargetConfig{
			{ID: "t1", Address: "127.0.0.1:8080", Weight: 100, Healthy: true},
		},
	}
	payload, _ := json.Marshal(upstream)
	cmd := FSMCommand{
		Type:    CmdAddUpstream,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	u, ok := fsm.GetUpstream("up-1")
	if !ok {
		t.Error("Upstream not found after Apply")
	}
	if u.Name != "Test Upstream" {
		t.Errorf("Upstream name = %v, want Test Upstream", u.Name)
	}
	if len(u.Targets) != 1 {
		t.Errorf("Targets length = %v, want 1", len(u.Targets))
	}
}

func TestGatewayFSM_Apply_DeleteUpstream(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add an upstream first
	fsm.Upstreams["up-1"] = &UpstreamConfig{ID: "up-1", Name: "Test Upstream"}

	// Delete it
	payload, _ := json.Marshal("up-1")
	cmd := FSMCommand{
		Type:    CmdDeleteUpstream,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	_, ok := fsm.GetUpstream("up-1")
	if ok {
		t.Error("Upstream still exists after DeleteUpstream")
	}
}

func TestGatewayFSM_Apply_UpdateRateLimit(t *testing.T) {
	fsm := NewGatewayFSM()

	// Increment counter
	update := struct {
		Key   string `json:"key"`
		Count int64  `json:"count"`
		Reset bool   `json:"reset"`
	}{
		Key:   "client-123",
		Count: 5,
		Reset: false,
	}
	payload, _ := json.Marshal(update)
	cmd := FSMCommand{
		Type:    CmdUpdateRateLimit,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	count := fsm.GetRateLimitCounter("client-123")
	if count != 5 {
		t.Errorf("Rate limit counter = %v, want 5", count)
	}

	// Increment again
	update.Count = 3
	payload, _ = json.Marshal(update)
	cmd.Payload = payload
	cmdData, _ = json.Marshal(cmd)
	entry.Command = cmdData
	fsm.Apply(entry)

	count = fsm.GetRateLimitCounter("client-123")
	if count != 8 {
		t.Errorf("Rate limit counter after increment = %v, want 8", count)
	}

	// Reset counter
	update.Reset = true
	update.Count = 0
	payload, _ = json.Marshal(update)
	cmd.Payload = payload
	cmdData, _ = json.Marshal(cmd)
	entry.Command = cmdData
	fsm.Apply(entry)

	count = fsm.GetRateLimitCounter("client-123")
	if count != 0 {
		t.Errorf("Rate limit counter after reset = %v, want 0", count)
	}
}

func TestGatewayFSM_Apply_UpdateCredits(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add credits
	update := struct {
		UserID string `json:"user_id"`
		Amount int64  `json:"amount"`
		Set    bool   `json:"set"`
	}{
		UserID: "user-123",
		Amount: 100,
		Set:    false,
	}
	payload, _ := json.Marshal(update)
	cmd := FSMCommand{
		Type:    CmdUpdateCredits,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	balance := fsm.GetCreditBalance("user-123")
	if balance != 100 {
		t.Errorf("Credit balance = %v, want 100", balance)
	}

	// Add more credits
	update.Amount = 50
	payload, _ = json.Marshal(update)
	cmd.Payload = payload
	cmdData, _ = json.Marshal(cmd)
	entry.Command = cmdData
	fsm.Apply(entry)

	balance = fsm.GetCreditBalance("user-123")
	if balance != 150 {
		t.Errorf("Credit balance after add = %v, want 150", balance)
	}

	// Set credits directly
	update.Set = true
	update.Amount = 500
	payload, _ = json.Marshal(update)
	cmd.Payload = payload
	cmdData, _ = json.Marshal(cmd)
	entry.Command = cmdData
	fsm.Apply(entry)

	balance = fsm.GetCreditBalance("user-123")
	if balance != 500 {
		t.Errorf("Credit balance after set = %v, want 500", balance)
	}
}

func TestGatewayFSM_Apply_UpdateHealthCheck(t *testing.T) {
	fsm := NewGatewayFSM()

	status := HealthStatus{
		ID:        "svc-1",
		Healthy:   true,
		LastCheck: 1234567890,
		Failures:  0,
		Successes: 10,
	}
	payload, _ := json.Marshal(status)
	cmd := FSMCommand{
		Type:    CmdUpdateHealthCheck,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	s, ok := fsm.GetHealthCheck("svc-1")
	if !ok {
		t.Error("Health check not found after Apply")
	}
	if !s.Healthy {
		t.Error("Health check should be healthy")
	}
	if s.Successes != 10 {
		t.Errorf("Successes = %v, want 10", s.Successes)
	}
}

func TestGatewayFSM_Apply_IncrementCounter(t *testing.T) {
	fsm := NewGatewayFSM()

	update := struct {
		Key   string `json:"key"`
		Count int64  `json:"count"`
	}{
		Key:   "requests",
		Count: 1,
	}
	payload, _ := json.Marshal(update)
	cmd := FSMCommand{
		Type:    CmdIncrementCounter,
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	// Apply multiple times
	for i := 0; i < 5; i++ {
		result := fsm.Apply(entry)
		if result != nil {
			t.Errorf("Apply() returned error: %v", result)
		}
	}

	count := fsm.GetRequestCount("requests")
	if count != 5 {
		t.Errorf("Request count = %v, want 5", count)
	}
}

func TestGatewayFSM_Apply_CertificateUpdate(t *testing.T) {
	fsm := NewGatewayFSM()

	update := CertificateUpdateLog{
		Domain:   "example.com",
		CertPEM:  "cert-data",
		KeyPEM:   "key-data",
		IssuedBy: "node-1",
	}
	payload, _ := json.Marshal(update)
	cmd := FSMCommand{
		Type:    "certificate_update",
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}

	cert, ok := fsm.GetCertificate("example.com")
	if !ok {
		t.Error("Certificate not found after Apply")
	}
	if cert.Domain != "example.com" {
		t.Errorf("Certificate domain = %v, want example.com", cert.Domain)
	}
	if cert.CertPEM != "cert-data" {
		t.Errorf("Certificate PEM = %v, want cert-data", cert.CertPEM)
	}
}

func TestGatewayFSM_Apply_ACMERenewalLock(t *testing.T) {
	fsm := NewGatewayFSM()

	lock := ACMERenewalLock{
		Domain:   "example.com",
		NodeID:   "node-1",
		Deadline: time.Now().Add(time.Hour),
	}
	payload, _ := json.Marshal(lock)
	cmd := FSMCommand{
		Type:    "acme_renewal_lock",
		Payload: payload,
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result != nil {
		t.Errorf("Apply() returned error: %v", result)
	}
	// ACME renewal lock doesn't modify FSM state, just ensures log consistency
}

func TestGatewayFSM_Apply_UnknownCommand(t *testing.T) {
	fsm := NewGatewayFSM()

	cmd := FSMCommand{
		Type:    "unknown_command",
		Payload: []byte("{}"),
	}
	cmdData, _ := json.Marshal(cmd)

	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: cmdData,
	}

	result := fsm.Apply(entry)
	if result == nil {
		t.Error("Apply() should return error for unknown command")
	}
}

func TestGatewayFSM_Apply_InvalidCommandData(t *testing.T) {
	fsm := NewGatewayFSM()

	// Invalid JSON in command
	entry := LogEntry{
		Index:   1,
		Term:    1,
		Command: []byte("invalid json"),
	}

	result := fsm.Apply(entry)
	if result == nil {
		t.Error("Apply() should return error for invalid command data")
	}
}

func TestGatewayFSM_Snapshot(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add some data
	fsm.Routes["route-1"] = &RouteConfig{ID: "route-1", Name: "Test Route"}
	fsm.Services["svc-1"] = &ServiceConfig{ID: "svc-1", Name: "Test Service"}
	fsm.RateLimitCounters["client-1"] = 100

	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot == nil {
		t.Fatal("Snapshot() returned nil")
	}
	if len(snapshot) == 0 {
		t.Error("Snapshot() returned empty data")
	}

	// Verify it's valid JSON
	var data map[string]interface{}
	if err := json.Unmarshal(snapshot, &data); err != nil {
		t.Errorf("Snapshot is not valid JSON: %v", err)
	}
}

func TestGatewayFSM_Restore(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add some data
	fsm.Routes["route-1"] = &RouteConfig{ID: "route-1", Name: "Test Route"}
	fsm.RateLimitCounters["client-1"] = 100

	// Take snapshot
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	// Create new FSM and restore
	newFSM := NewGatewayFSM()
	err = newFSM.Restore(snapshot)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify data was restored
	r, ok := newFSM.GetRoute("route-1")
	if !ok {
		t.Error("Route not found after restore")
	}
	if r.Name != "Test Route" {
		t.Errorf("Route name = %v, want Test Route", r.Name)
	}

	count := newFSM.GetRateLimitCounter("client-1")
	if count != 100 {
		t.Errorf("Rate limit counter = %v, want 100", count)
	}
}

func TestGatewayFSM_Restore_InvalidData(t *testing.T) {
	fsm := NewGatewayFSM()

	err := fsm.Restore([]byte("invalid json"))
	if err == nil {
		t.Error("Restore() should return error for invalid JSON")
	}
}

func TestGatewayFSM_GetRoute_NotFound(t *testing.T) {
	fsm := NewGatewayFSM()

	_, ok := fsm.GetRoute("nonexistent")
	if ok {
		t.Error("GetRoute should return false for non-existent route")
	}
}

func TestGatewayFSM_GetService_NotFound(t *testing.T) {
	fsm := NewGatewayFSM()

	_, ok := fsm.GetService("nonexistent")
	if ok {
		t.Error("GetService should return false for non-existent service")
	}
}

func TestGatewayFSM_GetUpstream_NotFound(t *testing.T) {
	fsm := NewGatewayFSM()

	_, ok := fsm.GetUpstream("nonexistent")
	if ok {
		t.Error("GetUpstream should return false for non-existent upstream")
	}
}

func TestGatewayFSM_GetHealthCheck_NotFound(t *testing.T) {
	fsm := NewGatewayFSM()

	_, ok := fsm.GetHealthCheck("nonexistent")
	if ok {
		t.Error("GetHealthCheck should return false for non-existent health check")
	}
}

func TestGatewayFSM_GetCertificate_NotFound(t *testing.T) {
	fsm := NewGatewayFSM()

	_, ok := fsm.GetCertificate("nonexistent.com")
	if ok {
		t.Error("GetCertificate should return false for non-existent certificate")
	}
}

func TestGatewayFSM_GetAllRoutes(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add routes
	fsm.Routes["route-1"] = &RouteConfig{ID: "route-1", Name: "Route 1"}
	fsm.Routes["route-2"] = &RouteConfig{ID: "route-2", Name: "Route 2"}

	routes := fsm.GetAllRoutes()
	if len(routes) != 2 {
		t.Errorf("GetAllRoutes length = %v, want 2", len(routes))
	}

	// Verify it's a copy (modifying returned map shouldn't affect original)
	routes["route-3"] = &RouteConfig{ID: "route-3", Name: "Route 3"}
	if len(fsm.Routes) != 2 {
		t.Error("GetAllRoutes should return a copy of the routes map")
	}
}

func TestGatewayFSM_GetClusterStatus(t *testing.T) {
	fsm := NewGatewayFSM()

	// Add data
	fsm.Routes["route-1"] = &RouteConfig{ID: "route-1"}
	fsm.Services["svc-1"] = &ServiceConfig{ID: "svc-1"}
	fsm.Upstreams["up-1"] = &UpstreamConfig{ID: "up-1"}
	fsm.RateLimitCounters["c1"] = 1
	fsm.CreditBalances["u1"] = 100
	fsm.HealthChecks["h1"] = &HealthStatus{ID: "h1"}
	fsm.RequestCounts["r1"] = 10
	fsm.Certificates["cert1"] = &CertificateState{Domain: "example.com"}

	status := fsm.GetClusterStatus()

	if status["routes_count"] != 1 {
		t.Errorf("routes_count = %v, want 1", status["routes_count"])
	}
	if status["services_count"] != 1 {
		t.Errorf("services_count = %v, want 1", status["services_count"])
	}
	if status["upstreams_count"] != 1 {
		t.Errorf("upstreams_count = %v, want 1", status["upstreams_count"])
	}
	if status["rate_limit_counters"] != 1 {
		t.Errorf("rate_limit_counters = %v, want 1", status["rate_limit_counters"])
	}
	if status["credit_balances"] != 1 {
		t.Errorf("credit_balances = %v, want 1", status["credit_balances"])
	}
	if status["health_checks"] != 1 {
		t.Errorf("health_checks = %v, want 1", status["health_checks"])
	}
	if status["request_counts"] != 1 {
		t.Errorf("request_counts = %v, want 1", status["request_counts"])
	}
	if status["certificates"] != 1 {
		t.Errorf("certificates = %v, want 1", status["certificates"])
	}
}

func TestGatewayFSM_ConcurrentAccess(t *testing.T) {
	fsm := NewGatewayFSM()

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 5; i++ {
		go func(id int) {
			route := RouteConfig{
				ID:   fmt.Sprintf("route-%d", id),
				Name: fmt.Sprintf("Route %d", id),
			}
			payload, _ := json.Marshal(route)
			cmd := FSMCommand{Type: CmdAddRoute, Payload: payload}
			cmdData, _ := json.Marshal(cmd)
			entry := LogEntry{Index: uint64(id + 1), Term: 1, Command: cmdData}
			fsm.Apply(entry)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func(id int) {
			fsm.GetAllRoutes()
			fsm.GetClusterStatus()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify routes were added
	routes := fsm.GetAllRoutes()
	if len(routes) != 5 {
		t.Errorf("Expected 5 routes after concurrent access, got %d", len(routes))
	}
}
