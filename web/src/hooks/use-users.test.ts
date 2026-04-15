import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import {
  useUsers,
  useUser,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
  useSuspendUser,
  useActivateUser,
} from "./use-users";
import { adminApiRequest } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  adminApiRequest: vi.fn(),
}));

const mockApi = vi.mocked(adminApiRequest);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

const mockUser = {
  id: "user-1",
  email: "alice@example.com",
  name: "Alice",
  role: "admin",
  status: "active",
  credit_balance: 500,
};

const mockUser2 = {
  id: "user-2",
  email: "bob@example.com",
  name: "Bob",
  role: "user",
  status: "suspended",
  credit_balance: 100,
};

const mockUserListPayload = {
  users: [mockUser, mockUser2],
  total: 2,
};

describe("useUsers", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches user list with default params", async () => {
    mockApi.mockResolvedValueOnce(mockUserListPayload);

    const { result } = renderHook(() => useUsers(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual({ users: [mockUser, mockUser2], total: 2 });
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users", { query: {} });
  });

  it("passes search/status/role params as query params", async () => {
    mockApi.mockResolvedValueOnce(mockUserListPayload);

    const params = { search: "alice", status: "active", role: "admin" };
    const { result } = renderHook(() => useUsers(params), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users", {
      query: { search: "alice", status: "active", role: "admin" },
    });
  });

  it("normalizes response via normalizeUserListResponse", async () => {
    // API returns uppercase keys — normalizeUserListResponse should handle both
    const uppercasePayload = { Users: [mockUser], Total: 1 };
    mockApi.mockResolvedValueOnce(uppercasePayload);

    const { result } = renderHook(() => useUsers(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual({ users: [mockUser], total: 1 });
  });

  it("handles fetch error", async () => {
    mockApi.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useUsers(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useUser", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("fetches single user by id", async () => {
    mockApi.mockResolvedValueOnce(mockUser);

    const { result } = renderHook(() => useUser("user-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockUser);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users/user-1");
  });

  it("is disabled when id is empty", () => {
    const { result } = renderHook(() => useUser(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockApi).not.toHaveBeenCalled();
  });
});

describe("useCreateUser", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls POST with payload", async () => {
    const payload = { name: "Charlie", email: "charlie@example.com", role: "user" };
    const created = { id: "user-3", ...payload, status: "active", credit_balance: 0 };
    mockApi.mockResolvedValueOnce(created);

    const { result } = renderHook(() => useCreateUser(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(payload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(created);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users", {
      method: "POST",
      body: payload,
    });
  });

  it("invalidates users query on success", async () => {
    mockApi.mockResolvedValueOnce(mockUser);

    // Spy on invalidateQueries by using a custom query client
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(() => useCreateUser(), { wrapper });

    result.current.mutate({ name: "Dave", email: "dave@example.com" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", {}],
    });
  });
});

describe("useUpdateUser", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls PUT with id and payload", async () => {
    const updated = { ...mockUser, name: "Alice Updated" };
    mockApi.mockResolvedValueOnce(updated);

    const { result } = renderHook(() => useUpdateUser(), {
      wrapper: createWrapper(),
    });

    const payload = { name: "Alice Updated" };
    result.current.mutate({ id: "user-1", payload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(updated);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users/user-1", {
      method: "PUT",
      body: payload,
    });
  });

  it("invalidates users list and single user query on success", async () => {
    mockApi.mockResolvedValueOnce(mockUser);

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(() => useUpdateUser(), { wrapper });

    result.current.mutate({ id: "user-1", payload: { name: "Updated" } });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", {}],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", "user-1"],
    });
  });
});

describe("useDeleteUser", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls DELETE with id", async () => {
    mockApi.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useDeleteUser(), {
      wrapper: createWrapper(),
    });

    result.current.mutate("user-1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users/user-1", {
      method: "DELETE",
    });
  });

  it("invalidates users query on success", async () => {
    mockApi.mockResolvedValueOnce(null);

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(() => useDeleteUser(), { wrapper });

    result.current.mutate("user-1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", {}],
    });
  });
});

describe("useSuspendUser", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls POST /suspend", async () => {
    const suspended = { ...mockUser, status: "suspended" };
    mockApi.mockResolvedValueOnce(suspended);

    const { result } = renderHook(() => useSuspendUser(), {
      wrapper: createWrapper(),
    });

    result.current.mutate("user-1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(suspended);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users/user-1/suspend", {
      method: "POST",
      body: {},
    });
  });

  it("invalidates users list and single user query on success", async () => {
    mockApi.mockResolvedValueOnce(mockUser);

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(() => useSuspendUser(), { wrapper });

    result.current.mutate("user-1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", {}],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", "user-1"],
    });
  });
});

describe("useActivateUser", () => {
  beforeEach(() => {
    mockApi.mockReset();
  });

  it("calls POST /activate", async () => {
    const activated = { ...mockUser2, status: "active" };
    mockApi.mockResolvedValueOnce(activated);

    const { result } = renderHook(() => useActivateUser(), {
      wrapper: createWrapper(),
    });

    result.current.mutate("user-2");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(activated);
    expect(mockApi).toHaveBeenCalledWith("/admin/api/v1/users/user-2/activate", {
      method: "POST",
      body: {},
    });
  });

  it("invalidates users list and single user query on success", async () => {
    mockApi.mockResolvedValueOnce(mockUser);

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const { result } = renderHook(() => useActivateUser(), { wrapper });

    result.current.mutate("user-2");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", {}],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["users", "user-2"],
    });
  });
});
