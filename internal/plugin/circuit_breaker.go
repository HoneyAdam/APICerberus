package plugin

import (
	"net/http"
	"sync"
	"time"
)

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// CircuitBreakerConfig controls trip and recovery behavior.
type CircuitBreakerConfig struct {
	ErrorThreshold   float64
	VolumeThreshold  int
	SleepWindow      time.Duration
	HalfOpenRequests int
	Window           time.Duration
}

// CircuitBreakerError indicates breaker is open and request must be rejected.
type CircuitBreakerError struct {
	PluginError
}

var ErrCircuitOpen = &CircuitBreakerError{
	PluginError: PluginError{
		Code:    "circuit_open",
		Message: "Circuit breaker is open",
		Status:  http.StatusServiceUnavailable,
	},
}

type circuitEvent struct {
	ts      time.Time
	success bool
}

// CircuitBreaker plugin guards unstable upstreams.
type CircuitBreaker struct {
	mu sync.Mutex

	state CircuitState
	cfg   CircuitBreakerConfig
	now   func() time.Time

	events []circuitEvent

	openUntil        time.Time
	halfOpenInFlight int
	halfOpenSuccess  int
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.ErrorThreshold <= 0 || cfg.ErrorThreshold > 1 {
		cfg.ErrorThreshold = 0.5
	}
	if cfg.VolumeThreshold <= 0 {
		cfg.VolumeThreshold = 20
	}
	if cfg.SleepWindow <= 0 {
		cfg.SleepWindow = 10 * time.Second
	}
	if cfg.HalfOpenRequests <= 0 {
		cfg.HalfOpenRequests = 1
	}
	if cfg.Window <= 0 {
		cfg.Window = 30 * time.Second
	}
	return &CircuitBreaker{
		state: CircuitClosed,
		cfg:   cfg,
		now:   time.Now,
	}
}

func (cb *CircuitBreaker) Name() string  { return "circuit-breaker" }
func (cb *CircuitBreaker) Phase() Phase  { return PhaseProxy }
func (cb *CircuitBreaker) Priority() int { return 30 }

// Allow checks if request can proceed.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.now()
	switch cb.state {
	case CircuitOpen:
		if now.After(cb.openUntil) || now.Equal(cb.openUntil) {
			cb.state = CircuitHalfOpen
			cb.halfOpenInFlight = 0
			cb.halfOpenSuccess = 0
		} else {
			return ErrCircuitOpen
		}
		fallthrough
	case CircuitHalfOpen:
		if cb.halfOpenInFlight >= cb.cfg.HalfOpenRequests {
			return ErrCircuitOpen
		}
		cb.halfOpenInFlight++
		return nil
	default:
		return nil
	}
}

// Report records request result and updates breaker state.
func (cb *CircuitBreaker) Report(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.now()
	switch cb.state {
	case CircuitHalfOpen:
		if cb.halfOpenInFlight > 0 {
			cb.halfOpenInFlight--
		}
		if success {
			cb.halfOpenSuccess++
			if cb.halfOpenSuccess >= cb.cfg.HalfOpenRequests {
				cb.state = CircuitClosed
				cb.events = nil
				cb.halfOpenSuccess = 0
				cb.halfOpenInFlight = 0
			}
			return
		}
		cb.tripOpenLocked(now)
		return
	case CircuitOpen:
		return
	default:
		cb.events = append(cb.events, circuitEvent{ts: now, success: success})
		cb.pruneEventsLocked(now)

		total := len(cb.events)
		if total < cb.cfg.VolumeThreshold {
			return
		}
		failures := 0
		for _, ev := range cb.events {
			if !ev.success {
				failures++
			}
		}
		errorRate := float64(failures) / float64(total)
		if errorRate >= cb.cfg.ErrorThreshold {
			cb.tripOpenLocked(now)
		}
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == CircuitOpen {
		now := cb.now()
		if now.After(cb.openUntil) || now.Equal(cb.openUntil) {
			cb.state = CircuitHalfOpen
			cb.halfOpenInFlight = 0
			cb.halfOpenSuccess = 0
		}
	}
	return cb.state
}

func (cb *CircuitBreaker) tripOpenLocked(now time.Time) {
	cb.state = CircuitOpen
	cb.openUntil = now.Add(cb.cfg.SleepWindow)
	cb.halfOpenInFlight = 0
	cb.halfOpenSuccess = 0
}

func (cb *CircuitBreaker) pruneEventsLocked(now time.Time) {
	threshold := now.Add(-cb.cfg.Window)
	keepFrom := 0
	for keepFrom < len(cb.events) && cb.events[keepFrom].ts.Before(threshold) {
		keepFrom++
	}
	if keepFrom <= 0 {
		return
	}
	cb.events = append([]circuitEvent(nil), cb.events[keepFrom:]...)
}
