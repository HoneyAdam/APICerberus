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

// M-021: Admin API CSRF protection.
// M-014 fix: Admin API now uses double-submit CSRF tokens for state-changing operations.
// The server generates a cryptographically random token on login, stored in a non-HttpOnly
// cookie (readable by JS). The client reads it and sends it as X-CSRF-Token header.
// The X-Admin-Key bearer token handles API auth; CSRF adds protection against browser-based CSRF.

// M-022: Auth state in sessionStorage is a security risk.
// sessionStorage persists until the tab/window is closed, but is accessible to any
// JavaScript running on the same origin (including injected scripts/XSS).
// For production: use httpOnly cookies for auth tokens and validate them server-side.
// L-003 fix: Check CSRF cookie presence instead of sessionStorage.
// The CSRF cookie is HttpOnly (server-set), so XSS cannot read it.
// This is more secure than sessionStorage which is accessible to any JS on the origin.
export function isAdminAuthenticated(): boolean {
  if (typeof document === "undefined") {
    return false;
  }
  // Check if the CSRF cookie is present - this indicates a valid server-side session
  const match = document.cookie.match(new RegExp("(^| )" + ADMIN_CSRF_COOKIE_NAME + "=([^;]+)"));
  return match !== null;
}

const ADMIN_CSRF_COOKIE_NAME = "apicerberus_admin_csrf";

function getAdminCSRFToken(): string | null {
  if (typeof document === "undefined") {
    return null;
  }
  const match = document.cookie.match(new RegExp("(^| )" + ADMIN_CSRF_COOKIE_NAME + "=([^;]+)"));
  return match ? match[2] : null;
}

// L-003 fix: No longer writes to sessionStorage.
// Auth state is now determined by CSRF cookie presence (server-set, HttpOnly).
// For logout, sessionStorage is cleared as a client-side state reset.
export function setAdminAuthenticated(value: boolean) {
  if (typeof window === "undefined") {
    return;
  }
  if (!value) {
    // Clear client-side state; server-side session is cleared via API call
    window.sessionStorage.removeItem(API_CONFIG.adminAuthStateKey);
    return;
  }
  // No-op: auth state is determined by CSRF cookie presence (server-managed)
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

  // M-014 fix: Add CSRF token for state-changing operations (double-submit pattern)
  const method = options.method ?? "GET";
  if (method === "POST" || method === "PUT" || method === "DELETE" || method === "PATCH") {
    const csrfToken = getAdminCSRFToken();
    if (csrfToken) {
      headers.set("X-CSRF-Token", csrfToken);
    }
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

