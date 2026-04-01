package gateway

import (
	"context"
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
)

type consumerContextKey struct{}

// ConsumerFromRequest returns the resolved consumer set by gateway request pipeline.
func ConsumerFromRequest(req *http.Request) *config.Consumer {
	if req == nil {
		return nil
	}
	raw := req.Context().Value(consumerContextKey{})
	if raw == nil {
		return nil
	}
	consumer, ok := raw.(*config.Consumer)
	if !ok {
		return nil
	}
	return consumer
}

func setRequestConsumer(req *http.Request, consumer *config.Consumer) {
	if req == nil || consumer == nil {
		return
	}
	ctx := context.WithValue(req.Context(), consumerContextKey{}, consumer)
	*req = *req.WithContext(ctx)
}

func extractAPIKey(req *http.Request) string {
	if req == nil {
		return ""
	}

	if value := strings.TrimSpace(req.Header.Get("X-API-Key")); value != "" {
		return value
	}

	if auth := strings.TrimSpace(req.Header.Get("Authorization")); auth != "" {
		if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
			return strings.TrimSpace(auth[7:])
		}
	}

	if value := strings.TrimSpace(req.URL.Query().Get("apikey")); value != "" {
		return value
	}
	if value := strings.TrimSpace(req.URL.Query().Get("api_key")); value != "" {
		return value
	}

	if cookie, err := req.Cookie("apikey"); err == nil {
		if value := strings.TrimSpace(cookie.Value); value != "" {
			return value
		}
	}
	return ""
}
