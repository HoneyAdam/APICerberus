package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunDB_NoSubcommand(t *testing.T) {
	err := runDB([]string{})
	if err == nil {
		t.Fatal("expected error for missing db subcommand")
	}
}

func TestRunDB_UnknownSubcommand(t *testing.T) {
	err := runDB([]string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown db subcommand")
	}
}

func TestRunDBMigrate_NoSubcommand(t *testing.T) {
	err := runDBMigrate([]string{})
	if err == nil {
		t.Fatal("expected error for missing migrate subcommand")
	}
}

func TestRunDBMigrate_UnknownSubcommand(t *testing.T) {
	err := runDBMigrate([]string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown migrate subcommand")
	}
}

func TestRunDBMigrateStatus_InvalidConfig(t *testing.T) {
	err := runDBMigrateStatus([]string{"--config", "/nonexistent/config.yaml"})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestRunDBMigrateApply_InvalidConfig(t *testing.T) {
	err := runDBMigrateApply([]string{"--config", "/nonexistent/config.yaml"})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestRunDBMigrateStatus_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	configContent := `
gateway:
  http_addr: :8080
admin:
  addr: :9876
  api_key: test-admin-api-key-at-least-32-chars
  token_secret: test-admin-token-secret-at-least-32-chars
store:
  path: ":memory:"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := runDBMigrateStatus([]string{"--config", configPath})
	if err != nil {
		t.Errorf("runDBMigrateStatus error: %v", err)
	}
}

func TestRunDBMigrateApply_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	configContent := `
gateway:
  http_addr: :8080
admin:
  addr: :9876
  api_key: test-admin-api-key-at-least-32-chars
  token_secret: test-admin-token-secret-at-least-32-chars
store:
  path: ":memory:"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := runDBMigrateApply([]string{"--config", configPath})
	if err != nil {
		t.Errorf("runDBMigrateApply error: %v", err)
	}
}
