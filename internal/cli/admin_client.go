package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

type adminClient struct {
	baseURL    string
	adminKey   string
	httpClient *http.Client
}

func newAdminClient(configPath, adminURL, adminKey string) (*adminClient, error) {
	base, key, err := resolveAdminConnection(configPath, adminURL, adminKey)
	if err != nil {
		return nil, err
	}
	return &adminClient{
		baseURL:  base,
		adminKey: key,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}

func resolveAdminConnection(configPath, adminURL, adminKey string) (string, string, error) {
	path := strings.TrimSpace(configPath)
	if path == "" {
		path = "apicerberus.yaml"
	}
	rawURL := strings.TrimSpace(adminURL)
	rawKey := strings.TrimSpace(adminKey)

	// Check environment variables as fallback
	if rawURL == "" {
		rawURL = strings.TrimSpace(os.Getenv("APICERBERUS_ADMIN_URL"))
	}
	if rawKey == "" {
		rawKey = strings.TrimSpace(os.Getenv("APICERBERUS_ADMIN_KEY"))
	}

	if rawURL != "" && rawKey != "" {
		return normalizeAdminBaseURL(rawURL), rawKey, nil
	}

	cfg, err := config.Load(path)
	if err != nil {
		if rawURL != "" && rawKey != "" {
			return normalizeAdminBaseURL(rawURL), rawKey, nil
		}
		return "", "", fmt.Errorf("load config for admin connection: %w", err)
	}

	if rawURL == "" {
		rawURL = strings.TrimSpace(cfg.Admin.Addr)
	}
	if rawKey == "" {
		rawKey = strings.TrimSpace(cfg.Admin.APIKey)
	}
	if rawURL == "" {
		return "", "", errors.New("admin url is required (set --admin-url or admin.addr in config)")
	}
	if rawKey == "" {
		return "", "", errors.New("admin key is required (set --admin-key or admin.api_key in config)")
	}
	return normalizeAdminBaseURL(rawURL), rawKey, nil
}

func normalizeAdminBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "http://127.0.0.1:9876"
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return strings.TrimSuffix(raw, "/")
	}
	if strings.HasPrefix(raw, ":") {
		return "http://127.0.0.1" + raw
	}
	if !strings.Contains(raw, "://") {
		return "http://" + strings.TrimSuffix(raw, "/")
	}
	return strings.TrimSuffix(raw, "/")
}

func (c *adminClient) call(method, path string, query url.Values, payload any) (any, error) {
	if c == nil {
		return nil, errors.New("admin client is nil")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path is required")
	}
	requestURL := c.baseURL + path
	if encoded := strings.TrimSpace(query.Encode()); encoded != "" {
		requestURL += "?" + encoded
	}

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request payload: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Admin-Key", c.adminKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var decoded any
	if trimmed := bytes.TrimSpace(rawResp); len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			decoded = string(trimmed)
		}
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("admin api error (%d): %s", resp.StatusCode, extractErrorMessage(decoded))
	}
	if resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(rawResp)) == 0 {
		return map[string]any{"ok": true}, nil
	}
	return decoded, nil
}

func extractErrorMessage(payload any) string {
	msg := strings.TrimSpace(asString(payload))
	if m, ok := payload.(map[string]any); ok {
		if rawErr, ok := findFirst(m, "error"); ok {
			if errMap, ok := rawErr.(map[string]any); ok {
				code, _ := findString(errMap, "code")
				message, _ := findString(errMap, "message")
				if strings.TrimSpace(code) != "" && strings.TrimSpace(message) != "" {
					return code + ": " + message
				}
				if strings.TrimSpace(message) != "" {
					return message
				}
			}
			if strings.TrimSpace(msg) == "" {
				msg = asString(rawErr)
			}
		}
	}
	if msg == "" {
		return "unknown error"
	}
	return msg
}
