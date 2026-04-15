import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useAuditLogs, useAuditLog, useAuditLogStats, useAuditLogExport } from "./use-audit-logs";

vi.mock("@/lib/api", () => ({
  adminApiRequest: vi.fn(),
}));

import { adminApiRequest } from "@/lib/api";

const mockedApi = vi.mocked(adminApiRequest);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useAuditLogs", () => {
  it("fetches from audit-logs with default params", async () => {
    const response = { entries: [], total: 0 };
    mockedApi.mockResolvedValueOnce(response);

    const { result } = renderHook(() => useAuditLogs(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(response);
    expect(mockedApi).toHaveBeenCalledWith("/admin/api/v1/audit-logs", {
      query: {},
    });
  });

  it("passes filter params as query params", async () => {
    const response = { entries: [], total: 0 };
    mockedApi.mockResolvedValueOnce(response);

    const params = {
      method: "GET",
      status_min: 200,
      status_max: 299,
      limit: 50,
      offset: 100,
      blocked: true,
    };

    const { result } = renderHook(() => useAuditLogs(params), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(response);
    expect(mockedApi).toHaveBeenCalledWith("/admin/api/v1/audit-logs", {
      query: {
        method: "GET",
        status_min: 200,
        status_max: 299,
        limit: 50,
        offset: 100,
        blocked: true,
      },
    });
  });
});

describe("useAuditLog", () => {
  it("fetches single audit log by id", async () => {
    const entry = {
      id: "audit-123",
      request_id: "req-1",
      route_id: "route-1",
    };
    mockedApi.mockResolvedValueOnce(entry);

    const { result } = renderHook(() => useAuditLog("audit-123"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(entry);
    expect(mockedApi).toHaveBeenCalledWith("/admin/api/v1/audit-logs/audit-123");
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHook(() => useAuditLog(""), { wrapper: createWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockedApi).not.toHaveBeenCalled();
  });
});

describe("useAuditLogStats", () => {
  it("fetches stats endpoint", async () => {
    const stats = { total_requests: 1000, avg_latency_ms: 45 };
    mockedApi.mockResolvedValueOnce(stats);

    const { result } = renderHook(() => useAuditLogStats(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(stats);
    expect(mockedApi).toHaveBeenCalledWith("/admin/api/v1/audit-logs/stats", {
      query: {},
    });
  });

  it("passes filter params", async () => {
    const stats = { total_requests: 500 };
    mockedApi.mockResolvedValueOnce(stats);

    const params = { method: "POST", client_ip: "10.0.0.1" };

    const { result } = renderHook(() => useAuditLogStats(params), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(stats);
    expect(mockedApi).toHaveBeenCalledWith("/admin/api/v1/audit-logs/stats", {
      query: { method: "POST", client_ip: "10.0.0.1" },
    });
  });
});

describe("useAuditLogExport", () => {
  it("fetches export endpoint with format param", async () => {
    const exportData = "id,method,status\naudit-1,GET,200\n";
    mockedApi.mockResolvedValueOnce(exportData);

    const { result } = renderHook(() => useAuditLogExport({ format: "csv" }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBe(exportData);
    expect(mockedApi).toHaveBeenCalledWith("/admin/api/v1/audit-logs/export", {
      query: { format: "csv" },
    });
  });
});
