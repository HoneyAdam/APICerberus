package portal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestPortalAuthSessionFlow(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-user@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-user@example.com",
		"password": "portal-pass",
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200 got %d body=%s", loginResp.StatusCode, string(loginResp.Body))
	}
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected session cookie %q to be set", cfg.Portal.Session.CookieName)
	}

	meResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/auth/me", []*http.Cookie{sessionCookie}, nil)
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("expected me 200 got %d body=%s", meResp.StatusCode, string(meResp.Body))
	}

	logoutResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/logout", []*http.Cookie{sessionCookie}, map[string]any{})
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("expected logout 200 got %d body=%s", logoutResp.StatusCode, string(logoutResp.Body))
	}

	postLogoutMe := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/auth/me", []*http.Cookie{sessionCookie}, nil)
	if postLogoutMe.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected me after logout to return 401 got %d body=%s", postLogoutMe.StatusCode, string(postLogoutMe.Body))
	}
}

func TestPortalLoginRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-invalid@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-invalid@example.com",
		"password": "wrong",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected login 401 got %d body=%s", resp.StatusCode, string(resp.Body))
	}
}

func TestPortalEndpointSuite(t *testing.T) {
	t.Parallel()

	gatewayStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"path":      r.URL.Path,
			"method":    r.Method,
			"api_key":   r.Header.Get("X-API-Key"),
			"raw_query": r.URL.RawQuery,
		})
	}))
	defer gatewayStub.Close()

	cfg, st := openPortalTestStoreWithGateway(t, strings.TrimPrefix(gatewayStub.URL, "http://"))
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "portal-suite@example.com", "portal-pass")
	if err := st.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-users",
		Methods: []string{"GET", "POST"},
		Allowed: true,
	}); err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			UserID:      user.ID,
			RouteID:     "route-users",
			RouteName:   "Users Route",
			ServiceName: "users-service",
			Method:      "GET",
			Path:        "/api/users",
			StatusCode:  200,
			LatencyMS:   15,
			ClientIP:    "127.0.0.1",
			CreatedAt:   time.Now().UTC().Add(-time.Minute),
		},
		{
			UserID:      user.ID,
			RouteID:     "route-users",
			RouteName:   "Users Route",
			ServiceName: "users-service",
			Method:      "POST",
			Path:        "/api/users",
			StatusCode:  500,
			LatencyMS:   55,
			ClientIP:    "127.0.0.1",
			CreatedAt:   time.Now().UTC().Add(-30 * time.Second),
		},
	}); err != nil {
		t.Fatalf("seed audit entries: %v", err)
	}
	if err := st.Credits().Create(&store.CreditTransaction{
		UserID:        user.ID,
		Type:          "consume",
		Amount:        -5,
		BalanceBefore: 100,
		BalanceAfter:  95,
		Description:   "seed usage",
		CreatedAt:     time.Now().UTC().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("seed credit transaction: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-suite@example.com",
		"password": "portal-pass",
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200 got %d body=%s", loginResp.StatusCode, string(loginResp.Body))
	}
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected session cookie %q to be set", cfg.Portal.Session.CookieName)
	}

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	createKeyResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie}, map[string]any{
		"name": "suite-key",
		"mode": "test",
	})
	assertPortalStatus(t, createKeyResp, http.StatusCreated)
	createdToken := getNestedString(t, createKeyResp.Body, "token")
	createdKeyID := getNestedString(t, createKeyResp.Body, "key.id")
	if createdToken == "" || createdKeyID == "" {
		t.Fatalf("expected created key token and id in response: %s", string(createKeyResp.Body))
	}
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/api-keys/"+createdKeyID, []*http.Cookie{sessionCookie}, map[string]any{
		"name": "suite-key-renamed",
	}), http.StatusOK)

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/apis", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/apis/route-users", []*http.Cookie{sessionCookie}, nil), http.StatusOK)

	playgroundResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/playground/send", []*http.Cookie{sessionCookie}, map[string]any{
		"method":  "POST",
		"path":    "/echo",
		"api_key": createdToken,
		"body":    `{"hello":"world"}`,
	})
	assertPortalStatus(t, playgroundResp, http.StatusOK)

	saveTemplateResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/playground/templates", []*http.Cookie{sessionCookie}, map[string]any{
		"name":   "Suite Template",
		"method": "GET",
		"path":   "/v1/example",
		"headers": map[string]string{
			"X-Test": "1",
		},
		"query": map[string]string{
			"trace": "true",
		},
	})
	assertPortalStatus(t, saveTemplateResp, http.StatusCreated)
	templateID := getNestedString(t, saveTemplateResp.Body, "id")
	if templateID == "" {
		t.Fatalf("expected created template id: %s", string(saveTemplateResp.Body))
	}
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/playground/templates", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/playground/templates/"+templateID, []*http.Cookie{sessionCookie}, nil), http.StatusNoContent)

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/overview?window=2h", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/timeseries?window=2h&granularity=30m", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/top-endpoints?window=2h", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/errors?window=2h", []*http.Cookie{sessionCookie}, nil), http.StatusOK)

	logsResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs?limit=10", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, logsResp, http.StatusOK)
	logID := firstLogIDFromPortalBody(t, logsResp.Body)
	if logID == "" {
		t.Fatalf("expected at least one log entry in portal logs response: %s", string(logsResp.Body))
	}
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/"+logID, []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/export?format=json", []*http.Cookie{sessionCookie}, nil), http.StatusOK)

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/balance", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/transactions?limit=20", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/forecast", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/credits/purchase", []*http.Cookie{sessionCookie}, map[string]any{
		"amount":      7,
		"description": "suite purchase",
	}), http.StatusOK)

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/security/ip-whitelist", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/security/ip-whitelist", []*http.Cookie{sessionCookie}, map[string]any{
		"ip": "203.0.113.10",
	}), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/security/ip-whitelist/203.0.113.10", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/security/activity", []*http.Cookie{sessionCookie}, nil), http.StatusOK)

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/settings/profile", []*http.Cookie{sessionCookie}, nil), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/settings/profile", []*http.Cookie{sessionCookie}, map[string]any{
		"name":    "Portal Suite User",
		"company": "Cerberus Labs",
	}), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/settings/notifications", []*http.Cookie{sessionCookie}, map[string]any{
		"notifications": map[string]any{
			"email": true,
		},
	}), http.StatusOK)

	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/auth/password", []*http.Cookie{sessionCookie}, map[string]any{
		"old_password": "portal-pass",
		"new_password": "portal-pass-new",
	}), http.StatusOK)
	assertPortalStatus(t, mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/logout", []*http.Cookie{sessionCookie}, map[string]any{}), http.StatusOK)
}

