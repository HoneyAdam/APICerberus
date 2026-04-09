package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/cli"
)

func TestE2ECLISmokeCommands(t *testing.T) {
	t.Parallel()

	adminStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin/api/v1/audit-logs/export") {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"id":"audit-1"}` + "\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
		})
	}))
	defer adminStub.Close()

	cfgPath := writeCLISmokeConfig(t, adminStub.URL)
	cfgCopyPath := writeCLISmokeConfig(t, adminStub.URL)
	retentionCfgPath := writeCLISmokeConfig(t, adminStub.URL)
	exportPath := filepath.Join(t.TempDir(), "exported.yaml")
	importTarget := filepath.Join(t.TempDir(), "imported.yaml")

	commands := [][]string{
		{"version"},
		{"config", "validate", cfgPath},
		{"config", "export", "--config", cfgPath, "--out", exportPath},
		{"config", "diff", cfgPath, cfgCopyPath},
		{"config", "import", "--target", importTarget, cfgCopyPath},

		{"user", "list", "--config", cfgPath, "--output", "json"},
		{"user", "create", "--config", cfgPath, "--output", "json", "--email", "u@example.com", "--name", "User"},
		{"user", "get", "--config", cfgPath, "--output", "json", "--id", "u1"},
		{"user", "update", "--config", cfgPath, "--output", "json", "--id", "u1", "--name", "User Updated"},
		{"user", "suspend", "--config", cfgPath, "--output", "json", "--id", "u1"},
		{"user", "activate", "--config", cfgPath, "--output", "json", "--id", "u1"},
		{"user", "apikey", "list", "--config", cfgPath, "--output", "json", "--user", "u1"},
		{"user", "apikey", "create", "--config", cfgPath, "--output", "json", "--user", "u1", "--name", "cli-key"},
		{"user", "apikey", "revoke", "--config", cfgPath, "--output", "json", "--user", "u1", "--key", "k1"},
		{"user", "permission", "list", "--config", cfgPath, "--output", "json", "--user", "u1"},
		{"user", "permission", "grant", "--config", cfgPath, "--output", "json", "--user", "u1", "--route", "route-smoke", "--methods", "GET"},
		{"user", "permission", "revoke", "--config", cfgPath, "--output", "json", "--user", "u1", "--permission", "p1"},
		{"user", "ip", "list", "--config", cfgPath, "--output", "json", "--user", "u1"},
		{"user", "ip", "add", "--config", cfgPath, "--output", "json", "--user", "u1", "--ip", "203.0.113.10"},
		{"user", "ip", "remove", "--config", cfgPath, "--output", "json", "--user", "u1", "--ip", "203.0.113.10"},

		{"credit", "overview", "--config", cfgPath, "--output", "json"},
		{"credit", "balance", "--config", cfgPath, "--output", "json", "--user", "u1"},
		{"credit", "topup", "--config", cfgPath, "--output", "json", "--user", "u1", "--amount", "10", "--reason", "smoke"},
		{"credit", "deduct", "--config", cfgPath, "--output", "json", "--user", "u1", "--amount", "5", "--reason", "smoke"},
		{"credit", "transactions", "--config", cfgPath, "--output", "json", "--user", "u1"},

		{"audit", "search", "--config", cfgPath, "--output", "json"},
		{"audit", "detail", "--config", cfgPath, "--output", "json", "a1"},
		{"audit", "stats", "--config", cfgPath, "--output", "json"},
		{"audit", "export", "--config", cfgPath, "--output", "json", "--format", "jsonl"},
		{"audit", "cleanup", "--config", cfgPath, "--output", "json", "--older-than-days", "1"},

		{"analytics", "overview", "--config", cfgPath, "--output", "json"},
		{"analytics", "requests", "--config", cfgPath, "--output", "json"},
		{"analytics", "latency", "--config", cfgPath, "--output", "json"},

		{"service", "list", "--config", cfgPath, "--output", "json"},
		{"service", "add", "--config", cfgPath, "--output", "json", "--body", `{"id":"svc2","name":"svc2","protocol":"http","upstream":"up-smoke"}`},
		{"service", "get", "--config", cfgPath, "--output", "json", "svc2"},
		{"service", "update", "--config", cfgPath, "--output", "json", "--body", `{"id":"svc2","name":"svc2","protocol":"http","upstream":"up-smoke"}`, "svc2"},
		{"service", "delete", "--config", cfgPath, "--output", "json", "svc2"},

		{"route", "list", "--config", cfgPath, "--output", "json"},
		{"route", "add", "--config", cfgPath, "--output", "json", "--body", `{"id":"route2","name":"route2","service":"svc-smoke","paths":["/r2"],"methods":["GET"]}`},
		{"route", "get", "--config", cfgPath, "--output", "json", "route2"},
		{"route", "update", "--config", cfgPath, "--output", "json", "--body", `{"id":"route2","name":"route2","service":"svc-smoke","paths":["/r2"],"methods":["GET"]}`, "route2"},
		{"route", "delete", "--config", cfgPath, "--output", "json", "route2"},

		{"upstream", "list", "--config", cfgPath, "--output", "json"},
		{"upstream", "add", "--config", cfgPath, "--output", "json", "--body", `{"id":"up2","name":"up2","algorithm":"round_robin","targets":[{"id":"t1","address":"127.0.0.1:9000","weight":1}]}`},
		{"upstream", "get", "--config", cfgPath, "--output", "json", "up2"},
		{"upstream", "update", "--config", cfgPath, "--output", "json", "--body", `{"id":"up2","name":"up2","algorithm":"round_robin","targets":[{"id":"t1","address":"127.0.0.1:9000","weight":1}]}`, "up2"},
		{"upstream", "delete", "--config", cfgPath, "--output", "json", "up2"},

		{"audit", "retention", "show", "--config", retentionCfgPath},
		{"audit", "retention", "set", "--config", retentionCfgPath, "--days", "45"},
	}

	for _, args := range commands {
		if err := cli.Run(args); err != nil {
			t.Fatalf("cli command failed (%s): %v", strings.Join(args, " "), err)
		}
	}
}

func writeCLISmokeConfig(t *testing.T, adminURL string) string {
	t.Helper()

	content := fmt.Sprintf(`
gateway:
  http_addr: "127.0.0.1:0"
admin:
  addr: "%s"
  api_key: "Xk9#mP$vL2@nQ8*wR5&tZ3(cY7)jF4!hK6_gH1~uE0-iO9=pA2|sD5>lN8<bM3"
  token_secret: "secret-admin-token-secret-at-least-32-chars-long"
services:
  - name: "svc-smoke"
    upstream: "up-smoke"
routes:
  - name: "route-smoke"
    service: "svc-smoke"
    paths:
      - "/smoke"
upstreams:
  - name: "up-smoke"
    targets:
      - address: "127.0.0.1:9000"
`, adminURL)

	path := filepath.Join(t.TempDir(), "apicerberus-cli-smoke.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write cli smoke config: %v", err)
	}
	return path
}
