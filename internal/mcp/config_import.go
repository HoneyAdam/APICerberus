package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/coerce"
)

// loadConfigFromArgs parses a config payload supplied via the MCP
// `system.config.import` tool.
//
// SEC-GQL-010: the prior implementation accepted a `path` argument and called
// config.Load(path) directly, giving any caller who held the admin key (or
// could reach the in-process stdio runtime) an arbitrary-file-read primitive
// against the gateway host. Error messages from config.Load embed file
// contents on parse failure — so `path: "/etc/shadow"` turned into a file-
// content oracle. The `path` branch has been removed entirely; operators
// that want file-based imports must read the file out-of-band and pass the
// bytes via the `yaml` argument.
func loadConfigFromArgs(args map[string]any) (*config.Config, error) {
	if path := strings.TrimSpace(coerce.AsString(args["path"])); path != "" {
		return nil, errors.New("config import: 'path' is no longer accepted (SEC-GQL-010); " +
			"pass inline config via 'yaml' or 'config' instead")
	}
	if rawYAML := strings.TrimSpace(coerce.AsString(args["yaml"])); rawYAML != "" {
		return loadConfigFromYAML(rawYAML)
	}
	if rawConfig, ok := args["config"]; ok && rawConfig != nil {
		encoded, err := json.Marshal(rawConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal config payload: %w", err)
		}
		var cfg config.Config
		if err := json.Unmarshal(encoded, &cfg); err != nil {
			return nil, fmt.Errorf("decode config payload: %w", err)
		}
		return &cfg, nil
	}
	return nil, errors.New("config import requires one of: yaml, config")
}

func loadConfigFromYAML(raw string) (*config.Config, error) {
	cfg, err := loadConfigFromYAMLRaw(raw)
	if err == nil {
		return cfg, nil
	}

	normalized := normalizeYAMLForConfigParser(raw)
	if normalized == raw {
		return nil, err
	}
	cfg, normalizedErr := loadConfigFromYAMLRaw(normalized)
	if normalizedErr != nil {
		return nil, err
	}
	return cfg, nil
}

func loadConfigFromYAMLRaw(raw string) (*config.Config, error) {
	temp, err := os.CreateTemp("", "apicerberus-mcp-import-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create temp config file: %w", err)
	}
	path := temp.Name()
	_ = temp.Close()
	defer os.Remove(path)

	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		return nil, fmt.Errorf("write temp config file: %w", err)
	}
	return config.Load(path)
}

func normalizeYAMLForConfigParser(raw string) string {
	out := raw
	out = strings.ReplaceAll(out, ": {}", ":")
	out = strings.ReplaceAll(out, ": []", ":")
	return out
}
