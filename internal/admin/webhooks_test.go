package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- validateWebhookURL Tests ---

func TestValidateWebhookURL_ValidHTTP(t *testing.T) {
	err := validateWebhookURL("https://example.com/webhook")
	if err != nil {
		t.Fatalf("expected valid URL to pass: %v", err)
	}
}

func TestValidateWebhookURL_ValidHTTPS(t *testing.T) {
	err := validateWebhookURL("https://api.example.com/hooks/abc-123")
	if err != nil {
		t.Fatalf("expected valid HTTPS URL to pass: %v", err)
	}
}

func TestValidateWebhookURL_ValidHTTPDev(t *testing.T) {
	err := validateWebhookURL("http://localhost:8080/webhook")
	if err != nil {
		t.Fatalf("expected HTTP (dev) URL to pass: %v", err)
	}
}

func TestValidateWebhookURL_InvalidScheme(t *testing.T) {
	err := validateWebhookURL("ftp://example.com/webhook")
	if err == nil {
		t.Fatal("expected error for ftp:// scheme")
	}
}

func TestValidateWebhookURL_NoHost(t *testing.T) {
	err := validateWebhookURL("https:///webhook")
	if err == nil {
		t.Fatal("expected error for URL with no host")
	}
}

func TestValidateWebhookURL_InvalidURL(t *testing.T) {
	err := validateWebhookURL("://not-a-url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestValidateWebhookURL_LoopbackIPv4(t *testing.T) {
	err := validateWebhookURL("https://127.0.0.1/webhook")
	if err == nil {
		t.Fatal("expected error for loopback IP")
	}
}

func TestValidateWebhookURL_LoopbackIPv6(t *testing.T) {
	err := validateWebhookURL("https://[::1]/webhook")
	if err == nil {
		t.Fatal("expected error for loopback IPv6")
	}
}

func TestValidateWebhookURL_UnspecifiedIP(t *testing.T) {
	err := validateWebhookURL("https://0.0.0.0/webhook")
	if err == nil {
		t.Fatal("expected error for unspecified IP")
	}
}

func TestValidateWebhookURL_LinkLocal(t *testing.T) {
	err := validateWebhookURL("https://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected error for link-local IP (SSRF protection)")
	}
}

func TestValidateWebhookURL_Multicast(t *testing.T) {
	err := validateWebhookURL("https://224.0.0.1/webhook")
	if err == nil {
		t.Fatal("expected error for multicast IP")
	}
}

// --- ValidateWebhookSignature Tests ---

func TestValidateWebhookSignature_Valid(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	secret := "test-secret"

	manager := &WebhookManager{}
	expectedSig := manager.signPayload(payload, secret)

	signature := "sha256=" + expectedSig
	if !ValidateWebhookSignature(payload, signature, secret) {
		t.Error("expected signature to be valid")
	}
}

func TestValidateWebhookSignature_WrongSecret(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	manager := &WebhookManager{}
	sig := manager.signPayload(payload, "correct-secret")

	if ValidateWebhookSignature(payload, "sha256="+sig, "wrong-secret") {
		t.Error("expected signature to be invalid with wrong secret")
	}
}

func TestValidateWebhookSignature_MissingPrefix(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	if ValidateWebhookSignature(payload, "abc123", "secret") {
		t.Error("expected signature without sha256= prefix to be invalid")
	}
}

func TestValidateWebhookSignature_TamperedPayload(t *testing.T) {
	secret := "secret"
	manager := &WebhookManager{}
	payload := []byte(`{"event":"test"}`)
	sig := manager.signPayload(payload, secret)

	tampered := []byte(`{"event":"hacked"}`)
	if ValidateWebhookSignature(tampered, "sha256="+sig, secret) {
		t.Error("expected signature to be invalid for tampered payload")
	}
}

// --- WebhookManager Tests ---

func TestWebhookManager_SignPayload(t *testing.T) {
	m := &WebhookManager{}
	payload := []byte(`{"test": true}`)
	secret := "my-secret"

	sig1 := m.signPayload(payload, secret)
	sig2 := m.signPayload(payload, secret)

	if sig1 != sig2 {
		t.Error("expected consistent signatures for same payload and secret")
	}
	if len(sig1) == 0 {
		t.Error("expected non-empty signature")
	}

	sig3 := m.signPayload(payload, "other-secret")
	if sig1 == sig3 {
		t.Error("expected different signature for different secret")
	}
}

func TestWebhookEvents_List(t *testing.T) {
	if len(WebhookEvents) == 0 {
		t.Error("expected webhook events to be defined")
	}

	for _, event := range WebhookEvents {
		if event.Type == "" {
			t.Error("event missing Type")
		}
		if event.Description == "" {
			t.Errorf("event %q missing Description", event.Type)
		}
		if event.Category == "" {
			t.Errorf("event %q missing Category", event.Type)
		}
	}
}

// --- HTTP Handler Integration Tests ---

