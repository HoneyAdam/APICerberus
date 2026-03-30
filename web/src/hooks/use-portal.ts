import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toQueryRecord } from "./helpers";
import { portalApiRequest } from "@/lib/portal-api";
import type {
  PlaygroundTemplate,
  PlaygroundTemplateListResponse,
  PortalAPIDetailResponse,
  PortalAPIKeyListResponse,
  PortalAPIListResponse,
  PortalActivityListResponse,
  PortalAuthResponse,
  PortalBalance,
  PortalForecast,
  PortalIPListResponse,
  PortalLogEntry,
  PortalLogListResponse,
  PortalMeResponse,
  PortalNotificationsResponse,
  PortalPlaygroundRequestPayload,
  PortalPlaygroundResponse,
  PortalPurchaseResponse,
  PortalTransactionList,
  PortalUsageErrors,
  PortalUsageOverview,
  PortalUsageTimeseries,
  PortalUsageTopEndpoints,
  PortalUser,
} from "@/lib/portal-types";

const portalQueryKeys = {
  authMe: ["portal", "auth", "me"] as const,
  apiKeys: ["portal", "api-keys"] as const,
  apis: ["portal", "apis"] as const,
  apiDetail: (routeID: string) => ["portal", "apis", routeID] as const,
  templates: ["portal", "playground", "templates"] as const,
  usageOverview: (params?: Record<string, unknown>) => ["portal", "usage", "overview", params ?? {}] as const,
  usageTimeseries: (params?: Record<string, unknown>) => ["portal", "usage", "timeseries", params ?? {}] as const,
  usageTopEndpoints: (params?: Record<string, unknown>) => ["portal", "usage", "top-endpoints", params ?? {}] as const,
  usageErrors: (params?: Record<string, unknown>) => ["portal", "usage", "errors", params ?? {}] as const,
  logs: (params?: Record<string, unknown>) => ["portal", "logs", params ?? {}] as const,
  logDetail: (id: string) => ["portal", "log", id] as const,
  balance: ["portal", "credits", "balance"] as const,
  transactions: (params?: Record<string, unknown>) => ["portal", "credits", "transactions", params ?? {}] as const,
  forecast: ["portal", "credits", "forecast"] as const,
  ipWhitelist: ["portal", "security", "ip-whitelist"] as const,
  activity: ["portal", "security", "activity"] as const,
  profile: ["portal", "settings", "profile"] as const,
};

export type PortalWindowParams = {
  window?: string;
  from?: string;
  to?: string;
};

export type PortalUsageTimeseriesParams = PortalWindowParams & {
  granularity?: string;
};

export type PortalLogParams = {
  q?: string;
  route?: string;
  method?: string;
  status_min?: number;
  status_max?: number;
  limit?: number;
  offset?: number;
  window?: string;
  from?: string;
  to?: string;
};

export type PortalTransactionParams = {
  type?: string;
  limit?: number;
  offset?: number;
};

export function usePortalMe() {
  return useQuery({
    queryKey: portalQueryKeys.authMe,
    queryFn: () => portalApiRequest<PortalMeResponse>("/portal/api/v1/auth/me"),
    retry: false,
  });
}

export function usePortalLogin() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { email: string; password: string }) =>
      portalApiRequest<PortalAuthResponse>("/portal/api/v1/auth/login", {
        method: "POST",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.authMe });
    },
  });
}

export function usePortalLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      portalApiRequest<{ logged_out: boolean }>("/portal/api/v1/auth/logout", {
        method: "POST",
        body: {},
      }),
    onSuccess: async () => {
      await queryClient.removeQueries({ queryKey: ["portal"] });
    },
  });
}

export function usePortalChangePassword() {
  return useMutation({
    mutationFn: (payload: { old_password: string; new_password: string }) =>
      portalApiRequest<{ password_changed: boolean }>("/portal/api/v1/auth/password", {
        method: "PUT",
        body: payload,
      }),
  });
}

