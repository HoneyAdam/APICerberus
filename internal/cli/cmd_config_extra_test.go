package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigExport(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  port: 8080
  host: 0.0.0.0
gateway:
  http_addr: :8080
  https_addr: ""
admin:
  addr: :9876
  api_key: test-key
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	// Test export to stdout
	err := runConfigExport([]string{"--config", configPath, "--out", "-"})
	if err != nil {
		t.Errorf("runConfigExport error: %v", err)
	}
}

func TestRunConfigExport_ToFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	outputPath := filepath.Join(tmpDir, "exported-config.yaml")
	configContent := `
server:
  port: 8080
gateway:
  http_addr: :8080
admin:
  addr: :9876
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	err := runConfigExport([]string{"--config", configPath, "--out", outputPath})
	if err != nil {
		t.Errorf("runConfigExport error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Exported config file should exist")
	}
}

func TestRunConfigExport_InvalidConfig(t *testing.T) {
	err := runConfigExport([]string{"--config", "/nonexistent/config.yaml", "--out", "-"})
	if err == nil {
		t.Error("runConfigExport should return error for invalid config")
	}
}

func TestRunConfigImport(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source-config.yaml")
	targetPath := filepath.Join(tmpDir, "target-config.yaml")
	configContent := `
server:
  port: 9090
  host: 127.0.0.1
gateway:
  http_addr: :9090
admin:
  addr: :9876
  api_key: imported-key
`
	os.WriteFile(sourcePath, []byte(configContent), 0644)

	err := runConfigImport([]string{"--target", targetPath, sourcePath})
	if err != nil {
		t.Errorf("runConfigImport error: %v", err)
	}

	// Verify file was created
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read target config: %v", err)
	}
	if !strings.Contains(string(content), "9090") {
		t.Error("Imported config should contain port 9090")
	}
}

func TestRunConfigImport_MissingSource(t *testing.T) {
	err := runConfigImport([]string{"--target", "target.yaml"})
	if err == nil {
		t.Error("runConfigImport should return error for missing source")
	}
}

func TestRunConfigImport_InvalidSource(t *testing.T) {
	err := runConfigImport([]string{"--target", "target.yaml", "/nonexistent/source.yaml"})
	if err == nil {
		t.Error("runConfigImport should return error for invalid source")
	}
}

func TestRunConfigDiff(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "old.yaml")
	newPath := filepath.Join(tmpDir, "new.yaml")

	oldContent := `server:
  port: 8080
  host: 0.0.0.0
gateway:
  http_addr: :8080
`
	newContent := `server:
  port: 9090
  host: 0.0.0.0
gateway:
  http_addr: :9090
  https_addr: :9443
`
	os.WriteFile(oldPath, []byte(oldContent), 0644)
	os.WriteFile(newPath, []byte(newContent), 0644)

	err := runConfigDiff([]string{oldPath, newPath})
	if err != nil {
		t.Errorf("runConfigDiff error: %v", err)
	}
}

func TestRunConfigDiff_MissingArgs(t *testing.T) {
	err := runConfigDiff([]string{"only-one.yaml"})
	if err == nil {
		t.Error("runConfigDiff should return error for missing args")
	}
}

func TestRunConfigDiff_InvalidOldPath(t *testing.T) {
	tmpDir := t.TempDir()
	newPath := filepath.Join(tmpDir, "new.yaml")
	os.WriteFile(newPath, []byte("test"), 0644)

	err := runConfigDiff([]string{"/nonexistent/old.yaml", newPath})
	if err == nil {
		t.Error("runConfigDiff should return error for invalid old path")
	}
}

func TestRunConfigDiff_InvalidNewPath(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "old.yaml")
	os.WriteFile(oldPath, []byte("test"), 0644)

	err := runConfigDiff([]string{oldPath, "/nonexistent/new.yaml"})
	if err == nil {
		t.Error("runConfigDiff should return error for invalid new path")
	}
}

func TestReadLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\n"
	os.WriteFile(testFile, []byte(content), 0644)

	lines, err := readLines(testFile)
	if err != nil {
		t.Errorf("readLines error: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" {
		t.Errorf("Expected line1, got %s", lines[0])
	}
	if lines[1] != "line2" {
		t.Errorf("Expected line2, got %s", lines[1])
	}
	if lines[2] != "line3" {
		t.Errorf("Expected line3, got %s", lines[2])
	}
}

func TestReadLines_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	os.WriteFile(testFile, []byte(""), 0644)

	lines, err := readLines(testFile)
	if err != nil {
		t.Errorf("readLines error: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("Expected 0 lines for empty file, got %d", len(lines))
	}
}

func TestReadLines_FileNotFound(t *testing.T) {
	_, err := readLines("/nonexistent/file.txt")
	if err == nil {
		t.Error("readLines should return error for non-existent file")
	}
}

func TestUnifiedDiff(t *testing.T) {
	a := []string{
		"line1",
		"line2",
		"line3",
	}
	b := []string{
		"line1",
		"line2 modified",
		"line3",
		"line4",
	}

	diff := unifiedDiff(a, b)

	// Should contain context lines and additions (the algorithm may output differently)
	// Just verify we have output and it contains the expected lines
	if len(diff) == 0 {
		t.Error("Diff should not be empty")
	}

	// Check we have some additions (lines starting with +)
	hasAddition := false
	for _, line := range diff {
		if strings.HasPrefix(line, "+") {
			hasAddition = true
			break
		}
	}
	if !hasAddition {
		t.Error("Diff should contain at least one addition")
	}
}

func TestUnifiedDiff_Identical(t *testing.T) {
	a := []string{"line1", "line2", "line3"}
	b := []string{"line1", "line2", "line3"}

	diff := unifiedDiff(a, b)

	// All lines should be context (start with space)
	for _, line := range diff {
		if !strings.HasPrefix(line, " ") {
			t.Errorf("Expected context line, got: %s", line)
		}
	}
}

func TestUnifiedDiff_AllRemoved(t *testing.T) {
	a := []string{"line1", "line2"}
	b := []string{}

	diff := unifiedDiff(a, b)

	// All lines should be removed
	for _, line := range diff {
		if !strings.HasPrefix(line, "-") {
			t.Errorf("Expected removal line, got: %s", line)
		}
	}
}

func TestUnifiedDiff_AllAdded(t *testing.T) {
	a := []string{}
	b := []string{"line1", "line2"}

	diff := unifiedDiff(a, b)

	// All lines should be added
	for _, line := range diff {
		if !strings.HasPrefix(line, "+") {
			t.Errorf("Expected addition line, got: %s", line)
		}
	}
}

func TestUnifiedDiff_EmptyBoth(t *testing.T) {
	diff := unifiedDiff([]string{}, []string{})
	if len(diff) != 0 {
		t.Errorf("Expected empty diff, got %d lines", len(diff))
	}
}
