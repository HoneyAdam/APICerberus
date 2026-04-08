package admin

import (
	"net/http"
	"testing"
)

func TestListRoutesEndpoint(t *testing.T) {
	serverURL, _, _, token := newAdminTestServer(t)
 _ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/routes", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestListUpstreamsEndpoint(t *testing.T) {
	serverURL, _, _, token := newAdminTestServer(t)
 _ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/upstreams", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestDeleteUserEndpoint(t *testing.T) {
	serverURL, _, _, token := newAdminTestServer(t)
 _ = token

	// Test deleting non-existent user
	resp := mustJSONRequest(t, http.MethodDelete, serverURL+"/admin/api/v1/users/non-existent-id", token, nil)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestHandleStatusEndpoint(t *testing.T) {
	serverURL, _, _, token := newAdminTestServer(t)
 _ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/status", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestHandleInfoEndpoint(t *testing.T) {
	serverURL, _, _, token := newAdminTestServer(t)
 _ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/info", token, nil)
	assertStatus(t, resp, http.StatusOK)
}

func TestListServicesEndpoint(t *testing.T) {
	serverURL, _, _, token := newAdminTestServer(t)
 _ = token

	resp := mustJSONRequest(t, http.MethodGet, serverURL+"/admin/api/v1/services", token, nil)
	assertStatus(t, resp, http.StatusOK)
}
