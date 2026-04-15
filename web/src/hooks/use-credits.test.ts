import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useCreditsOverview, useUserCreditBalance, useUserCreditTransactions, useTopupCredits, useDeductCredits } from "./use-credits";
import { adminApiRequest } from "@/lib/api";
import { normalizeCreditTransactionList, toQueryRecord } from "./helpers";

vi.mock("@/lib/api", () => ({
  adminApiRequest: vi.fn(),
}));

vi.mock("./helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./helpers")>();
  return {
    ...actual,
    normalizeCreditTransactionList: vi.fn((payload: unknown) => actual.normalizeCreditTransactionList(payload)),
    toQueryRecord: vi.fn((params: Record<string, unknown>) => actual.toQueryRecord(params)),
  };
});

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    );
  };
}

describe("useCreditsOverview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches from /credits/overview", async () => {
    const overview = {
      total_distributed: 10000,
      total_consumed: 3000,
      top_consumers: [],
    };
    vi.mocked(adminApiRequest).mockResolvedValueOnce(overview);

    const { result } = renderHook(() => useCreditsOverview(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(overview);
    expect(adminApiRequest).toHaveBeenCalledTimes(1);
    expect(adminApiRequest).toHaveBeenCalledWith("/admin/api/v1/credits/overview");
  });
});

