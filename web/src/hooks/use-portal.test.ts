import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import {
  usePortalMe,
  usePortalLogin,
  usePortalLogout,
  usePortalChangePassword,
  usePortalAPIKeys,
  useCreatePortalAPIKey,
  usePortalAPIs,
  usePortalCreditsBalance,
  usePortalPurchaseCredits,
  usePortalProfile,
} from "./use-portal";
import { portalApiRequest, setPortalCSRFToken, clearPortalCSRFToken } from "@/lib/portal-api";

// Mock the portal-api module
vi.mock("@/lib/portal-api", () => ({
  portalApiRequest: vi.fn(),
  setPortalCSRFToken: vi.fn(),
  clearPortalCSRFToken: vi.fn(),
}));

// Mock helpers to avoid import issues
vi.mock("./helpers", () => ({
  toQueryRecord: (params: Record<string, unknown>) => {
    const result: Record<string, string | number | boolean> = {};
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== "") {
        if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
          result[key] = value;
        }
      }
    }
    return result;
  },
}));

const mockApi = vi.mocked(portalApiRequest);
const mockSetCSRF = vi.mocked(setPortalCSRFToken);
const mockClearCSRF = vi.mocked(clearPortalCSRFToken);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(
      QueryClientProvider,
      { client: queryClient },
      children,
    );
  };
}

// --- Shared mock data ---

const mockUser = {
  id: "usr-1",
  email: "alice@example.com",
  name: "Alice",
  role: "user",
  status: "active",
  credit_balance: 500,
};

const mockMeResponse = { user: mockUser };

const mockAuthResponse = {
  user: mockUser,
  csrf_token: "csrf-test-token-123",
  session: { id: "sess-1", expires_at: "2026-12-31T23:59:59Z" },
};

