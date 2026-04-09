package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestE2EMCPStdioInitializeAndToolsList(t *testing.T) {
	t.Parallel()

	cfgPath := writeMCPTestConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/apicerberus", "mcp", "start", "--transport", "stdio", "--config", cfgPath)
	cmd.Dir = filepath.Join("..")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp process: %v", err)
	}

	enc := json.NewEncoder(stdin)
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	}); err != nil {
		t.Fatalf("send initialize: %v", err)
	}
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}); err != nil {
		t.Fatalf("send tools/list: %v", err)
	}

	dec := json.NewDecoder(stdout)
	gotInitialize := false
	gotToolsList := false

	readDeadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(readDeadline) && (!gotInitialize || !gotToolsList) {
		var response map[string]any
		if err := dec.Decode(&response); err != nil {
			t.Fatalf("decode mcp response: %v stderr=%s", err, stderr.String())
		}

		id, _ := response["id"].(float64)
		result, _ := response["result"].(map[string]any)
		switch int(id) {
		case 1:
			if result == nil {
				t.Fatalf("initialize response missing result: %#v", response)
			}
			if _, ok := result["protocolVersion"]; !ok {
				t.Fatalf("initialize response missing protocolVersion: %#v", result)
			}
			gotInitialize = true
		case 2:
			if result == nil {
				t.Fatalf("tools/list response missing result: %#v", response)
			}
			rawTools, ok := result["tools"].([]any)
			if !ok || len(rawTools) == 0 {
				t.Fatalf("tools/list response missing tools: %#v", result)
			}
			gotToolsList = true
		}
	}
	if !gotInitialize || !gotToolsList {
		t.Fatalf("did not receive expected mcp responses (initialize=%v tools/list=%v) stderr=%s", gotInitialize, gotToolsList, stderr.String())
	}

	_ = stdin.Close()
	waitErr := cmd.Wait()
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		t.Fatalf("mcp process exited with error: %v stderr=%s", waitErr, stderr.String())
	}
}

func writeMCPTestConfig(t *testing.T) string {
	t.Helper()

	content := `
gateway:
  http_addr: "127.0.0.1:0"
admin:
  api_key: "Xk9#mP$vL2@nQ8*wR5&tZ3(cY7)jF4!hK6_gH1~uE0-iO9=pA2|sD5>lN8<bM3"
  token_secret: "secret-admin-token-secret-at-least-32-chars-long"
services:
  - name: "svc-mcp"
    upstream: "up-mcp"
routes:
  - name: "route-mcp"
    service: "svc-mcp"
    paths:
      - "/mcp"
upstreams:
  - name: "up-mcp"
    targets:
      - address: "127.0.0.1:65534"
`
	path := filepath.Join(t.TempDir(), "apicerberus-mcp-test.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}
