package raft

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// GatewayFSM implements the StateMachine interface for the API Gateway.
type GatewayFSM struct {
	mu sync.RWMutex

	// Configuration state
	Routes    map[string]*RouteConfig    `json:"routes"`
	Services  map[string]*ServiceConfig  `json:"services"`
	Upstreams map[string]*UpstreamConfig `json:"upstreams"`

	// Rate limiting state (cluster-wide counters)
	RateLimitCounters map[string]int64 `json:"rate_limit_counters"`

	// Credit balances (cluster-wide)
	CreditBalances map[string]int64 `json:"credit_balances"`

	// Health check results shared across cluster
	HealthChecks map[string]*HealthStatus `json:"health_checks"`

	// Analytics aggregation
	RequestCounts map[string]int64 `json:"request_counts"`

	// Certificates for cluster-wide TLS sync
	Certificates map[string]*CertificateState `json:"certificates"`
}

// RouteConfig represents a route configuration.
type RouteConfig struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ServiceID string   `json:"service_id"`
	Hosts     []string `json:"hosts"`
	Paths     []string `json:"paths"`
	Methods   []string `json:"methods"`
	StripPath bool     `json:"strip_path"`
	Priority  int      `json:"priority"`
	Version   uint64   `json:"version"`
}

// ServiceConfig represents a service configuration.
type ServiceConfig struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Protocol   string            `json:"protocol"`
	UpstreamID string            `json:"upstream_id"`
	Timeout    int               `json:"timeout"`
	Retries    int               `json:"retries"`
	Headers    map[string]string `json:"headers"`
	Version    uint64            `json:"version"`
}

// UpstreamConfig represents an upstream configuration.
type UpstreamConfig struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Algorithm   string             `json:"algorithm"`
	Targets     []TargetConfig     `json:"targets"`
	HealthCheck *HealthCheckConfig `json:"health_check,omitempty"`
	Version     uint64             `json:"version"`
}

// TargetConfig represents an upstream target.
type TargetConfig struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Weight  int    `json:"weight"`
	Healthy bool   `json:"healthy"`
}

// HealthCheckConfig represents health check settings.
type HealthCheckConfig struct {
	Path               string `json:"path"`
	Interval           int    `json:"interval"`
	Timeout            int    `json:"timeout"`
	HealthyThreshold   int    `json:"healthy_threshold"`
	UnhealthyThreshold int    `json:"unhealthy_threshold"`
}

// HealthStatus represents the health status of a service/target.
type HealthStatus struct {
	ID        string `json:"id"`
	Healthy   bool   `json:"healthy"`
	LastCheck int64  `json:"last_check"`
	Failures  int    `json:"failures"`
	Successes int    `json:"successes"`
}

// FSMCommand represents a command to be applied to the FSM.
type FSMCommand struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Command types
const (
	CmdAddRoute           = "add_route"
	CmdDeleteRoute        = "delete_route"
	CmdAddService         = "add_service"
	CmdDeleteService      = "delete_service"
	CmdAddUpstream        = "add_upstream"
	CmdDeleteUpstream     = "delete_upstream"
	CmdUpdateRateLimit    = "update_rate_limit"
	CmdUpdateCredits      = "update_credits"
	CmdUpdateHealthCheck   = "update_health_check"
	CmdIncrementCounter    = "increment_counter"
	CmdCertificateUpdate   = "certificate_update"
	CmdACMERenewalLock    = "acme_renewal_lock"
)

