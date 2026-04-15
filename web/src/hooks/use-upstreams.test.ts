import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import {
  useUpstreams,
  useUpstream,
  useUpstreamHealth,
  useCreateUpstream,
  useUpdateUpstream,
  useDeleteUpstream,
  useAddUpstreamTarget,
  useDeleteUpstreamTarget,
} from "./use-upstreams";
import { adminApiRequest } from "@/lib/api";

// Mock the API module
vi.mock("@/lib/api", () => ({
  adminApiRequest: vi.fn(),
}));

const mockApi = vi.mocked(adminApiRequest);

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

const mockUpstreams = [
  {
    id: "up-1",
    name: "Primary Upstream",
    algorithm: "round_robin",
    targets: [
      { id: "tgt-1", address: "10.0.0.1:3000", weight: 100 },
      { id: "tgt-2", address: "10.0.0.2:3000", weight: 50 },
    ],
  },
  {
    id: "up-2",
    name: "Secondary Upstream",
    algorithm: "least_connections",
    targets: [{ id: "tgt-3", address: "10.0.1.1:8080", weight: 100 }],
  },
];

describe("useUpstreams", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches and returns upstreams list from correct endpoint", async () => {
    mockApi.mockResolvedValueOnce(mockUpstreams);

    const { result } = renderHook(() => useUpstreams(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockUpstreams);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/upstreams");
  });

  it("handles fetch error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useUpstreams(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useUpstream", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches a single upstream by id", async () => {
    mockApi.mockResolvedValueOnce(mockUpstreams[0]);

    const { result } = renderHook(() => useUpstream("up-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(mockUpstreams[0]);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/upstreams/up-1");
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHook(() => useUpstream(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockApi).not.toHaveBeenCalled();
  });
});

describe("useUpstreamHealth", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches health data for an upstream", async () => {
    const healthData = {
      "10.0.0.1:3000": { status: "healthy", latency_ms: 12 },
      "10.0.0.2:3000": { status: "unhealthy", latency_ms: 500 },
    };
    mockApi.mockResolvedValueOnce(healthData);

    const { result } = renderHook(() => useUpstreamHealth("up-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(healthData);
    expect(mockApi).toHaveBeenCalledWith(
      "/admin/api/v1/upstreams/up-1/health",
    );
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHook(() => useUpstreamHealth(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockApi).not.toHaveBeenCalled();
  });
});

describe("useCreateUpstream", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls POST with payload and returns created upstream", async () => {
    const payload = { name: "New Upstream", algorithm: "random" };
    const created = { id: "up-3", ...payload, targets: [] };
    mockApi.mockResolvedValueOnce(created);

    const { result } = renderHook(() => useCreateUpstream(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(payload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(created);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/upstreams", {
      method: "POST",
      body: payload,
    });
  });
});

describe("useUpdateUpstream", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls PUT with id and payload", async () => {
    const payload = { name: "Updated Upstream", algorithm: "ip_hash" };
    const updated = { ...mockUpstreams[0], ...payload };
    mockApi.mockResolvedValueOnce(updated);

    const { result } = renderHook(() => useUpdateUpstream(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ id: "up-1", payload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(updated);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/upstreams/up-1", {
      method: "PUT",
      body: payload,
    });
  });
});

describe("useDeleteUpstream", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls DELETE with the upstream id", async () => {
    mockApi.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useDeleteUpstream(), {
      wrapper: createWrapper(),
    });

    result.current.mutate("up-1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/upstreams/up-1", {
      method: "DELETE",
    });
  });
});

describe("useAddUpstreamTarget", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls POST to targets endpoint with payload", async () => {
    const targetPayload = { address: "10.0.0.3:3000", weight: 75 };
    const createdTarget = { id: "tgt-4", ...targetPayload };
    mockApi.mockResolvedValueOnce(createdTarget);

    const { result } = renderHook(() => useAddUpstreamTarget(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ id: "up-1", payload: targetPayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(createdTarget);
    expect(mockApi).toHaveBeenCalledWith(
      "/admin/api/v1/upstreams/up-1/targets",
      { method: "POST", body: targetPayload },
    );
  });
});

describe("useDeleteUpstreamTarget", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls DELETE to target endpoint", async () => {
    mockApi.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useDeleteUpstreamTarget(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ id: "up-1", targetId: "tgt-1" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApi).toHaveBeenCalledWith(
      "/admin/api/v1/upstreams/up-1/targets/tgt-1",
      { method: "DELETE" },
    );
  });
});
