package observability

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
)

// ProfilingConfig holds profiling endpoint configuration
type ProfilingConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Path      string `yaml:"path" json:"path"`
	AuthToken string `yaml:"auth_token" json:"auth_token"`
}

// ProfileServer provides runtime profiling endpoints
type ProfileServer struct {
	config ProfilingConfig
}

// NewProfileServer creates a new profiling server
func NewProfileServer(config ProfilingConfig) *ProfileServer {
	if config.Path == "" {
		config.Path = "/debug/pprof"
	}
	return &ProfileServer{config: config}
}

// RegisterRoutes registers profiling endpoints
func (ps *ProfileServer) RegisterRoutes(mux *http.ServeMux) {
	if !ps.config.Enabled {
		return
	}

	base := ps.config.Path

	// Standard pprof endpoints
	mux.HandleFunc(base+"/", ps.wrapAuth(pprof.Index))
	mux.HandleFunc(base+"/cmdline", ps.wrapAuth(pprof.Cmdline))
	mux.HandleFunc(base+"/profile", ps.wrapAuth(pprof.Profile))
	mux.HandleFunc(base+"/symbol", ps.wrapAuth(pprof.Symbol))
	mux.HandleFunc(base+"/trace", ps.wrapAuth(pprof.Trace))
	mux.HandleFunc(base+"/allocs", ps.wrapAuth(pprof.Handler("allocs").ServeHTTP))
	mux.HandleFunc(base+"/block", ps.wrapAuth(pprof.Handler("block").ServeHTTP))
	mux.HandleFunc(base+"/goroutine", ps.wrapAuth(pprof.Handler("goroutine").ServeHTTP))
	mux.HandleFunc(base+"/heap", ps.wrapAuth(pprof.Handler("heap").ServeHTTP))
	mux.HandleFunc(base+"/mutex", ps.wrapAuth(pprof.Handler("mutex").ServeHTTP))
	mux.HandleFunc(base+"/threadcreate", ps.wrapAuth(pprof.Handler("threadcreate").ServeHTTP))

	// Custom endpoints
	mux.HandleFunc(base+"/gc", ps.wrapAuth(ps.handleGC))
	mux.HandleFunc(base+"/stats", ps.wrapAuth(ps.handleStats))
	mux.HandleFunc(base+"/freemem", ps.wrapAuth(ps.handleFreeMemory))
	mux.HandleFunc(base+"/setgc", ps.wrapAuth(ps.handleSetGCPercent))
}

// wrapAuth adds authentication wrapper
func (ps *ProfileServer) wrapAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ps.config.AuthToken != "" {
			token := r.Header.Get("Authorization")
			if token != "Bearer "+ps.config.AuthToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		handler(w, r)
	}
}

// handleGC triggers garbage collection
func (ps *ProfileServer) handleGC(w http.ResponseWriter, r *http.Request) {
	runtime.GC()
	jsonutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Garbage collection triggered",
	})
}

// handleStats returns runtime statistics
func (ps *ProfileServer) handleStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := map[string]interface{}{
		"memory": map[string]interface{}{
			"alloc":         m.Alloc,
			"total_alloc":   m.TotalAlloc,
			"sys":           m.Sys,
			"heap_alloc":    m.HeapAlloc,
			"heap_sys":      m.HeapSys,
			"heap_idle":     m.HeapIdle,
			"heap_inuse":    m.HeapInuse,
			"heap_released": m.HeapReleased,
			"heap_objects":  m.HeapObjects,
			"stacks_inuse":  m.StackInuse,
			"stacks_sys":    m.StackSys,
			"mspan_inuse":   m.MSpanInuse,
			"mspan_sys":     m.MSpanSys,
			"mcache_inuse":  m.MCacheInuse,
			"mcache_sys":    m.MCacheSys,
			"buck_hash_sys": m.BuckHashSys,
			"gc_sys":        m.GCSys,
			"other_sys":     m.OtherSys,
		},
		"gc": map[string]interface{}{
			"num_gc":        m.NumGC,
			"num_forced_gc": m.NumForcedGC,
			"pause_total":   m.PauseTotalNs,
			"pause_avg":     m.PauseNs[(m.NumGC+255)%256],
			"cpu_fraction":  m.GCCPUFraction,
		},
		"goroutines": runtime.NumGoroutine(),
		"cpus":       runtime.NumCPU(),
		"go_version": runtime.Version(),
	}

	jsonutil.WriteJSON(w, http.StatusOK, stats)
}

// handleFreeMemory returns memory to OS
func (ps *ProfileServer) handleFreeMemory(w http.ResponseWriter, r *http.Request) {
	debug.FreeOSMemory()
	jsonutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Memory returned to OS",
	})
}

// handleSetGCPercent sets GC target percentage
func (ps *ProfileServer) handleSetGCPercent(w http.ResponseWriter, r *http.Request) {
	percentStr := r.URL.Query().Get("percent")
	if percentStr == "" {
		http.Error(w, "Missing percent parameter", http.StatusBadRequest)
		return
	}

	percent, err := strconv.Atoi(percentStr)
	if err != nil {
		http.Error(w, "Invalid percent value", http.StatusBadRequest)
		return
	}

	old := debug.SetGCPercent(percent)
	jsonutil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "GC percent updated",
		"old_percent": old,
		"new_percent": percent,
	})
}

// PerformanceMetrics holds performance-related metrics
type PerformanceMetrics struct {
	RequestCount    uint64
	RequestDuration time.Duration
	ActiveRequests  int64
	ErrorsTotal     uint64
}

// Global metrics instance
var globalMetrics = &PerformanceMetrics{}

// RecordRequest records request metrics
func RecordRequest(duration time.Duration, err bool) {
	globalMetrics.RequestCount++
	globalMetrics.RequestDuration = duration
	if err {
		globalMetrics.ErrorsTotal++
	}
}

// GetPerformanceMetrics returns current performance metrics
func GetPerformanceMetrics() *PerformanceMetrics {
	return globalMetrics
}
