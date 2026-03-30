import { API_CONFIG } from "./constants";

type QueryValue = string | number | boolean | null | undefined;

export type ApiRequestOptions = Omit<RequestInit, "body"> & {
  query?: Record<string, QueryValue>;
  body?: unknown;
  timeoutMs?: number;
  adminApiKey?: string;
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

function resolveAdminKey(override?: string) {
  if (override) {
    return override.trim();
  }
  return getStoredAdminApiKey();
}

export function getStoredAdminApiKey() {
  if (typeof window === "undefined") {
    return "";
  }
  return window.localStorage.getItem(API_CONFIG.adminApiKeyStorageKey) ?? "";
}

export function setStoredAdminApiKey(value: string) {
  if (typeof window === "undefined") {
    return;
  }
  const trimmed = value.trim();
  if (!trimmed) {
    window.localStorage.removeItem(API_CONFIG.adminApiKeyStorageKey);
    return;
  }
  window.localStorage.setItem(API_CONFIG.adminApiKeyStorageKey, trimmed);
}

export function clearStoredAdminApiKey() {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.removeItem(API_CONFIG.adminApiKeyStorageKey);
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
  const apiKey = resolveAdminKey(options.adminApiKey);
  if (apiKey) {
    headers.set("X-Admin-Key", apiKey);
  }

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
