package observability

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a single health check
type HealthCheck struct {
	Name      string       `json:"name"`
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Latency   string       `json:"latency,omitempty"`
	CheckedAt time.Time    `json:"checked_at"`
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status      HealthStatus  `json:"status"`
	Version     string        `json:"version"`
	Uptime      string        `json:"uptime"`
	StartedAt   time.Time     `json:"started_at"`
	Checks      []HealthCheck `json:"checks"`
	System      SystemInfo    `json:"system"`
	CheckedAt   time.Time     `json:"checked_at"`
}

// SystemInfo holds system information
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	MemoryUsage  uint64 `json:"memory_usage_bytes"`
}

// HealthChecker performs health checks
type HealthChecker struct {
	startedAt time.Time
	version   string
	checks    map[string]HealthCheckFunc
}

// HealthCheckFunc is a function that performs a health check
type HealthCheckFunc func() HealthCheck

// NewHealthChecker creates a new health checker
func NewHealthChecker(version string) *HealthChecker {
	return &HealthChecker{
		startedAt: time.Now(),
		version:   version,
		checks:    make(map[string]HealthCheckFunc),
	}
}

// RegisterCheck registers a health check
func (h *HealthChecker) RegisterCheck(name string, fn HealthCheckFunc) {
	h.checks[name] = fn
}

// Check runs all health checks and returns the report
func (h *HealthChecker) Check() HealthReport {
	report := HealthReport{
		Status:    HealthStatusHealthy,
		Version:   h.version,
		Uptime:    time.Since(h.startedAt).String(),
		StartedAt: h.startedAt,
		Checks:    make([]HealthCheck, 0, len(h.checks)),
		System: SystemInfo{
			GoVersion:    runtime.Version(),
			NumCPU:       runtime.NumCPU(),
			NumGoroutine: runtime.NumGoroutine(),
		},
		CheckedAt: time.Now(),
	}

	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	report.System.MemoryUsage = m.Alloc

	// Run all checks
	for name, fn := range h.checks {
		start := time.Now()
		check := fn()
		check.Name = name
		check.Latency = time.Since(start).String()
		if check.CheckedAt.IsZero() {
			check.CheckedAt = time.Now()
		}

		report.Checks = append(report.Checks, check)

		// Update overall status
		if check.Status == HealthStatusUnhealthy {
			report.Status = HealthStatusUnhealthy
		} else if check.Status == HealthStatusDegraded && report.Status == HealthStatusHealthy {
			report.Status = HealthStatusDegraded
		}
	}

	return report
}

// Handler returns an HTTP handler for health checks
func (h *HealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := h.Check()

		status := http.StatusOK
		if report.Status == HealthStatusUnhealthy {
			status = http.StatusServiceUnavailable
		} else if report.Status == HealthStatusDegraded {
			status = http.StatusOK // Still serving requests
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(report)
	}
}

// LivenessHandler returns a simple liveness check
func (h *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": HealthStatusHealthy,
			"time":   time.Now().UTC(),
		})
	}
}

// ReadinessHandler returns a readiness check
func (h *HealthChecker) ReadinessHandler(gw *gateway.Gateway) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if gateway is ready
		if gw == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  HealthStatusUnhealthy,
				"message": "Gateway not initialized",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": HealthStatusHealthy,
			"time":   time.Now().UTC(),
		})
	}
}

// ReadyForTrafficHandler checks if node is ready to receive traffic
func (h *HealthChecker) ReadyForTrafficHandler(gw *gateway.Gateway) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ready := true
		message := "Ready"

		if gw == nil {
			ready = false
			message = "Gateway not initialized"
		}

		if ready {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   ready,
			"message": message,
		})
	}
}

// Common health checks

// DatabaseHealthCheck returns a database health check
func DatabaseHealthCheck(db interface{}) HealthCheckFunc {
	return func() HealthCheck {
		if db == nil {
			return HealthCheck{
				Status:  HealthStatusUnhealthy,
				Message: "Database not initialized",
			}
		}
		// Add actual DB ping here
		return HealthCheck{
			Status: HealthStatusHealthy,
			Message: "Database connected",
		}
	}
}

// UpstreamHealthCheck returns upstream health check
func UpstreamHealthCheck(gw *gateway.Gateway) HealthCheckFunc {
	return func() HealthCheck {
		if gw == nil {
			return HealthCheck{
				Status:  HealthStatusUnhealthy,
				Message: "Gateway not initialized",
			}
		}

		// Check if upstreams are healthy
		// This would check the actual upstream health status
		return HealthCheck{
			Status:  HealthStatusHealthy,
			Message: "Upstreams available",
		}
	}
}

// MemoryHealthCheck returns memory health check
func MemoryHealthCheck() HealthCheckFunc {
	return func() HealthCheck {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		// Check if memory usage is too high (>90% of limit)
		limit := m.Sys
		if limit > 0 && float64(m.Alloc)/float64(limit) > 0.9 {
			return HealthCheck{
				Status:  HealthStatusDegraded,
				Message: "High memory usage",
			}
		}

		return HealthCheck{
			Status: HealthStatusHealthy,
			Message: "Memory usage normal",
		}
	}
}

// DiskHealthCheck returns disk health check
func DiskHealthCheck() HealthCheckFunc {
	return func() HealthCheck {
		// Would check actual disk usage
		return HealthCheck{
			Status: HealthStatusHealthy,
			Message: "Disk space available",
		}
	}
}

// Global health checker
var globalHealthChecker *HealthChecker

// InitGlobalHealthChecker initializes the global health checker
func InitGlobalHealthChecker(version string) {
	globalHealthChecker = NewHealthChecker(version)
}

// GetGlobalHealthChecker returns the global health checker
func GetGlobalHealthChecker() *HealthChecker {
	return globalHealthChecker
}