describe("useUserCreditBalance", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches balance for user", async () => {
    const balance = { balance: 500 };
    vi.mocked(adminApiRequest).mockResolvedValueOnce(balance);

    const { result } = renderHook(() => useUserCreditBalance("user-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(balance);
    expect(adminApiRequest).toHaveBeenCalledTimes(1);
    expect(adminApiRequest).toHaveBeenCalledWith("/admin/api/v1/users/user-1/credits/balance");
  });

  it("is disabled when userID is empty", () => {
    const { result } = renderHook(() => useUserCreditBalance(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(result.current.isEnabled).toBe(false);
    expect(adminApiRequest).not.toHaveBeenCalled();
  });
});

describe("useUserCreditTransactions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches transactions with params", async () => {
    const rawPayload = {
      transactions: [
        { id: "tx-1", user_id: "user-1", type: "topup", amount: 100, balance_before: 0, balance_after: 100, description: "Initial", created_at: "2025-01-01T00:00:00Z" },
      ],
      total: 1,
    };
    const normalized = { transactions: rawPayload.transactions, total: 1 };

    vi.mocked(adminApiRequest).mockResolvedValueOnce(rawPayload);
    vi.mocked(normalizeCreditTransactionList).mockReturnValueOnce(normalized);

    const { result } = renderHook(
      () => useUserCreditTransactions("user-1", { type: "topup", limit: 10, offset: 0 }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // toQueryRecord should have been called with the params
    expect(toQueryRecord).toHaveBeenCalledWith({ type: "topup", limit: 10, offset: 0 });

    // adminApiRequest called with query params
    expect(adminApiRequest).toHaveBeenCalledTimes(1);
    const [url, options] = vi.mocked(adminApiRequest).mock.calls[0];
    expect(url).toBe("/admin/api/v1/users/user-1/credits/transactions");
    expect(options).toHaveProperty("query");

    // normalizeCreditTransactionList should have been called with raw payload
    expect(normalizeCreditTransactionList).toHaveBeenCalledWith(rawPayload);

    // Final data should be the normalized result
    expect(result.current.data).toEqual(normalized);
  });

  it("is disabled when userID is empty", () => {
    const { result } = renderHook(
      () => useUserCreditTransactions("", { limit: 10 }),
      { wrapper: createWrapper() },
    );

    expect(result.current.fetchStatus).toBe("idle");
    expect(result.current.isEnabled).toBe(false);
    expect(adminApiRequest).not.toHaveBeenCalled();
  });

  it("normalizes response via normalizeCreditTransactionList", async () => {
    const rawPayload = {
      Transactions: [
        { id: "tx-2", user_id: "user-2", type: "deduct", amount: 50, balance_before: 200, balance_after: 150, description: "Usage", created_at: "2025-01-02T00:00:00Z" },
      ],
      Total: 1,
    };
    const normalized = {
      transactions: rawPayload.Transactions,
      total: 1,
    };

    vi.mocked(adminApiRequest).mockResolvedValueOnce(rawPayload);
    vi.mocked(normalizeCreditTransactionList).mockReturnValueOnce(normalized);

    const { result } = renderHook(
      () => useUserCreditTransactions("user-2"),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(normalizeCreditTransactionList).toHaveBeenCalledWith(rawPayload);
    expect(result.current.data).toEqual(normalized);
  });
});

describe("useTopupCredits", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls POST topup with amount and reason", async () => {
    const response = { balance: 600 };
    vi.mocked(adminApiRequest).mockResolvedValueOnce(response);

    const { result } = renderHook(() => useTopupCredits(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ userID: "user-1", amount: 100, reason: "Bonus credits" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(adminApiRequest).toHaveBeenCalledTimes(1);
    expect(adminApiRequest).toHaveBeenCalledWith(
      "/admin/api/v1/users/user-1/credits/topup",
      { method: "POST", body: { amount: 100, reason: "Bonus credits" } },
    );
    expect(result.current.data).toEqual(response);
  });

  it("invalidates related queries on success", async () => {
    vi.mocked(adminApiRequest).mockResolvedValueOnce({ balance: 700 });

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useTopupCredits(), { wrapper });

    result.current.mutate({ userID: "user-1", amount: 200 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const calls = invalidateSpy.mock.calls.map((call) => JSON.stringify(call[0]));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["credits", "overview"] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["credits", "balance", "user-1"] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["credits", "transactions", "user-1", {}] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["users", {}] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["users", "user-1"] }));
    expect(invalidateSpy).toHaveBeenCalledTimes(5);
  });

  it("works without optional reason", async () => {
    vi.mocked(adminApiRequest).mockResolvedValueOnce({ balance: 800 });

    const { result } = renderHook(() => useTopupCredits(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ userID: "user-3", amount: 50 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(adminApiRequest).toHaveBeenCalledWith(
      "/admin/api/v1/users/user-3/credits/topup",
      { method: "POST", body: { amount: 50, reason: undefined } },
    );
  });
});

describe("useDeductCredits", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls POST deduct with amount and reason", async () => {
    const response = { balance: 400 };
    vi.mocked(adminApiRequest).mockResolvedValueOnce(response);

    const { result } = renderHook(() => useDeductCredits(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ userID: "user-1", amount: 100, reason: "Penalty" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(adminApiRequest).toHaveBeenCalledTimes(1);
    expect(adminApiRequest).toHaveBeenCalledWith(
      "/admin/api/v1/users/user-1/credits/deduct",
      { method: "POST", body: { amount: 100, reason: "Penalty" } },
    );
    expect(result.current.data).toEqual(response);
  });

  it("invalidates related queries on success", async () => {
    vi.mocked(adminApiRequest).mockResolvedValueOnce({ balance: 300 });

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useDeductCredits(), { wrapper });

    result.current.mutate({ userID: "user-1", amount: 50 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const calls = invalidateSpy.mock.calls.map((call) => JSON.stringify(call[0]));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["credits", "overview"] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["credits", "balance", "user-1"] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["credits", "transactions", "user-1", {}] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["users", {}] }));
    expect(calls).toContainEqual(JSON.stringify({ queryKey: ["users", "user-1"] }));
    expect(invalidateSpy).toHaveBeenCalledTimes(5);
  });

  it("works without optional reason", async () => {
    vi.mocked(adminApiRequest).mockResolvedValueOnce({ balance: 250 });

    const { result } = renderHook(() => useDeductCredits(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ userID: "user-3", amount: 25 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(adminApiRequest).toHaveBeenCalledWith(
      "/admin/api/v1/users/user-3/credits/deduct",
      { method: "POST", body: { amount: 25, reason: undefined } },
    );
  });
});
