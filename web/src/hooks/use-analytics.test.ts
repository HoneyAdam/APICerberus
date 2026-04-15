import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
  useAnalyticsOverview,
  useAnalyticsTimeseries,
  useAnalyticsTopRoutes,
} from "./use-analytics";

vi.mock("@/lib/api", () => ({
  adminApiRequest: vi.fn(),
}));

import { adminApiRequest } from "@/lib/api";

const mockedApi = vi.mocked(adminApiRequest);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

const mockOverview = {
  from: "2025-01-01T00:00:00Z",
  to: "2025-01-31T23:59:59Z",
  total_requests: 12_480,
  active_conns: 34,
  error_rate: 0.021,
  avg_latency_ms: 45.3,
  p50_latency_ms: 38.0,
  p95_latency_ms: 120.0,
  p99_latency_ms: 210.0,
  throughput_rps: 207.5,
};

const mockTimeseries = {
  from: "2025-01-01T00:00:00Z",
  to: "2025-01-31T23:59:59Z",
  granularity: "1h",
  items: [
    {
      timestamp: "2025-01-15T10:00:00Z",
      requests: 512,
      errors: 3,
      avg_latency_ms: 42.1,
      p50_latency_ms: 35.0,
      p95_latency_ms: 110.0,
      p99_latency_ms: 195.0,
    },
    {
      timestamp: "2025-01-15T11:00:00Z",
      requests: 487,
      errors: 1,
      avg_latency_ms: 39.8,
      p50_latency_ms: 33.0,
      p95_latency_ms: 105.0,
      p99_latency_ms: 188.0,
    },
  ],
};

const mockTopRoutes = {
  from: "2025-01-01T00:00:00Z",
  to: "2025-01-31T23:59:59Z",
  limit: 5,
  routes: [
    { route_id: "r-1", route_name: "GET /api/v1/users", request_count: 5200, error_count: 42, avg_latency_ms: 38.5 },
    { route_id: "r-2", route_name: "POST /api/v1/orders", request_count: 3100, error_count: 15, avg_latency_ms: 62.1 },
  ],
};

describe("useAnalyticsOverview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches overview with default params", async () => {
    mockedApi.mockResolvedValueOnce(mockOverview);

    const { result } = renderHook(() => useAnalyticsOverview(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockOverview);
    expect(mockedApi).toHaveBeenCalledOnce();
    expect(mockedApi).toHaveBeenCalledWith(
      "/admin/api/v1/analytics/overview",
      expect.objectContaining({ query: {} }),
    );
  });

  it("passes window/from/to params", async () => {
    mockedApi.mockResolvedValueOnce(mockOverview);

    const params = { window: "7d", from: "2025-01-01", to: "2025-01-07" };

    const { result } = renderHook(() => useAnalyticsOverview(params), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockOverview);
    expect(mockedApi).toHaveBeenCalledWith(
      "/admin/api/v1/analytics/overview",
      expect.objectContaining({ query: params }),
    );
  });
});

describe("useAnalyticsTimeseries", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches timeseries endpoint", async () => {
    mockedApi.mockResolvedValueOnce(mockTimeseries);

    const { result } = renderHook(() => useAnalyticsTimeseries(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockTimeseries);
    expect(mockedApi).toHaveBeenCalledOnce();
    expect(mockedApi).toHaveBeenCalledWith(
      "/admin/api/v1/analytics/timeseries",
      expect.objectContaining({ query: {} }),
    );
  });

  it("passes granularity param", async () => {
    mockedApi.mockResolvedValueOnce(mockTimeseries);

    const params = { granularity: "1h", window: "24h" };

    const { result } = renderHook(() => useAnalyticsTimeseries(params), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockTimeseries);
    expect(mockedApi).toHaveBeenCalledWith(
      "/admin/api/v1/analytics/timeseries",
      expect.objectContaining({ query: params }),
    );
  });
});

describe("useAnalyticsTopRoutes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches top-routes endpoint", async () => {
    mockedApi.mockResolvedValueOnce(mockTopRoutes);

    const { result } = renderHook(() => useAnalyticsTopRoutes(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockTopRoutes);
    expect(mockedApi).toHaveBeenCalledOnce();
    expect(mockedApi).toHaveBeenCalledWith(
      "/admin/api/v1/analytics/top-routes",
      expect.objectContaining({ query: {} }),
    );
  });

  it("passes limit param", async () => {
    mockedApi.mockResolvedValueOnce(mockTopRoutes);

    const params = { limit: 5, from: "2025-01-01", to: "2025-01-31" };

    const { result } = renderHook(() => useAnalyticsTopRoutes(params), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockTopRoutes);
    expect(mockedApi).toHaveBeenCalledWith(
      "/admin/api/v1/analytics/top-routes",
      expect.objectContaining({ query: params }),
    );
  });
});
