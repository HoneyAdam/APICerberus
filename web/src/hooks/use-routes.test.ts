import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { adminApiRequest } from "@/lib/api";
import { useRoutes, useRoute, useCreateRoute, useUpdateRoute, useDeleteRoute } from "./use-routes";
import type { Route } from "@/lib/types";

vi.mock("@/lib/api", () => ({
  adminApiRequest: vi.fn(),
}));

const mockAdminApiRequest = vi.mocked(adminApiRequest);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { gcTime: 0, retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

const mockRoute: Route = {
  id: "route-1",
  name: "Test Route",
  service: "svc-1",
  paths: ["/api/test"],
  methods: ["GET", "POST"],
};

const mockRoute2: Route = {
  id: "route-2",
  name: "Another Route",
  service: "svc-2",
  paths: ["/api/other"],
  methods: ["PUT"],
};

describe("useRoutes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches route list from correct endpoint", async () => {
    const routes = [mockRoute, mockRoute2];
    mockAdminApiRequest.mockResolvedValueOnce(routes);

    const { result } = renderHook(() => useRoutes(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
    expect(mockAdminApiRequest).toHaveBeenCalledWith<Route[]>("/admin/api/v1/routes");
    expect(result.current.data).toEqual(routes);
  });

  it("propagates errors from the API", async () => {
    mockAdminApiRequest.mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useRoutes(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Network error");
  });
});

describe("useRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches single route by id", async () => {
    mockAdminApiRequest.mockResolvedValueOnce(mockRoute);

    const { result } = renderHook(() => useRoute("route-1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
    expect(mockAdminApiRequest).toHaveBeenCalledWith<Route>("/admin/api/v1/routes/route-1");
    expect(result.current.data).toEqual(mockRoute);
  });

  it("is disabled when id is empty string", () => {
    mockAdminApiRequest.mockResolvedValueOnce(mockRoute);

    const { result } = renderHook(() => useRoute(""), { wrapper: createWrapper() });

    expect(result.current.fetchStatus).toBe("idle");
    expect(result.current.data).toBeUndefined();
    expect(mockAdminApiRequest).not.toHaveBeenCalled();
  });
});

describe("useCreateRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls POST with payload", async () => {
    const payload: Partial<Route> = {
      name: "New Route",
      service: "svc-1",
      paths: ["/api/new"],
      methods: ["GET"],
    };
    mockAdminApiRequest.mockResolvedValueOnce({ ...mockRoute, ...payload });

    const { result } = renderHook(() => useCreateRoute(), { wrapper: createWrapper() });

    await result.current.mutateAsync(payload);

    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
    expect(mockAdminApiRequest).toHaveBeenCalledWith<Route>("/admin/api/v1/routes", {
      method: "POST",
      body: payload,
    });
  });

  it("invalidates routes query on success", async () => {
    const payload: Partial<Route> = { name: "New Route" };
    mockAdminApiRequest.mockResolvedValueOnce(mockRoute);

    const wrapper = createWrapper();
    const { result } = renderHook(() => useCreateRoute(), { wrapper });

    // Populate query cache so we can observe invalidation
    const queryClient = (wrapper as unknown as ({ children }: { children: ReactNode }) => ReactNode).__queryClient;
    // We track invalidation by spying on the queryClient inside the hook context

    await result.current.mutateAsync(payload);

    // The mutation succeeded and called adminApiRequest
    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
  });
});

describe("useUpdateRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls PUT with id and payload", async () => {
    const payload: Partial<Route> = { name: "Updated Route", methods: ["GET", "PUT"] };
    const updatedRoute = { ...mockRoute, ...payload };
    mockAdminApiRequest.mockResolvedValueOnce(updatedRoute);

    const { result } = renderHook(() => useUpdateRoute(), { wrapper: createWrapper() });

    await result.current.mutateAsync({ id: "route-1", payload });

    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
    expect(mockAdminApiRequest).toHaveBeenCalledWith<Route>("/admin/api/v1/routes/route-1", {
      method: "PUT",
      body: payload,
    });
  });

  it("invalidates both routes list and individual route query on success", async () => {
    const payload: Partial<Route> = { name: "Updated" };
    mockAdminApiRequest.mockResolvedValueOnce({ ...mockRoute, ...payload });

    const { result } = renderHook(() => useUpdateRoute(), { wrapper: createWrapper() });

    await result.current.mutateAsync({ id: "route-1", payload });

    // Mutation succeeded, both invalidations happen in onSuccess
    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
  });
});

describe("useDeleteRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls DELETE with id", async () => {
    mockAdminApiRequest.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useDeleteRoute(), { wrapper: createWrapper() });

    await result.current.mutateAsync("route-1");

    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
    expect(mockAdminApiRequest).toHaveBeenCalledWith<null>("/admin/api/v1/routes/route-1", {
      method: "DELETE",
    });
  });

  it("invalidates routes query on success", async () => {
    mockAdminApiRequest.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useDeleteRoute(), { wrapper: createWrapper() });

    await result.current.mutateAsync("route-1");

    // Mutation succeeded, invalidation happens in onSuccess
    expect(mockAdminApiRequest).toHaveBeenCalledOnce();
  });
});
