import { API_CONFIG } from "./constants";

type QueryValue = string | number | boolean | null | undefined;

export type ApiRequestOptions = Omit<RequestInit, "body"> & {
  query?: Record<string, QueryValue>;
  body?: unknown;
  timeoutMs?: number;
};

export class ApiError extends Error {
  status: number;
  code?: string;
  payload?: unknown;

  constructor(message: string, status: number, code?: string, payload?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.payload = payload;
  }
}

export function isAdminAuthenticated(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return window.sessionStorage.getItem(API_CONFIG.adminAuthStateKey) === "true";
}

export function setAdminAuthenticated(value: boolean) {
  if (typeof window === "undefined") {
    return;
  }
  if (!value) {
    window.sessionStorage.removeItem(API_CONFIG.adminAuthStateKey);
    return;
  }
  window.sessionStorage.setItem(API_CONFIG.adminAuthStateKey, "true");
}

export function clearAdminAuthenticated() {
  if (typeof window === "undefined") {
    return;
  }
  window.sessionStorage.removeItem(API_CONFIG.adminAuthStateKey);
}

export async function exchangeAdminKeyForToken(adminApiKey: string): Promise<void> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), API_CONFIG.requestTimeoutMs);

  try {
    const response = await fetch(resolveUrl("/admin/api/v1/auth/token"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Admin-Key": adminApiKey.trim(),
      },
      signal: controller.signal,
      credentials: "same-origin",
    });

    const payload = await parseJsonSafe(response);
    if (!response.ok) {
      throw new ApiError("Invalid admin key", response.status, "admin_unauthorized", payload);
    }
    setAdminAuthenticated(true);
  } finally {
    clearTimeout(timeout);
  }
}

function withQuery(path: string, query?: Record<string, QueryValue>) {
  if (!query || Object.keys(query).length === 0) {
    return path;
  }
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === "") {
      continue;
    }
    params.set(key, String(value));
  }
  const qs = params.toString();
  if (!qs) {
    return path;
  }
  return `${path}${path.includes("?") ? "&" : "?"}${qs}`;
}

function resolveUrl(path: string) {
  if (/^https?:\/\//i.test(path)) {
    return path;
  }
  if (!API_CONFIG.baseUrl) {
    return path;
  }
  return `${API_CONFIG.baseUrl.replace(/\/+$/, "")}/${path.replace(/^\/+/, "")}`;
}

async function parseJsonSafe(response: Response) {
  const raw = await response.text();
  if (!raw) {
    return null;
  }
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    return raw;
  }
}

export async function adminApiRequest<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), options.timeoutMs ?? API_CONFIG.requestTimeoutMs);
  const signal = options.signal ? AbortSignal.any([options.signal, controller.signal]) : controller.signal;
  const headers = new Headers(options.headers);

  let body: BodyInit | null = null;
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(options.body);
  }

  const url = resolveUrl(withQuery(path, options.query));
  try {
    const response = await fetch(url, {
      method: options.method ?? "GET",
      headers,
      body,
      signal,
      credentials: options.credentials ?? "same-origin",
    });

    const payload = await parseJsonSafe(response);
    if (!response.ok) {
      const message =
        (typeof payload === "object" &&
          payload !== null &&
          "error" in payload &&
          typeof (payload as { error?: unknown }).error === "object" &&
          (payload as { error?: { message?: string } }).error?.message) ||
        response.statusText ||
        "API request failed";
      const code =
        typeof payload === "object" &&
        payload !== null &&
        "error" in payload &&
        typeof (payload as { error?: unknown }).error === "object" &&
        (payload as { error?: { code?: string } }).error?.code
          ? (payload as { error?: { code?: string } }).error?.code
          : undefined;
      throw new ApiError(String(message), response.status, code, payload);
    }

    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) {
      throw error;
    }
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new ApiError("Request timed out", 408, "request_timeout");
    }
    throw new ApiError("Network request failed", 0, "network_error", error);
  } finally {
    clearTimeout(timeout);
  }
}

export function isApiError(value: unknown): value is ApiError {
  return value instanceof ApiError;
}
