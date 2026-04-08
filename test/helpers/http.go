// Package testhelpers provides utilities for testing APICerebrus.
package testhelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestServer wraps an httptest.Server with test utilities.
type TestServer struct {
	*httptest.Server
	t testing.TB
}

// NewTestServer creates a new test HTTP server with the given handler.
// The server is automatically closed when the test completes.
func NewTestServer(t testing.TB, handler http.Handler) *TestServer {
	t.Helper()

	server := httptest.NewServer(handler)

	ts := &TestServer{
		Server: server,
		t:      t,
	}

	t.Cleanup(func() {
		ts.Close()
	})

	return ts
}

// NewTestTLSServer creates a new test HTTPS server with the given handler.
func NewTestTLSServer(t testing.TB, handler http.Handler) *TestServer {
	t.Helper()

	server := httptest.NewTLSServer(handler)

	ts := &TestServer{
		Server: server,
		t:      t,
	}

	t.Cleanup(func() {
		ts.Close()
	})

	return ts
}

// URL returns the base URL of the test server.
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// MakeRequest makes an HTTP request to the test server.
// It returns the response for further assertions.
func (ts *TestServer) MakeRequest(method, path string, body any, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.makeRequestWithContext(context.Background(), method, path, body, headers)
}

// MakeRequestWithContext makes an HTTP request with a specific context.
func (ts *TestServer) MakeRequestWithContext(ctx context.Context, method, path string, body any, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.makeRequestWithContext(ctx, method, path, body, headers)
}

func (ts *TestServer) makeRequestWithContext(ctx context.Context, method, path string, body any, headers map[string]string) *http.Response {
	var bodyReader io.Reader

	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		case io.Reader:
			bodyReader = v
		default:
			jsonBody, err := json.Marshal(body)
			if err != nil {
				ts.t.Fatalf("Failed to marshal request body: %v", err)
			}
			bodyReader = bytes.NewReader(jsonBody)
			if headers == nil {
				headers = make(map[string]string)
			}
			if _, hasContentType := headers["Content-Type"]; !hasContentType {
				headers["Content-Type"] = "application/json"
			}
		}
	}

	url := ts.URL() + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		ts.t.Fatalf("Failed to create request: %v", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		ts.t.Fatalf("Failed to make request: %v", err)
	}

	return resp
}

// GET makes a GET request to the test server.
func (ts *TestServer) GET(path string, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.MakeRequest(http.MethodGet, path, nil, headers)
}

// POST makes a POST request to the test server.
func (ts *TestServer) POST(path string, body any, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.MakeRequest(http.MethodPost, path, body, headers)
}

// PUT makes a PUT request to the test server.
func (ts *TestServer) PUT(path string, body any, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.MakeRequest(http.MethodPut, path, body, headers)
}

// PATCH makes a PATCH request to the test server.
func (ts *TestServer) PATCH(path string, body any, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.MakeRequest(http.MethodPatch, path, body, headers)
}

// DELETE makes a DELETE request to the test server.
func (ts *TestServer) DELETE(path string, headers map[string]string) *http.Response {
	ts.t.Helper()
	return ts.MakeRequest(http.MethodDelete, path, nil, headers)
}

// AssertStatus asserts that the response has the expected status code.
func AssertStatus(t testing.TB, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Errorf("Expected status %d, got %d", expected, resp.StatusCode)
	}
}

// AssertStatusOK asserts that the response status is 200 OK.
func AssertStatusOK(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusOK)
}

// AssertStatusCreated asserts that the response status is 201 Created.
func AssertStatusCreated(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusCreated)
}

// AssertStatusNoContent asserts that the response status is 204 No Content.
func AssertStatusNoContent(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusNoContent)
}

// AssertStatusBadRequest asserts that the response status is 400 Bad Request.
func AssertStatusBadRequest(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusBadRequest)
}

// AssertStatusUnauthorized asserts that the response status is 401 Unauthorized.
func AssertStatusUnauthorized(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusUnauthorized)
}

// AssertStatusForbidden asserts that the response status is 403 Forbidden.
func AssertStatusForbidden(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusForbidden)
}

// AssertStatusNotFound asserts that the response status is 404 Not Found.
func AssertStatusNotFound(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusNotFound)
}

// AssertStatusInternalServerError asserts that the response status is 500 Internal Server Error.
func AssertStatusInternalServerError(t testing.TB, resp *http.Response) {
	t.Helper()
	AssertStatus(t, resp, http.StatusInternalServerError)
}

// AssertJSON asserts that the response body matches the expected JSON.
// The expected parameter can be a string, []byte, or a struct that will be marshaled to JSON.
func AssertJSON(t testing.TB, resp *http.Response, expected any) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	resp.Body.Close()

	var expectedJSON []byte
	switch v := expected.(type) {
	case string:
		expectedJSON = []byte(v)
	case []byte:
		expectedJSON = v
	default:
		expectedJSON, err = json.Marshal(v)
		if err != nil {
			t.Fatalf("Failed to marshal expected JSON: %v", err)
		}
	}

	var expectedMap, actualMap map[string]any
	if err := json.Unmarshal(expectedJSON, &expectedMap); err != nil {
		t.Fatalf("Failed to unmarshal expected JSON: %v", err)
	}
	if err := json.Unmarshal(body, &actualMap); err != nil {
		t.Fatalf("Failed to unmarshal actual JSON: %v\nBody: %s", err, string(body))
	}

	if !jsonEqual(expectedMap, actualMap) {
		t.Errorf("JSON mismatch:\nExpected: %s\nActual: %s", string(expectedJSON), string(body))
	}
}

