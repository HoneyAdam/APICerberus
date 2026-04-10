package audit

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Masker redacts configured header/body fields before persistence.
type Masker struct {
	headerKeys    map[string]struct{}
	bodyFieldPath [][]string
	replacement   string
}

// Default sensitive header/body fields masked when no explicit config is provided.
var (
	defaultMaskHeaders = []string{
		"Authorization", "Cookie", "X-Admin-Key", "X-API-Key",
		"X-Real-IP", "X-Forwarded-For", "Proxy-Authorization",
	}
	defaultMaskBodyFields = []string{
		"password", "secret", "token", "access_token", "refresh_token",
		"api_key", "api_secret", "private_key", "credit_card",
	}
)

func NewMasker(maskHeaders, maskBodyFields []string, replacement string) *Masker {
	// Apply defaults when no explicit config is provided (CWE-209)
	if len(maskHeaders) == 0 {
		maskHeaders = defaultMaskHeaders
	}
	if len(maskBodyFields) == 0 {
		maskBodyFields = defaultMaskBodyFields
	}

	headers := make(map[string]struct{}, len(maskHeaders))
	for _, key := range maskHeaders {
		trimmed := strings.ToLower(strings.TrimSpace(key))
		if trimmed == "" {
			continue
		}
		headers[trimmed] = struct{}{}
	}

	paths := make([][]string, 0, len(maskBodyFields))
	for _, field := range maskBodyFields {
		parts := make([]string, 0)
		for _, segment := range strings.Split(strings.TrimSpace(field), ".") {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}
			parts = append(parts, segment)
		}
		if len(parts) == 0 {
			continue
		}
		paths = append(paths, parts)
	}

	if strings.TrimSpace(replacement) == "" {
		replacement = "***REDACTED***"
	}

	return &Masker{
		headerKeys:    headers,
		bodyFieldPath: paths,
		replacement:   replacement,
	}
}

func (m *Masker) MaskHeaders(headers http.Header) map[string]any {
	return m.MaskHeadersInto(headers, nil)
}

// MaskHeadersInto writes masked headers into dst, reusing its capacity.
// If dst is nil, a new map is allocated. Returns the populated map.
func (m *Masker) MaskHeadersInto(headers http.Header, dst map[string]any) map[string]any {
	if headers == nil {
		if dst != nil {
			for k := range dst {
				delete(dst, k)
			}
			return dst
		}
		return map[string]any{}
	}
	out := dst
	if out == nil {
		out = make(map[string]any, len(headers))
	} else {
		for k := range out {
			delete(out, k)
		}
	}
	for key, values := range headers {
		copied := make([]string, len(values))
		copy(copied, values)
		if m.shouldMaskHeader(key) {
			for i := range copied {
				copied[i] = m.replacement
			}
		}
		if len(copied) == 1 {
			out[key] = copied[0]
		} else {
			items := make([]any, len(copied))
			for i := range copied {
				items[i] = copied[i]
			}
			out[key] = items
		}
	}
	return out
}

func (m *Masker) MaskBody(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	if m == nil || len(m.bodyFieldPath) == 0 {
		out := make([]byte, len(raw))
		copy(out, raw)
		return out
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		out := make([]byte, len(raw))
		copy(out, raw)
		return out
	}
	for _, path := range m.bodyFieldPath {
		maskJSONPath(payload, path, m.replacement)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		out := make([]byte, len(raw))
		copy(out, raw)
		return out
	}
	return encoded
}

func (m *Masker) shouldMaskHeader(key string) bool {
	if m == nil {
		return false
	}
	_, ok := m.headerKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

func maskJSONPath(node any, parts []string, replacement string) {
	if len(parts) == 0 || node == nil {
		return
	}
	switch value := node.(type) {
	case map[string]any:
		next, ok := value[parts[0]]
		if !ok {
			return
		}
		if len(parts) == 1 {
			value[parts[0]] = replacement
			return
		}
		maskJSONPath(next, parts[1:], replacement)
	case []any:
		for i := range value {
			maskJSONPath(value[i], parts, replacement)
		}
	}
}
