package plugin

import (
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

// CorrelationID propagates a request ID through request/response flow.
type CorrelationID struct{}

func NewCorrelationID() *CorrelationID {
	return &CorrelationID{}
}

func (c *CorrelationID) Name() string  { return "correlation-id" }
func (c *CorrelationID) Phase() Phase  { return PhasePreAuth }
func (c *CorrelationID) Priority() int { return 0 }

func (c *CorrelationID) Apply(in *PipelineContext) {
	if c == nil || in == nil || in.Request == nil {
		return
	}

	id := strings.TrimSpace(in.Request.Header.Get("X-Request-ID"))
	if id == "" {
		generated, err := uuid.NewString()
		if err != nil {
			generated = "req-generated"
		}
		id = generated
	}
	in.Request.Header.Set("X-Request-ID", id)
	in.CorrelationID = id
	if in.ResponseWriter != nil {
		in.ResponseWriter.Header().Set("X-Request-ID", id)
	}
}

// CorrelationIDError indicates request-id setup failed.
type CorrelationIDError struct {
	PluginError
}

var ErrCorrelationIDInvalid = &CorrelationIDError{
	PluginError: PluginError{
		Code:    "invalid_correlation_id",
		Message: "Correlation ID is invalid",
		Status:  http.StatusBadRequest,
	},
}
