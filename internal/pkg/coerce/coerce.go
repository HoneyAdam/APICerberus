// Package coerce provides type-coercion helpers for converting
// untyped JSON values (any) into concrete Go types.
package coerce

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AsString converts any value to a trimmed string.
// Newlines are replaced with spaces for safe use in headers and logs.
// Returns empty string for nil input.
func AsString(value any) string {
	if value == nil {
		return ""
	}
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case fmt.Stringer:
		s = v.String()
	default:
		s = fmt.Sprint(value)
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

// AsStringPtr converts any value to a string pointer.
// Returns nil for nil input.
func AsStringPtr(value any) *string {
	if value == nil {
		return nil
	}
	s := AsString(value)
	return &s
}

// AsInt converts any numeric value or numeric string to int.
func AsInt(value any, fallback int) int {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// AsInt64 converts any numeric value or numeric string to int64.
func AsInt64(value any, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

// AsBool converts any value to bool, recognizing "1", "true", "yes", "on".
func AsBool(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		if s == "" {
			return fallback
		}
		return s == "1" || s == "true" || s == "yes" || s == "on"
	}
	return fallback
}

// AsFloat64 converts any numeric value or numeric string to float64.
func AsFloat64(value any, fallback float64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

// AsFloat converts any numeric value or numeric string to float64 with a fallback.
func AsFloat(value any, fallback float64) float64 {
	if n, ok := AsFloat64(value, 0); ok {
		return n
	}
	return fallback
}

// AsStringSlice converts a value to a string slice.
// Accepts []string, []any (string elements only), or comma-separated string.
func AsStringSlice(value any) []string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	case string:
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				result = append(result, t)
			}
		}
		return result
	}
	return nil
}

// AsIntSlice converts a value to an int slice.
// Accepts []int, []any (numeric elements only), or comma-separated string.
func AsIntSlice(value any, fallback []int) []int {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case []int:
		return v
	case []any:
		result := make([]int, 0, len(v))
		for _, item := range v {
			if n := AsInt(item, -1); n >= 0 {
				result = append(result, n)
			}
		}
		return result
	}
	return fallback
}

// AsAnyMap converts a value to map[string]any.
// Keys are trimmed and empty keys are skipped.
// Returns an empty map on failure (never nil).
func AsAnyMap(value any) map[string]any {
	if value == nil {
		return make(map[string]any)
	}
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			result[key] = val
		}
		return result
	case map[any]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			if key, ok := k.(string); ok {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				result[key] = val
			}
		}
		return result
	default:
		return make(map[string]any)
	}
}

// AsStringMap converts map[string]any to map[string]string.
// Skips non-string values and empty keys/values.
func AsStringMap(value any) map[string]string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]string, len(v))
		for k, val := range v {
			if s := AsString(val); s != "" && strings.TrimSpace(k) != "" {
				result[strings.TrimSpace(k)] = s
			}
		}
		return result
	case map[string]string:
		return v
	}
	return nil
}

// AsDuration converts a value to time.Duration.
// Accepts time.Duration, numeric (interpreted as seconds), or string (ParseDuration or numeric seconds).
func AsDuration(value any, fallback time.Duration) time.Duration {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case time.Duration:
		return v
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v * float64(time.Second))
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return fallback
}

// Get extracts a value from a map by key with fallback keys.
// Useful for case-insensitive or alternative key lookups.
func Get(m map[string]any, key string, fallbackKeys ...string) any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		return v
	}
	for _, k := range fallbackKeys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// GetString is shorthand for Get + AsString.
func GetString(m map[string]any, key string) string {
	return AsString(Get(m, key))
}

// GetInt is shorthand for Get + AsInt.
func GetInt(m map[string]any, key string, fallback int) int {
	return AsInt(Get(m, key), fallback)
}

// GetBool is shorthand for Get + AsBool.
func GetBool(m map[string]any, key string, fallback bool) bool {
	return AsBool(Get(m, key), fallback)
}

// GetStringSlice is shorthand for Get + AsStringSlice.
func GetStringSlice(m map[string]any, key string) []string {
	return AsStringSlice(Get(m, key))
}