type portalResponse struct {
	StatusCode int
	Body       []byte
	Cookies    []*http.Cookie
}

func mustPortalJSONRequest(t *testing.T, client *http.Client, method, rawURL string, cookies []*http.Cookie, payload any) portalResponse {
	t.Helper()

	var bodyReader *bytes.Reader
	if payload == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("json marshal request: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return portalResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Cookies:    resp.Cookies(),
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func openPortalTestStore(t *testing.T) (*config.Config, *store.Store) {
	return openPortalTestStoreWithGateway(t, "127.0.0.1:18080")
}

func openPortalTestStoreWithGateway(t *testing.T, gatewayAddr string) (*config.Config, *store.Store) {
	t.Helper()

	cfg := &config.Config{
		Store: config.StoreConfig{
			Path:        filepath.Join(t.TempDir(), "portal-auth.db"),
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Portal: config.PortalConfig{
			Enabled:    true,
			Addr:       "127.0.0.1:0",
			PathPrefix: "/portal",
			Session: config.PortalSessionConfig{
				CookieName: "portal_test_session",
				MaxAge:     2 * time.Hour,
				Secure:     false,
			},
		},
		Gateway: config.GatewayConfig{
			HTTPAddr: gatewayAddr,
		},
		Services: []config.Service{
			{
				ID:       "svc-users",
				Name:     "users-service",
				Protocol: "http",
				Upstream: "users-upstream",
			},
		},
		Routes: []config.Route{
			{
				ID:      "route-users",
				Name:    "Users Route",
				Service: "svc-users",
				Paths:   []string{"/api/users"},
				Methods: []string{"GET", "POST"},
			},
		},
		Billing: config.BillingConfig{
			Enabled:     true,
			DefaultCost: 2,
			RouteCosts: map[string]int64{
				"route-users": 3,
			},
		},
	}
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return cfg, st
}

func createPortalTestUser(t *testing.T, st *store.Store, email, password string) {
	t.Helper()
	_ = createPortalTestUserWithID(t, st, email, password)
}

func createPortalTestUserWithID(t *testing.T, st *store.Store, email, password string) *store.User {
	t.Helper()
	hash, err := store.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	user := &store.User{
		Email:        email,
		Name:         "Portal User",
		PasswordHash: hash,
		Role:         "user",
		Status:       "active",
	}
	if err := st.Users().Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func assertPortalStatus(t *testing.T, resp portalResponse, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("expected status %d got %d body=%s", expected, resp.StatusCode, string(resp.Body))
	}
}

func getNestedString(t *testing.T, raw []byte, path string) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v body=%s", err, string(raw))
	}
	current := any(payload)
	parts := strings.Split(path, ".")
	for _, part := range parts {
		record, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = record[part]
		if !ok {
			return ""
		}
	}
	value, _ := current.(string)
	return value
}

func firstLogIDFromPortalBody(t *testing.T, raw []byte) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal logs payload: %v body=%s", err, string(raw))
	}
	items, ok := payload["entries"].([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := first["id"].(string)
	return id
}
