import { API_CONFIG } from "./constants";

type QueryValue = string | number | boolean | null | undefined;

// CSRF Token Management — double-submit pattern
// The CSRF token is stored in an HttpOnly cookie by the server and read by
// JavaScript for the double-submit header. We read from cookie directly
// to avoid duplicating sensitive tokens in sessionStorage (XSS risk).
const CSRF_COOKIE_NAME = "csrf_token";

export function setPortalCSRFToken(_token: string) {
  // No-op: token is managed by the server via HttpOnly cookie.
  // The browser automatically sends cookies on same-origin requests.
}

export function getPortalCSRFToken(): string | null {
  if (typeof document === "undefined") {
    return null;
  }
  const match = document.cookie.match(new RegExp("(^| )" + CSRF_COOKIE_NAME + "=([^;]+)"));
  return match ? match[2] : null;
}

export function clearPortalCSRFToken() {
  // No-op: server manages cookie expiry via MaxAge.
}

export type PortalApiRequestOptions = Omit<RequestInit, "body"> & {
  query?: Record<string, QueryValue>;
  body?: unknown;
  timeoutMs?: number;
};

export class PortalApiError extends Error {
  status: number;
  code?: string;
  payload?: unknown;

  constructor(message: string, status: number, code?: string, payload?: unknown) {
    super(message);
    this.name = "PortalApiError";
    this.status = status;
    this.code = code;
    this.payload = payload;
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

function resolveBaseUrl() {
  const explicit = (import.meta.env.VITE_PORTAL_API_BASE_URL as string | undefined)?.trim();
  if (explicit) {
    return explicit.replace(/\/+$/, "");
  }
  if (API_CONFIG.baseUrl) {
    return API_CONFIG.baseUrl.replace(/\/+$/, "");
  }
  return "";
}

function resolveUrl(path: string) {
  if (/^https?:\/\//i.test(path)) {
    return path;
  }
  const base = resolveBaseUrl();
  if (!base) {
    return path;
  }
  return `${base}/${path.replace(/^\/+/, "")}`;
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

export async function portalApiRequest<T>(path: string, options: PortalApiRequestOptions = {}): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), options.timeoutMs ?? API_CONFIG.requestTimeoutMs);
  const signal = options.signal ? AbortSignal.any([options.signal, controller.signal]) : controller.signal;

  const headers = new Headers(options.headers);
  let body: BodyInit | null = null;
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(options.body);
  }

  // Add CSRF token for state-changing operations
  const method = options.method ?? "GET";
  if (method === "POST" || method === "PUT" || method === "DELETE" || method === "PATCH") {
    const csrfToken = getPortalCSRFToken();
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
      credentials: options.credentials ?? "include",
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
        "Portal API request failed";
      const code =
        typeof payload === "object" &&
        payload !== null &&
        "error" in payload &&
        typeof (payload as { error?: unknown }).error === "object" &&
        (payload as { error?: { code?: string } }).error?.code
          ? (payload as { error?: { code?: string } }).error?.code
          : undefined;
      throw new PortalApiError(String(message), response.status, code, payload);
    }
    return payload as T;
  } catch (error) {
    if (error instanceof PortalApiError) {
      throw error;
    }
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new PortalApiError("Request timed out", 408, "request_timeout");
    }
    throw new PortalApiError("Network request failed", 0, "network_error", error);
  } finally {
    clearTimeout(timeout);
  }
}

export function isPortalApiError(value: unknown): value is PortalApiError {
  return value instanceof PortalApiError;
}
