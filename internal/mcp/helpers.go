package mcp

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func queryFromArgs(args map[string]any, ignoreKeys ...string) url.Values {
	values := url.Values{}
	if len(args) == 0 {
		return values
	}
	ignored := make(map[string]struct{}, len(ignoreKeys))
	for _, key := range ignoreKeys {
		ignored[strings.TrimSpace(key)] = struct{}{}
	}
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := ignored[key]; ok {
			continue
		}
		appendQueryValue(values, key, value)
	}
	return values
}

func appendQueryValue(values url.Values, key string, value any) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		if strings.TrimSpace(v) == "" {
			return
		}
		values.Set(key, v)
	case []string:
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			values.Add(key, item)
		}
	case []any:
		for _, item := range v {
			text := strings.TrimSpace(asString(item))
			if text == "" {
				continue
			}
			values.Add(key, text)
		}
	default:
		values.Set(key, asString(value))
	}
}

func payloadFromArgs(args map[string]any, nestedKey string, ignoreKeys ...string) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	if nestedKey != "" {
		if raw, ok := args[nestedKey]; ok {
			if payload, ok := raw.(map[string]any); ok {
				return config.CloneAnyMap(payload)
			}
		}
	}

	ignored := make(map[string]struct{}, len(ignoreKeys))
	for _, key := range ignoreKeys {
		ignored[strings.TrimSpace(key)] = struct{}{}
	}
	if nestedKey != "" {
		ignored[strings.TrimSpace(nestedKey)] = struct{}{}
	}

	out := make(map[string]any, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := ignored[key]; ok {
			continue
		}
		out[key] = value
	}
	return out
}

func requireString(args map[string]any, key string) (string, error) {
	value := strings.TrimSpace(asString(args[key]))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requireAnyString(args map[string]any, keys ...string) (string, error) {
	for _, key := range keys {
		value := strings.TrimSpace(asString(args[key]))
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s is required", strings.Join(keys, " or "))
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
