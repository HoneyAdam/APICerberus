package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
)

var uuidFormatPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// RequestValidatorConfig configures basic JSON-schema validation.
type RequestValidatorConfig struct {
	Schema map[string]any
}

// RequestValidator validates request payload before proxying.
type RequestValidator struct {
	schema requestSchema
}

type requestSchema struct {
	Type       string
	Required   []string
	Properties map[string]schemaProperty
}

type schemaProperty struct {
	Type   string
	Format string
}

func NewRequestValidator(cfg RequestValidatorConfig) (*RequestValidator, error) {
	schema, err := parseRequestSchema(cfg.Schema)
	if err != nil {
		return nil, err
	}
	return &RequestValidator{schema: schema}, nil
}

func (p *RequestValidator) Name() string  { return "request-validator" }
func (p *RequestValidator) Phase() Phase  { return PhasePreProxy }
func (p *RequestValidator) Priority() int { return 30 }

func (p *RequestValidator) Validate(in *PipelineContext) error {
	if p == nil || in == nil || in.Request == nil {
		return nil
	}
	if in.Request.Body == nil {
		return nil
	}

	body, err := io.ReadAll(in.Request.Body)
	if closeErr := in.Request.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	in.Request.Body = io.NopCloser(bytes.NewReader(body))
	in.Request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	in.Request.ContentLength = int64(len(body))

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &RequestValidatorError{
			Code:    "validation_failed",
			Message: "Request body must be valid JSON object",
			Status:  http.StatusBadRequest,
			Details: []string{"body is empty"},
		}
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return &RequestValidatorError{
			Code:    "validation_failed",
			Message: "Request body must be valid JSON object",
			Status:  http.StatusBadRequest,
			Details: []string{"invalid JSON payload"},
		}
	}

	details := validatePayloadAgainstSchema(payload, p.schema)
	if len(details) > 0 {
		return &RequestValidatorError{
			Code:    "validation_failed",
			Message: "Request body validation failed",
			Status:  http.StatusBadRequest,
			Details: details,
		}
	}
	return nil
}

func parseRequestSchema(raw map[string]any) (requestSchema, error) {
	schema := requestSchema{
		Type:       "object",
		Properties: map[string]schemaProperty{},
	}
	if raw == nil {
		return schema, nil
	}

	if value, ok := raw["type"]; ok {
		schema.Type = normalizeSchemaValue(value)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	if schema.Type != "object" {
		return requestSchema{}, fmt.Errorf("request-validator supports only object schema type")
	}

	if required, ok := raw["required"]; ok {
		schema.Required = coerce.AsStringSlice(required)
	}
	if propertiesRaw, ok := raw["properties"].(map[string]any); ok {
		for field, value := range propertiesRaw {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			property := schemaProperty{}
			if item, ok := value.(map[string]any); ok {
				property.Type = normalizeSchemaValue(item["type"])
				property.Format = normalizeSchemaValue(item["format"])
			}
			schema.Properties[field] = property
		}
	}
	return schema, nil
}

func normalizeSchemaValue(value any) string {
	if value == nil {
		return ""
	}
	out := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if out == "" || out == "<nil>" {
		return ""
	}
	return out
}

func validatePayloadAgainstSchema(payload any, schema requestSchema) []string {
	objectValue, ok := payload.(map[string]any)
	if !ok {
		return []string{"payload must be a JSON object"}
	}

	var details []string
	for _, field := range schema.Required {
		if _, exists := objectValue[field]; !exists {
			details = append(details, fmt.Sprintf("required field %q is missing", field))
		}
	}

	for field, rule := range schema.Properties {
		value, exists := objectValue[field]
		if !exists {
			continue
		}
		if rule.Type != "" && !valueMatchesType(value, rule.Type) {
			details = append(details, fmt.Sprintf("field %q must be %s", field, rule.Type))
			continue
		}
		if formatErr := validateFormat(field, value, rule.Format); formatErr != "" {
			details = append(details, formatErr)
		}
	}
	return details
}

func valueMatchesType(value any, expected string) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		num, ok := value.(float64)
		if !ok {
			return false
		}
		return num == float64(int64(num))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "":
		return true
	default:
		return true
	}
}

func validateFormat(field string, value any, format string) string {
	if format == "" {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Sprintf("field %q must be a string for %s format", field, format)
	}

	switch format {
	case "email":
		if !looksLikeEmail(text) {
			return fmt.Sprintf("field %q must be a valid email", field)
		}
	case "uuid":
		if !uuidFormatPattern.MatchString(text) {
			return fmt.Sprintf("field %q must be a valid uuid", field)
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return fmt.Sprintf("field %q must be a valid RFC3339 date-time", field)
		}
	}
	return ""
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	at := strings.Index(value, "@")
	if at <= 0 || at >= len(value)-1 {
		return false
	}
	domain := value[at+1:]
	return strings.Contains(domain, ".")
}

// RequestValidatorError indicates payload validation failure.
type RequestValidatorError struct {
	Code    string
	Message string
	Status  int
	Details []string
}

func (e *RequestValidatorError) Error() string { return e.Message }