// certSnapshot is a safe representation of CertificateState used in snapshots.
// KeyPEM is intentionally excluded — private keys must not be persisted to disk.
type certSnapshot struct {
	Domain    string    `json:"domain"`
	CertPEM   string    `json:"cert_pem"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IssuedBy  string    `json:"issued_by"`
}

// fsmSnapshot is a sanitized version of GatewayFSM for disk snapshots.
// Private key material is deliberately excluded.
type fsmSnapshot struct {
	Routes            map[string]*RouteConfig    `json:"routes"`
	Services          map[string]*ServiceConfig  `json:"services"`
	Upstreams         map[string]*UpstreamConfig `json:"upstreams"`
	RateLimitCounters map[string]int64           `json:"rate_limit_counters"`
	CreditBalances    map[string]int64           `json:"credit_balances"`
	HealthChecks      map[string]*HealthStatus   `json:"health_checks"`
	RequestCounts     map[string]int64           `json:"request_counts"`
	Certificates      map[string]*certSnapshot   `json:"certificates"`
}

// NewGatewayFSM creates a new Gateway FSM.
func NewGatewayFSM() *GatewayFSM {
	return &GatewayFSM{
		Routes:            make(map[string]*RouteConfig),
		Services:          make(map[string]*ServiceConfig),
		Upstreams:         make(map[string]*UpstreamConfig),
		RateLimitCounters: make(map[string]int64),
		CreditBalances:    make(map[string]int64),
		HealthChecks:      make(map[string]*HealthStatus),
		RequestCounts:     make(map[string]int64),
		Certificates:      make(map[string]*CertificateState),
	}
}

// Apply applies a log entry to the FSM.
func (f *GatewayFSM) Apply(entry LogEntry) any {
	f.mu.Lock()
	defer f.mu.Unlock()

	var cmd FSMCommand
	if err := json.Unmarshal(entry.Command.([]byte), &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %w", err)
	}

	switch cmd.Type {
	case CmdAddRoute:
		return f.applyAddRoute(cmd.Payload)
	case CmdDeleteRoute:
		return f.applyDeleteRoute(cmd.Payload)
	case CmdAddService:
		return f.applyAddService(cmd.Payload)
	case CmdDeleteService:
		return f.applyDeleteService(cmd.Payload)
	case CmdAddUpstream:
		return f.applyAddUpstream(cmd.Payload)
	case CmdDeleteUpstream:
		return f.applyDeleteUpstream(cmd.Payload)
	case CmdUpdateRateLimit:
		return f.applyUpdateRateLimit(cmd.Payload)
	case CmdUpdateCredits:
		return f.applyUpdateCredits(cmd.Payload)
	case CmdUpdateHealthCheck:
		return f.applyUpdateHealthCheck(cmd.Payload)
	case CmdIncrementCounter:
		return f.applyIncrementCounter(cmd.Payload)
	case CmdCertificateUpdate:
		return f.applyCertificateUpdate(cmd.Payload)
	case CmdACMERenewalLock:
		return f.applyACMERenewalLock(cmd.Payload)
	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// Snapshot returns a snapshot of the FSM state.
// Private key material (KeyPEM) is deliberately excluded so snapshot files
// on disk never contain TLS private keys (CWE-312).
func (f *GatewayFSM) Snapshot() ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	snap := fsmSnapshot{
		Routes:            f.Routes,
		Services:          f.Services,
		Upstreams:         f.Upstreams,
		RateLimitCounters: f.RateLimitCounters,
		CreditBalances:    f.CreditBalances,
		HealthChecks:      f.HealthChecks,
		RequestCounts:     f.RequestCounts,
		Certificates:      make(map[string]*certSnapshot, len(f.Certificates)),
	}
	for domain, cert := range f.Certificates {
		snap.Certificates[domain] = &certSnapshot{
			Domain:    cert.Domain,
			CertPEM:   cert.CertPEM,
			IssuedAt:  cert.IssuedAt,
			ExpiresAt: cert.ExpiresAt,
			IssuedBy:  cert.IssuedBy,
			// KeyPEM intentionally omitted.
		}
	}
	return json.Marshal(snap)
}

// Restore restores the FSM from a snapshot.
// Certificates are restored without KeyPEM; private keys are not persisted in snapshots.
func (f *GatewayFSM) Restore(snapshot []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var snap fsmSnapshot
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return err
	}
	f.Routes = snap.Routes
	f.Services = snap.Services
	f.Upstreams = snap.Upstreams
	f.RateLimitCounters = snap.RateLimitCounters
	f.CreditBalances = snap.CreditBalances
	f.HealthChecks = snap.HealthChecks
	f.RequestCounts = snap.RequestCounts
	f.Certificates = make(map[string]*CertificateState, len(snap.Certificates))
	for domain, c := range snap.Certificates {
		f.Certificates[domain] = &CertificateState{
			Domain:    c.Domain,
			CertPEM:   c.CertPEM,
			IssuedAt:  c.IssuedAt,
			ExpiresAt: c.ExpiresAt,
			IssuedBy:  c.IssuedBy,
			// KeyPEM is not restored from snapshots.
		}
	}
	return nil
}

// Apply methods

func (f *GatewayFSM) applyAddRoute(payload json.RawMessage) error {
	var route RouteConfig
	if err := json.Unmarshal(payload, &route); err != nil {
		return err
	}
	f.Routes[route.ID] = &route
	return nil
}

func (f *GatewayFSM) applyDeleteRoute(payload json.RawMessage) error {
	var id string
	if err := json.Unmarshal(payload, &id); err != nil {
		return err
	}
	delete(f.Routes, id)
	return nil
}

func (f *GatewayFSM) applyAddService(payload json.RawMessage) error {
	var service ServiceConfig
	if err := json.Unmarshal(payload, &service); err != nil {
		return err
	}
	f.Services[service.ID] = &service
	return nil
}

func (f *GatewayFSM) applyDeleteService(payload json.RawMessage) error {
	var id string
	if err := json.Unmarshal(payload, &id); err != nil {
		return err
	}
	delete(f.Services, id)
	return nil
}

func (f *GatewayFSM) applyAddUpstream(payload json.RawMessage) error {
	var upstream UpstreamConfig
	if err := json.Unmarshal(payload, &upstream); err != nil {
		return err
	}
	f.Upstreams[upstream.ID] = &upstream
	return nil
}

func (f *GatewayFSM) applyDeleteUpstream(payload json.RawMessage) error {
	var id string
	if err := json.Unmarshal(payload, &id); err != nil {
		return err
	}
	delete(f.Upstreams, id)
	return nil
}

func (f *GatewayFSM) applyUpdateRateLimit(payload json.RawMessage) error {
	var update struct {
		Key   string `json:"key"`
		Count int64  `json:"count"`
		Reset bool   `json:"reset"`
	}
	if err := json.Unmarshal(payload, &update); err != nil {
		return err
	}
	if update.Reset {
		f.RateLimitCounters[update.Key] = 0
	} else {
		f.RateLimitCounters[update.Key] += update.Count
	}
	return nil
}

func (f *GatewayFSM) applyUpdateCredits(payload json.RawMessage) error {
	var update struct {
		UserID string `json:"user_id"`
		Amount int64  `json:"amount"`
		Set    bool   `json:"set"`
	}
	if err := json.Unmarshal(payload, &update); err != nil {
		return err
	}
	if update.Set {
		f.CreditBalances[update.UserID] = update.Amount
	} else {
		f.CreditBalances[update.UserID] += update.Amount
	}
	return nil
}

func (f *GatewayFSM) applyUpdateHealthCheck(payload json.RawMessage) error {
	var status HealthStatus
	if err := json.Unmarshal(payload, &status); err != nil {
		return err
	}
	f.HealthChecks[status.ID] = &status
	return nil
}

func (f *GatewayFSM) applyIncrementCounter(payload json.RawMessage) error {
	var update struct {
		Key   string `json:"key"`
		Count int64  `json:"count"`
	}
	if err := json.Unmarshal(payload, &update); err != nil {
		return err
	}
	f.RequestCounts[update.Key] += update.Count
	return nil
}

// applyCertificateUpdate applies a certificate update to the FSM
func (f *GatewayFSM) applyCertificateUpdate(payload json.RawMessage) error {
	var update CertificateUpdateLog
	if err := json.Unmarshal(payload, &update); err != nil {
		return fmt.Errorf("failed to unmarshal certificate update: %w", err)
	}
	f.Certificates[update.Domain] = &CertificateState{
		Domain:    update.Domain,
		CertPEM:   update.CertPEM,
		KeyPEM:    update.KeyPEM,
		IssuedAt:  update.IssuedAt,
		ExpiresAt: update.ExpiresAt,
		IssuedBy:  update.IssuedBy,
	}
	return nil
}

// applyACMERenewalLock applies an ACME renewal lock to the FSM
// For now, this is a no-op since the lock is just for coordination
// The actual lock validation happens at the node level
func (f *GatewayFSM) applyACMERenewalLock(payload json.RawMessage) error {
	// ACME renewal locks are coordinated at the node level through the deadline field
	// The FSM just needs to accept this command type to avoid errors
	var lock ACMERenewalLock
	if err := json.Unmarshal(payload, &lock); err != nil {
		return fmt.Errorf("failed to unmarshal ACME renewal lock: %w", err)
	}
	// Lock is tracked - for future use if we want to query lock status
	return nil
}

// Query methods (read-only)

// GetRoute returns a route by ID.
func (f *GatewayFSM) GetRoute(id string) (*RouteConfig, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	route, ok := f.Routes[id]
	return route, ok
}

// GetService returns a service by ID.
func (f *GatewayFSM) GetService(id string) (*ServiceConfig, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	service, ok := f.Services[id]
	return service, ok
}

// GetUpstream returns an upstream by ID.
func (f *GatewayFSM) GetUpstream(id string) (*UpstreamConfig, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	upstream, ok := f.Upstreams[id]
	return upstream, ok
}

// GetRateLimitCounter returns the rate limit counter for a key.
func (f *GatewayFSM) GetRateLimitCounter(key string) int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.RateLimitCounters[key]
}

// GetCreditBalance returns the credit balance for a user.
func (f *GatewayFSM) GetCreditBalance(userID string) int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.CreditBalances[userID]
}

// GetHealthCheck returns the health status for an ID.
func (f *GatewayFSM) GetHealthCheck(id string) (*HealthStatus, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	status, ok := f.HealthChecks[id]
	return status, ok
}

// GetRequestCount returns the request count for a key.
func (f *GatewayFSM) GetRequestCount(key string) int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.RequestCounts[key]
}

// GetAllRoutes returns all routes.
func (f *GatewayFSM) GetAllRoutes() map[string]*RouteConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()

	routes := make(map[string]*RouteConfig)
	for k, v := range f.Routes {
		routes[k] = v
	}
	return routes
}

// GetClusterStatus returns the current cluster status.
func (f *GatewayFSM) GetClusterStatus() map[string]any {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return map[string]any{
		"routes_count":        len(f.Routes),
		"services_count":      len(f.Services),
		"upstreams_count":     len(f.Upstreams),
		"rate_limit_counters": len(f.RateLimitCounters),
		"credit_balances":     len(f.CreditBalances),
		"health_checks":       len(f.HealthChecks),
		"request_counts":      len(f.RequestCounts),
		"certificates":        len(f.Certificates),
	}
}

// GetCertificate returns a certificate by domain
func (f *GatewayFSM) GetCertificate(domain string) (*CertificateState, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cert, ok := f.Certificates[domain]
	return cert, ok
}
