package plugin

import (
	"math"
	randv2 "math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries    int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	Jitter        bool
	RetryMethods  []string
	RetryOnStatus []int
}

// Retry controls retries for upstream transport/status failures.
type Retry struct {
	maxRetries  int
	baseDelay   time.Duration
	maxDelay    time.Duration
	jitter      bool
	methods     map[string]struct{}
	retryStatus map[int]struct{}
}

func NewRetry(cfg RetryConfig) *Retry {
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	base := cfg.BaseDelay
	if base <= 0 {
		base = 50 * time.Millisecond
	}
	maxDelay := cfg.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 500 * time.Millisecond
	}

	methods := make(map[string]struct{})
	defaultMethods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	list := cfg.RetryMethods
	if len(list) == 0 {
		list = defaultMethods
	}
	for _, method := range list {
		m := strings.ToUpper(strings.TrimSpace(method))
		if m == "" {
			continue
		}
		methods[m] = struct{}{}
	}
	if len(methods) == 0 {
		for _, m := range defaultMethods {
			methods[m] = struct{}{}
		}
	}

	retryStatus := make(map[int]struct{})
	statusList := cfg.RetryOnStatus
	if len(statusList) == 0 {
		statusList = []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	}
	for _, status := range statusList {
		if status >= 100 {
			retryStatus[status] = struct{}{}
		}
	}

	return &Retry{
		maxRetries:  maxRetries,
		baseDelay:   base,
		maxDelay:    maxDelay,
		jitter:      cfg.Jitter,
		methods:     methods,
		retryStatus: retryStatus,
	}
}

func (r *Retry) Name() string  { return "retry" }
func (r *Retry) Phase() Phase  { return PhaseProxy }
func (r *Retry) Priority() int { return 20 }

func (r *Retry) MaxAttempts(method string) int {
	if !r.IsMethodRetryable(method) {
		return 1
	}
	return r.maxRetries + 1
}

func (r *Retry) IsMethodRetryable(method string) bool {
	_, ok := r.methods[strings.ToUpper(strings.TrimSpace(method))]
	return ok
}

func (r *Retry) IsStatusRetryable(status int) bool {
	_, ok := r.retryStatus[status]
	return ok
}

func (r *Retry) ShouldRetry(method string, attempt int, status int, proxyErr error) bool {
	if !r.IsMethodRetryable(method) {
		return false
	}
	if attempt >= r.maxRetries {
		return false
	}
	if proxyErr != nil {
		return true
	}
	return r.IsStatusRetryable(status)
}

func (r *Retry) Backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return r.baseDelay
	}
	exp := math.Pow(2, float64(attempt))
	delay := time.Duration(exp * float64(r.baseDelay))
	if delay > r.maxDelay {
		delay = r.maxDelay
	}
	if !r.jitter {
		return delay
	}
	// jitter in [50%, 100%] range
	// #nosec G404 -- math/rand/v2 is acceptable for non-cryptographic retry backoff jitter.
	factor := 0.5 + randv2.Float64()*0.5
	jittered := time.Duration(float64(delay) * factor)
	if jittered < time.Millisecond {
		return time.Millisecond
	}
	return jittered
}