export function usePortalAPIKeys() {
  return useQuery({
    queryKey: portalQueryKeys.apiKeys,
    queryFn: () => portalApiRequest<PortalAPIKeyListResponse>("/portal/api/v1/api-keys"),
  });
}

export function useCreatePortalAPIKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { name: string; mode?: string }) =>
      portalApiRequest<{ token: string; key: PortalAPIKeyListResponse["items"][number] }>("/portal/api/v1/api-keys", {
        method: "POST",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.apiKeys });
    },
  });
}

export function useRenamePortalAPIKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) =>
      portalApiRequest<{ id: string; name: string; renamed: boolean }>(`/portal/api/v1/api-keys/${id}`, {
        method: "PUT",
        body: { name },
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.apiKeys });
    },
  });
}

export function useRevokePortalAPIKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      portalApiRequest<void>(`/portal/api/v1/api-keys/${id}`, {
        method: "DELETE",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.apiKeys });
    },
  });
}

export function usePortalAPIs() {
  return useQuery({
    queryKey: portalQueryKeys.apis,
    queryFn: () => portalApiRequest<PortalAPIListResponse>("/portal/api/v1/apis"),
  });
}

export function usePortalAPIDetail(routeID: string) {
  return useQuery({
    queryKey: portalQueryKeys.apiDetail(routeID),
    queryFn: () => portalApiRequest<PortalAPIDetailResponse>(`/portal/api/v1/apis/${routeID}`),
    enabled: Boolean(routeID),
  });
}

export function usePortalPlaygroundSend() {
  return useMutation({
    mutationFn: (payload: PortalPlaygroundRequestPayload) =>
      portalApiRequest<PortalPlaygroundResponse>("/portal/api/v1/playground/send", {
        method: "POST",
        body: payload,
      }),
  });
}

export function usePortalTemplates() {
  return useQuery({
    queryKey: portalQueryKeys.templates,
    queryFn: () => portalApiRequest<PlaygroundTemplateListResponse>("/portal/api/v1/playground/templates"),
  });
}

export function useSavePortalTemplate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: Partial<PlaygroundTemplate>) =>
      portalApiRequest<PlaygroundTemplate>("/portal/api/v1/playground/templates", {
        method: "POST",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.templates });
    },
  });
}

export function useDeletePortalTemplate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      portalApiRequest<void>(`/portal/api/v1/playground/templates/${id}`, {
        method: "DELETE",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.templates });
    },
  });
}

export function usePortalUsageOverview(params: PortalWindowParams = {}) {
  const query = toQueryRecord(params);
  return useQuery({
    queryKey: portalQueryKeys.usageOverview(query),
    queryFn: () => portalApiRequest<PortalUsageOverview>("/portal/api/v1/usage/overview", { query }),
  });
}

export function usePortalUsageTimeseries(params: PortalUsageTimeseriesParams = {}) {
  const query = toQueryRecord(params);
  return useQuery({
    queryKey: portalQueryKeys.usageTimeseries(query),
    queryFn: () => portalApiRequest<PortalUsageTimeseries>("/portal/api/v1/usage/timeseries", { query }),
  });
}

export function usePortalUsageTopEndpoints(params: PortalWindowParams = {}) {
  const query = toQueryRecord(params);
  return useQuery({
    queryKey: portalQueryKeys.usageTopEndpoints(query),
    queryFn: () => portalApiRequest<PortalUsageTopEndpoints>("/portal/api/v1/usage/top-endpoints", { query }),
  });
}

export function usePortalUsageErrors(params: PortalWindowParams = {}) {
  const query = toQueryRecord(params);
  return useQuery({
    queryKey: portalQueryKeys.usageErrors(query),
    queryFn: () => portalApiRequest<PortalUsageErrors>("/portal/api/v1/usage/errors", { query }),
  });
}

export function usePortalLogs(params: PortalLogParams = {}) {
  const query = toQueryRecord(params);
  return useQuery({
    queryKey: portalQueryKeys.logs(query),
    queryFn: () => portalApiRequest<PortalLogListResponse>("/portal/api/v1/logs", { query }),
  });
}

