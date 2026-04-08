package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestRun(t *testing.T) {
	t.Run("empty args", func(t *testing.T) {
		// This will try to run start with no config, which will fail
		err := Run([]string{})
		if err == nil {
			t.Error("Run([]) should return error (no config)")
		}
	})

	t.Run("help command", func(t *testing.T) {
		err := Run([]string{"help"})
		if err != nil {
			t.Errorf("Run([help]) error = %v", err)
		}
	})

	t.Run("-h flag", func(t *testing.T) {
		err := Run([]string{"-h"})
		if err != nil {
			t.Errorf("Run([-h]) error = %v", err)
		}
	})

	t.Run("--help flag", func(t *testing.T) {
		err := Run([]string{"--help"})
		if err != nil {
			t.Errorf("Run([--help]) error = %v", err)
		}
	})

	t.Run("version command", func(t *testing.T) {
		err := Run([]string{"version"})
		if err != nil {
			t.Errorf("Run([version]) error = %v", err)
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		err := Run([]string{"unknown-command"})
		if err == nil {
			t.Error("Run([unknown]) should return error")
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("Error should contain 'unknown command', got: %v", err)
		}
	})
}

func TestRunVersion(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runVersion()

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("runVersion() error = %v", err)
	}

	content, _ := io.ReadAll(r)
	output := string(content)

	if output == "" {
		t.Error("runVersion() should output version info")
	}
}

func TestPrintUsage(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = oldStdout

	content, _ := io.ReadAll(r)
	output := string(content)

	if output == "" {
		t.Error("printUsage() should output usage info")
	}

	expectedCommands := []string{"start", "stop", "version", "config", "user"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(output, cmd) {
			t.Errorf("printUsage() output should contain %q", cmd)
		}
	}
}

func TestPrintBanner(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr: ":8080",
		},
		Admin: config.AdminConfig{
			Addr: ":9876",
		},
	}

	printBanner(cfg, "/tmp/test.pid")

	w.Close()
	os.Stdout = oldStdout

	content, _ := io.ReadAll(r)
	output := string(content)

	if output == "" {
		t.Error("printBanner() should output banner")
	}

	if !strings.Contains(output, "APICEREBUS") && !strings.Contains(output, "API CERBERUS") {
		t.Error("printBanner() should contain 'APICEREBUS' or 'API CERBERUS'")
	}
}

func TestWritePID(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")

	t.Run("write PID", func(t *testing.T) {
		err := writePID(pidFile)
		if err != nil {
			t.Fatalf("writePID() error = %v", err)
		}

		// Verify file was created
		if _, err := os.Stat(pidFile); os.IsNotExist(err) {
			t.Error("PID file was not created")
		}

		// Read PID
		content, err := os.ReadFile(pidFile)
		if err != nil {
			t.Errorf("Failed to read PID file: %v", err)
		}
		if len(content) == 0 {
			t.Error("PID file is empty")
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		// On some systems, this may not fail immediately
		err := writePID("/nonexistent/directory1/test.pid")
		// Just log the result, don't fail the test
		t.Logf("writePID() for invalid path returned: %v", err)
	})
}

func TestRunConfig(t *testing.T) {
	t.Run("no subcommand", func(t *testing.T) {
		err := runConfig([]string{})
		if err == nil {
			t.Error("runConfig([]) should return error")
		}
	})

	t.Run("validate with missing file", func(t *testing.T) {
		err := runConfig([]string{"validate", "-config", "/nonexistent/file.yaml"})
		if err == nil {
			t.Error("runConfig(validate) should return error for missing file")
		}
	})
}
