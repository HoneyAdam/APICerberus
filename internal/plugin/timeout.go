package plugin

import (
	"context"
	"net/http"
	"time"
)

// TimeoutConfig sets request timeout duration.
type TimeoutConfig struct {
	Duration time.Duration
}

// Timeout plugin applies per-route context timeout.
type Timeout struct {
	duration time.Duration
}

func NewTimeout(cfg TimeoutConfig) *Timeout {
	d := cfg.Duration
	if d <= 0 {
		d = 5 * time.Second
	}
	return &Timeout{duration: d}
}

func (t *Timeout) Name() string  { return "timeout" }
func (t *Timeout) Phase() Phase  { return PhaseProxy }
func (t *Timeout) Priority() int { return 10 }

func (t *Timeout) Apply(in *PipelineContext) {
	if t == nil || in == nil || in.Request == nil {
		return
	}
	ctx, cancel := context.WithTimeout(in.Request.Context(), t.duration) // #nosec G118 -- cancel is registered in Cleanup and invoked by the pipeline executor.
	in.Request = in.Request.WithContext(ctx)
	in.Cleanup = append(in.Cleanup, cancel)
}

// TimeoutError is returned when timeout plugin receives invalid config at runtime.
type TimeoutError struct {
	Code    string
	Message string
	Status  int
}

func (e *TimeoutError) Error() string { return e.Message }

var ErrTimeoutInvalid = &TimeoutError{
	Code:    "invalid_timeout",
	Message: "Timeout value is invalid",
	Status:  http.StatusBadRequest,
}
