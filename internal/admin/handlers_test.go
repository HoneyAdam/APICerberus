package admin

import (
	"net/http"
	"testing"
)

func TestListRoutesEndpoint(t *testing.T) {
	serverURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/routes", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestListUpstreamsEndpoint(t *testing.T) {
	serverURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/upstreams", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestDeleteUserEndpoint(t *testing.T) {
	serverURL, _, _ := newAdminTestServer(t)

	// Test deleting non-existent user
	resp := mustJSONRequest(t, http.MethodDelete, serverURL+"/admin/api/v1/users/non-existent-id", "secret-admin", nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestHandleStatusEndpoint(t *testing.T) {
	serverURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/status", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestHandleInfoEndpoint(t *testing.T) {
	serverURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/info", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestListServicesEndpoint(t *testing.T) {
	serverURL, _, _ := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/services", "secret-admin", nil)
	assertStatus(t, resp, http.StatusOK)
}
