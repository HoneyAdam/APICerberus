package plugin

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// RequestSizeLimitConfig configures max accepted request body size.
type RequestSizeLimitConfig struct {
	MaxBytes int64
}

// RequestSizeLimit validates request body size before proxying.
type RequestSizeLimit struct {
	maxBytes int64
}

func NewRequestSizeLimit(cfg RequestSizeLimitConfig) *RequestSizeLimit {
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1 << 20 // 1 MiB default
	}
	return &RequestSizeLimit{maxBytes: maxBytes}
}

func (p *RequestSizeLimit) Name() string  { return "request-size-limit" }
func (p *RequestSizeLimit) Phase() Phase  { return PhasePreProxy }
func (p *RequestSizeLimit) Priority() int { return 25 }

func (p *RequestSizeLimit) Enforce(in *PipelineContext) error {
	if p == nil || in == nil || in.Request == nil {
		return nil
	}
	req := in.Request
	limit := p.maxBytes
	if limit <= 0 {
		limit = 1 << 20
	}

	if req.ContentLength > limit {
		return &RequestSizeLimitError{
			PluginError: PluginError{
				Code:    "payload_too_large",
				Message: fmt.Sprintf("Request body exceeds %d bytes", limit),
				Status:  http.StatusRequestEntityTooLarge,
			},
		}
	}
	if req.Body == nil {
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(req.Body, limit+1))
	if closeErr := req.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return &RequestSizeLimitError{
			PluginError: PluginError{
				Code:    "payload_too_large",
				Message: fmt.Sprintf("Request body exceeds %d bytes", limit),
				Status:  http.StatusRequestEntityTooLarge,
			},
		}
	}

	req.ContentLength = int64(len(data))
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	in.Request = req
	return nil
}

// RequestSizeLimitError indicates body-size validation failure.
type RequestSizeLimitError struct {
	PluginError
}