func TestWebhook_CreateWebhook_HappyPath(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	payload := map[string]any{
		"name":   "test-webhook",
		"url":    "https://example.com/hook",
		"events": []string{"route.created"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusCreated)
	assertHasJSONField(t, resp, "id")
}

func TestWebhook_CreateWebhook_MissingName(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	payload := map[string]any{
		"url":    "https://example.com/hook",
		"events": []string{"route.created"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestWebhook_CreateWebhook_MissingURL(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	payload := map[string]any{
		"name":   "test-webhook",
		"events": []string{"route.created"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestWebhook_CreateWebhook_InvalidURL(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	payload := map[string]any{
		"name":   "test-webhook",
		"url":    "https://127.0.0.1/hook",
		"events": []string{"route.created"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestWebhook_CreateWebhook_MissingEvents(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	payload := map[string]any{
		"name": "test-webhook",
		"url":  "https://example.com/hook",
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestWebhook_ListWebhooks(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestWebhook_ListWebhookEvents(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks/events", token, nil)
	assertStatus(t, resp, http.StatusOK)
	assertJSONArrayLenAtLeast(t, resp, 1)
}

func TestWebhook_GetWebhook_NotFound(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks/nonexistent-id", token, nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestWebhook_DeleteWebhook_NotFound(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodDelete, serverURL+"/admin/api/v1/webhooks/nonexistent-id", token, nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestWebhook_CreateGetDelete_FullCycle(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	// Create
	payload := map[string]any{
		"id":     "wh-test-cycle",
		"name":   "cycle-webhook",
		"url":    "https://example.com/cycle",
		"events": []string{"route.created", "route.deleted"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusCreated)

	// Get
	resp = mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks/wh-test-cycle", token, nil)
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "name", "cycle-webhook")

	// Delete
	resp = mustJSONRequest(t, http.MethodDelete, serverURL+"/admin/api/v1/webhooks/wh-test-cycle", token, nil)
	assertStatus(t, resp, http.StatusNoContent)

	// Verify deleted
	resp = mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks/wh-test-cycle", token, nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestWebhook_UpdateWebhook(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	// Create first
	payload := map[string]any{
		"id":     "wh-update-test",
		"name":   "original-name",
		"url":    "https://example.com/original",
		"events": []string{"route.created"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusCreated)

	// Update
	updatePayload := map[string]any{
		"name": "updated-name",
	}
	resp = mustJSONRequest(t, http.MethodPut, serverURL+"/admin/api/v1/webhooks/wh-update-test", token, updatePayload)
	assertStatus(t, resp, http.StatusOK)
	assertJSONField(t, resp, "name", "updated-name")
}

func TestWebhook_UpdateWebhook_NotFound(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	payload := map[string]any{"name": "new-name"}
	resp := mustJSONRequest(t, http.MethodPut, serverURL+"/admin/api/v1/webhooks/nonexistent", token, payload)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestWebhook_ListDeliveries(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks/nonexistent/deliveries", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestWebhook_ListDeliveries_WithLimit(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/webhooks/nonexistent/deliveries?limit=10", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestWebhook_TestWebhook_NotFound(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks/nonexistent/test", token, nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestWebhook_RotateSecret_NotFound(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks/nonexistent/rotate-secret", token, nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestWebhook_TestWebhook_Success(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	// Create a mock server — use localhost hostname instead of 127.0.0.1
	// because the webhook URL validator rejects loopback IPs (SSRF protection)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// Replace 127.0.0.1 with localhost to pass SSRF check
	mockURL := strings.Replace(mockServer.URL, "127.0.0.1", "localhost", 1)

	// Create webhook pointing to mock server
	payload := map[string]any{
		"id":     "wh-test-success",
		"name":   "test-webhook",
		"url":    mockURL + "/hook",
		"events": []string{"route.created"},
		"secret": "test-secret",
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusCreated)

	// Test the webhook
	resp = mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks/wh-test-success/test", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestWebhook_RotateSecret_Success(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	// Create webhook
	payload := map[string]any{
		"id":     "wh-rotate-secret",
		"name":   "rotate-test",
		"url":    "https://example.com/hook",
		"events": []string{"route.created"},
	}
	resp := mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, payload)
	assertStatus(t, resp, http.StatusCreated)

	// Rotate secret
	resp = mustJSONRequest(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks/wh-rotate-secret/rotate-secret", token, nil)
	assertStatus(t, resp, http.StatusOK)
	assertHasJSONField(t, resp, "secret")
	assertJSONField(t, resp, "webhook_id", "wh-rotate-secret")
}

func TestWebhook_CreateEmptyPayload(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)
	_ = token

	statusCode, _, _ := mustRawRequestWithBody(t, http.MethodPost, serverURL+"/admin/api/v1/webhooks", token, "application/json", []byte("{}"))
	if statusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty payload, got %d", statusCode)
	}
}