export function usePortalLogDetail(id: string) {
  return useQuery({
    queryKey: portalQueryKeys.logDetail(id),
    queryFn: () => portalApiRequest<PortalLogEntry>(`/portal/api/v1/logs/${id}`),
    enabled: Boolean(id),
  });
}

export function usePortalCreditsBalance() {
  return useQuery({
    queryKey: portalQueryKeys.balance,
    queryFn: () => portalApiRequest<PortalBalance>("/portal/api/v1/credits/balance"),
  });
}

export function usePortalCreditTransactions(params: PortalTransactionParams = {}) {
  const query = toQueryRecord(params);
  return useQuery({
    queryKey: portalQueryKeys.transactions(query),
    queryFn: () => portalApiRequest<PortalTransactionList>("/portal/api/v1/credits/transactions", { query }),
  });
}

export function usePortalForecast() {
  return useQuery({
    queryKey: portalQueryKeys.forecast,
    queryFn: () => portalApiRequest<PortalForecast>("/portal/api/v1/credits/forecast"),
  });
}

export function usePortalPurchaseCredits() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { amount: number; description?: string }) =>
      portalApiRequest<PortalPurchaseResponse>("/portal/api/v1/credits/purchase", {
        method: "POST",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.balance });
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.transactions() });
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.forecast });
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.usageOverview() });
    },
  });
}

export function usePortalIPWhitelist() {
  return useQuery({
    queryKey: portalQueryKeys.ipWhitelist,
    queryFn: () => portalApiRequest<PortalIPListResponse>("/portal/api/v1/security/ip-whitelist"),
  });
}

export function usePortalAddIPs() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { ips: string[] }) =>
      portalApiRequest<PortalIPListResponse>("/portal/api/v1/security/ip-whitelist", {
        method: "POST",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.ipWhitelist });
    },
  });
}

export function usePortalRemoveIP() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (ip: string) =>
      portalApiRequest<PortalIPListResponse>(`/portal/api/v1/security/ip-whitelist/${encodeURIComponent(ip)}`, {
        method: "DELETE",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.ipWhitelist });
    },
  });
}

export function usePortalActivity() {
  return useQuery({
    queryKey: portalQueryKeys.activity,
    queryFn: () => portalApiRequest<PortalActivityListResponse>("/portal/api/v1/security/activity"),
  });
}

export function usePortalProfile() {
  return useQuery({
    queryKey: portalQueryKeys.profile,
    queryFn: async () => {
      const response = await portalApiRequest<{ user: PortalUser }>("/portal/api/v1/settings/profile");
      return response.user;
    },
  });
}

export function usePortalUpdateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: Partial<PortalUser>) =>
      portalApiRequest<{ user: PortalUser }>("/portal/api/v1/settings/profile", {
        method: "PUT",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.profile });
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.authMe });
    },
  });
}

export function usePortalUpdateNotifications() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { notifications: unknown }) =>
      portalApiRequest<PortalNotificationsResponse>("/portal/api/v1/settings/notifications", {
        method: "PUT",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.profile });
      await queryClient.invalidateQueries({ queryKey: portalQueryKeys.authMe });
    },
  });
}

export async function exportPortalLogs(params: PortalLogParams = {}, format: "json" | "csv" | "jsonl" = "jsonl") {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(toQueryRecord({ ...params, format }))) {
    query.set(key, String(value));
  }
  const suffix = query.toString() ? `?${query.toString()}` : "";
  const response = await fetch(`/portal/api/v1/logs/export${suffix}`, {
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error("Failed to export logs");
  }
  const blob = await response.blob();
  const contentDisposition = response.headers.get("content-disposition") ?? "";
  const matched = contentDisposition.match(/filename="?([^";]+)"?/i);
  const fileName = matched?.[1] ?? `portal-logs.${format}`;
  return { blob, fileName };
}