// AssertJSONContains asserts that the response JSON contains the expected fields.
func AssertJSONContains(t testing.TB, resp *http.Response, expected map[string]any) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	resp.Body.Close()

	var actual map[string]any
	if err := json.Unmarshal(body, &actual); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v\nBody: %s", err, string(body))
	}

	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		if !exists {
			t.Errorf("Expected key %q not found in response", key)
			continue
		}
		if !jsonValuesEqual(expectedValue, actualValue) {
			t.Errorf("Key %q: expected %v, got %v", key, expectedValue, actualValue)
		}
	}
}

// AssertHeader asserts that the response has the expected header value.
func AssertHeader(t testing.TB, resp *http.Response, key, expected string) {
	t.Helper()
	actual := resp.Header.Get(key)
	if actual != expected {
		t.Errorf("Expected header %q to be %q, got %q", key, expected, actual)
	}
}

// AssertHeaderContains asserts that the response header contains the expected substring.
func AssertHeaderContains(t testing.TB, resp *http.Response, key, expected string) {
	t.Helper()
	actual := resp.Header.Get(key)
	if !strings.Contains(actual, expected) {
		t.Errorf("Expected header %q to contain %q, got %q", key, expected, actual)
	}
}

// AssertBodyContains asserts that the response body contains the expected substring.
func AssertBodyContains(t testing.TB, resp *http.Response, expected string) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	resp.Body.Close()

	if !strings.Contains(string(body), expected) {
		t.Errorf("Expected body to contain %q, got:\n%s", expected, string(body))
	}
}

// ReadBody reads and returns the response body as a string.
func ReadBody(t testing.TB, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	resp.Body.Close()
	return string(body)
}

// ParseJSON parses the response body as JSON into the target.
func ParseJSON(t testing.TB, resp *http.Response, target any) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	resp.Body.Close()

	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nBody: %s", err, string(body))
	}
}

// NewRequest creates a new HTTP request for testing.
func NewRequest(t testing.TB, method, url string, body any) *http.Request {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		case io.Reader:
			bodyReader = v
		default:
			jsonBody, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}
			bodyReader = bytes.NewReader(jsonBody)
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	return req
}

// WithContext returns a new request with the given context.
func WithContext(req *http.Request, ctx context.Context) *http.Request {
	return req.WithContext(ctx)
}

// WithHeader adds a header to the request.
func WithHeader(req *http.Request, key, value string) *http.Request {
	req.Header.Set(key, value)
	return req
}

// WithHeaders adds multiple headers to the request.
func WithHeaders(req *http.Request, headers map[string]string) *http.Request {
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req
}

// WithBearerAuth adds an Authorization header with Bearer token.
func WithBearerAuth(req *http.Request, token string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// WithAPIKey adds an X-API-Key header.
func WithAPIKey(req *http.Request, key string) *http.Request {
	req.Header.Set("X-API-Key", key)
	return req
}

// WithJSONContentType sets the Content-Type to application/json.
func WithJSONContentType(req *http.Request) *http.Request {
	req.Header.Set("Content-Type", "application/json")
	return req
}

// RecordResponse records an HTTP response for assertions.
type RecordResponse struct {
	*httptest.ResponseRecorder
}

// NewRecorder creates a new response recorder for testing handlers directly.
func NewRecorder() *RecordResponse {
	return &RecordResponse{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

// Result returns the recorded response.
func (rr *RecordResponse) Result() *http.Response {
	return rr.ResponseRecorder.Result()
}

// BodyString returns the response body as a string.
func (rr *RecordResponse) BodyString() string {
	return rr.ResponseRecorder.Body.String()
}

// jsonEqual compares two JSON objects for equality.
func jsonEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for key, aVal := range a {
		bVal, exists := b[key]
		if !exists {
			return false
		}
		if !jsonValuesEqual(aVal, bVal) {
			return false
		}
	}
	return true
}

// jsonValuesEqual compares two JSON values for equality.
func jsonValuesEqual(a, b any) bool {
	switch aVal := a.(type) {
	case map[string]any:
		bVal, ok := b.(map[string]any)
		if !ok {
			return false
		}
		return jsonEqual(aVal, bVal)
	case []any:
		bVal, ok := b.([]any)
		if !ok {
			return false
		}
		if len(aVal) != len(bVal) {
			return false
		}
		for i := range aVal {
			if !jsonValuesEqual(aVal[i], bVal[i]) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}

// WaitForServer waits for the server to be ready by making requests until success or timeout.
func WaitForServer(t testing.TB, url string, timeout time.Duration) {
	t.Helper()

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Server at %s did not become ready within %v", url, timeout)
}