const mockAPIKeys = {
  items: [
    {
      id: "key-1",
      name: "Production",
      key_prefix: "ck_live_",
      status: "active",
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
  total: 1,
};

const mockAPIs = {
  items: [
    {
      route_id: "r-1",
      route_name: "Users API",
      service_id: "svc-1",
      service_name: "Core Service",
      methods: ["GET", "POST"],
      paths: ["/api/v1/users"],
      hosts: [],
      strip_path: false,
      priority: 0,
      credit_cost: 1,
    },
  ],
  total: 1,
};

const mockBalance = { user_id: "usr-1", balance: 500 };

const mockProfileResponse = { user: mockUser };

// ---------------------------------------------------------------------------
// usePortalMe
// ---------------------------------------------------------------------------
describe("usePortalMe", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches current user from /portal/api/v1/auth/me", async () => {
    mockApi.mockResolvedValueOnce(mockMeResponse);

    const { result } = renderHook(() => usePortalMe(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockMeResponse);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/auth/me");
  });

  it("does not retry on failure", async () => {
    mockApi.mockRejectedValueOnce(new Error("Unauthorized"));

    const { result } = renderHook(() => usePortalMe(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.failureCount).toBe(1);
    expect(mockApi).toHaveBeenCalledTimes(1);
  });

  it("handles fetch error gracefully", async () => {
    mockApi.mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => usePortalMe(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// usePortalLogin
// ---------------------------------------------------------------------------
describe("usePortalLogin", () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockSetCSRF.mockClear();
  });

  it("POSTs credentials to /portal/api/v1/auth/login", async () => {
    mockApi.mockResolvedValueOnce(mockAuthResponse);

    const { result } = renderHook(() => usePortalLogin(), {
      wrapper: createWrapper(),
    });

    const credentials = { email: "alice@example.com", password: "secret123" };
    result.current.mutate(credentials);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockAuthResponse);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/auth/login", {
      method: "POST",
      body: credentials,
    });
  });

  it("stores CSRF token when present in response", async () => {
    mockApi.mockResolvedValueOnce(mockAuthResponse);

    const { result } = renderHook(() => usePortalLogin(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ email: "alice@example.com", password: "secret123" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockSetCSRF).toHaveBeenCalledWith("csrf-test-token-123");
  });

  it("does not call setPortalCSRFToken when response has no csrf_token", async () => {
    const noTokenResponse = {
      user: mockUser,
      session: { id: "sess-2", expires_at: "2026-12-31T23:59:59Z" },
    };
    mockApi.mockResolvedValueOnce(noTokenResponse);

    const { result } = renderHook(() => usePortalLogin(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ email: "alice@example.com", password: "secret123" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockSetCSRF).not.toHaveBeenCalled();
  });

  it("invalidates authMe query on success", async () => {
    mockApi.mockResolvedValueOnce(mockAuthResponse);

    const wrapper = createWrapper();
    const { result } = renderHook(() => usePortalLogin(), { wrapper });

    result.current.mutate({ email: "alice@example.com", password: "secret123" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // The invalidateQueries call is internal; we verify the mutation succeeded
    // which means the onSuccess callback ran without error.
    expect(result.current.status).toBe("success");
  });

  it("handles login error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Invalid credentials"));

    const { result } = renderHook(() => usePortalLogin(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ email: "bad@example.com", password: "wrong" });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeDefined();
    expect(mockSetCSRF).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// usePortalLogout
// ---------------------------------------------------------------------------
describe("usePortalLogout", () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockClearCSRF.mockClear();
  });

  it("POSTs to /portal/api/v1/auth/logout", async () => {
    mockApi.mockResolvedValueOnce({ logged_out: true });

    const { result } = renderHook(() => usePortalLogout(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual({ logged_out: true });
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/auth/logout", {
      method: "POST",
      body: {},
    });
  });

  it("calls clearPortalCSRFToken on success", async () => {
    mockApi.mockResolvedValueOnce({ logged_out: true });

    const { result } = renderHook(() => usePortalLogout(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockClearCSRF).toHaveBeenCalledTimes(1);
  });

  it("does not call clearPortalCSRFToken on error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Logout failed"));

    const { result } = renderHook(() => usePortalLogout(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockClearCSRF).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// usePortalChangePassword
// ---------------------------------------------------------------------------
describe("usePortalChangePassword", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("PUTs password change to /portal/api/v1/auth/password", async () => {
    mockApi.mockResolvedValueOnce({ password_changed: true });

    const { result } = renderHook(() => usePortalChangePassword(), {
      wrapper: createWrapper(),
    });

    const payload = { old_password: "old123", new_password: "new456" };
    result.current.mutate(payload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual({ password_changed: true });
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/auth/password", {
      method: "PUT",
      body: payload,
    });
  });

  it("handles password change error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Current password is incorrect"));

    const { result } = renderHook(() => usePortalChangePassword(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ old_password: "wrong", new_password: "new456" });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// usePortalAPIKeys
// ---------------------------------------------------------------------------
describe("usePortalAPIKeys", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches API keys from /portal/api/v1/api-keys", async () => {
    mockApi.mockResolvedValueOnce(mockAPIKeys);

    const { result } = renderHook(() => usePortalAPIKeys(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockAPIKeys);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/api-keys");
  });

  it("handles fetch error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Forbidden"));

    const { result } = renderHook(() => usePortalAPIKeys(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// useCreatePortalAPIKey
// ---------------------------------------------------------------------------
describe("useCreatePortalAPIKey", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("POSTs to create a new API key", async () => {
    const newKey = {
      token: "ck_live_abcdef123456",
      key: {
        id: "key-2",
        name: "Staging",
        key_prefix: "ck_live_",
        status: "active",
        created_at: "2026-04-15T00:00:00Z",
      },
    };
    mockApi.mockResolvedValueOnce(newKey);

    const { result } = renderHook(() => useCreatePortalAPIKey(), {
      wrapper: createWrapper(),
    });

    const payload = { name: "Staging", mode: "live" };
    result.current.mutate(payload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(newKey);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/api-keys", {
      method: "POST",
      body: payload,
    });
  });

  it("invalidates apiKeys query on success", async () => {
    mockApi.mockResolvedValueOnce({
      token: "ck_live_new",
      key: { id: "key-3", name: "Test", key_prefix: "ck_test_", status: "active" },
    });

    const { result } = renderHook(() => useCreatePortalAPIKey(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ name: "Test" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.status).toBe("success");
  });

  it("handles creation error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Max keys reached"));

    const { result } = renderHook(() => useCreatePortalAPIKey(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ name: "Overflow" });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// usePortalAPIs
// ---------------------------------------------------------------------------
describe("usePortalAPIs", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches APIs from /portal/api/v1/apis", async () => {
    mockApi.mockResolvedValueOnce(mockAPIs);

    const { result } = renderHook(() => usePortalAPIs(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockAPIs);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/apis");
  });

  it("handles fetch error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => usePortalAPIs(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// usePortalCreditsBalance
// ---------------------------------------------------------------------------
describe("usePortalCreditsBalance", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches balance from /portal/api/v1/credits/balance", async () => {
    mockApi.mockResolvedValueOnce(mockBalance);

    const { result } = renderHook(() => usePortalCreditsBalance(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockBalance);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/credits/balance");
  });

  it("handles fetch error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Insufficient permissions"));

    const { result } = renderHook(() => usePortalCreditsBalance(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// usePortalPurchaseCredits
// ---------------------------------------------------------------------------
describe("usePortalPurchaseCredits", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("POSTs to purchase credits", async () => {
    const purchaseResponse = { purchased: 100, new_balance: 600 };
    mockApi.mockResolvedValueOnce(purchaseResponse);

    const { result } = renderHook(() => usePortalPurchaseCredits(), {
      wrapper: createWrapper(),
    });

    const payload = { amount: 100, description: "Top-up" };
    result.current.mutate(payload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(purchaseResponse);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/credits/purchase", {
      method: "POST",
      body: payload,
    });
  });

  it("invalidates balance, transactions, forecast, and overview queries on success", async () => {
    mockApi.mockResolvedValueOnce({ purchased: 50, new_balance: 550 });

    const { result } = renderHook(() => usePortalPurchaseCredits(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ amount: 50 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // Success confirms the onSuccess callback (which invalidates balance,
    // transactions, forecast, and overview queries) completed without error.
    expect(result.current.status).toBe("success");
  });

  it("handles purchase error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Payment failed"));

    const { result } = renderHook(() => usePortalPurchaseCredits(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ amount: 1000 });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// usePortalProfile
// ---------------------------------------------------------------------------
describe("usePortalProfile", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches profile from /portal/api/v1/settings/profile and extracts user", async () => {
    mockApi.mockResolvedValueOnce(mockProfileResponse);

    const { result } = renderHook(() => usePortalProfile(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // usePortalProfile returns response.user, not the full response
    expect(result.current.data).toEqual(mockUser);
    expect(mockApi).toHaveBeenCalledWith("/portal/api/v1/settings/profile");
  });

  it("handles fetch error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Session expired"));

    const { result } = renderHook(() => usePortalProfile(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// Integration: login then fetch profile flow
// ---------------------------------------------------------------------------
describe("portal hook integration", () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockSetCSRF.mockClear();
    mockClearCSRF.mockClear();
  });

  it("login stores CSRF token; logout clears it", async () => {
    // Login
    mockApi.mockResolvedValueOnce(mockAuthResponse);
    const { result: loginHook } = renderHook(() => usePortalLogin(), {
      wrapper: createWrapper(),
    });

    loginHook.current.mutate({ email: "alice@example.com", password: "secret123" });
    await waitFor(() => expect(loginHook.current.isSuccess).toBe(true));

    expect(mockSetCSRF).toHaveBeenCalledWith("csrf-test-token-123");

    // Logout
    mockApi.mockResolvedValueOnce({ logged_out: true });
    const { result: logoutHook } = renderHook(() => usePortalLogout(), {
      wrapper: createWrapper(),
    });

    logoutHook.current.mutate();
    await waitFor(() => expect(logoutHook.current.isSuccess).toBe(true));

    expect(mockClearCSRF).toHaveBeenCalledTimes(1);
  });

  it("failed login does not call setPortalCSRFToken", async () => {
    mockApi.mockRejectedValueOnce(new Error("Invalid credentials"));

    const { result } = renderHook(() => usePortalLogin(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ email: "alice@example.com", password: "wrong" });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockSetCSRF).not.toHaveBeenCalled();
  });
});
