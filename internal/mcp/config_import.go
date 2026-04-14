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

func loadConfigFromArgs(args map[string]any) (*config.Config, error) {
	if path := strings.TrimSpace(coerce.AsString(args["path"])); path != "" {
		return config.Load(path)
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
	return nil, errors.New("config import requires one of: path, yaml, or config")
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
