export const APP_NAME = "API Cerberus";

export const ROUTES = {
  dashboard: "/",
  services: "/services",
  serviceDetail: (id: string) => `/services/${id}`,
  routes: "/routes",
  routeDetail: (id: string) => `/routes/${id}`,
  upstreams: "/upstreams",
  upstreamDetail: (id: string) => `/upstreams/${id}`,
  consumers: "/consumers",
  plugins: "/plugins",
  users: "/users",
  userDetail: (id: string) => `/users/${id}`,
  credits: "/credits",
  auditLogs: "/audit-logs",
  analytics: "/analytics",
  config: "/config",
  settings: "/settings",
} as const;

export const COLOR_TOKENS = {
  brand: "rgb(207 65 16)",
  success: "rgb(8 145 107)",
  warning: "rgb(217 119 6)",
  danger: "rgb(185 28 28)",
  info: "rgb(2 132 199)",
} as const;

export const API_CONFIG = {
  baseUrl: import.meta.env.VITE_ADMIN_API_BASE_URL ?? "",
  adminApiKeyStorageKey: "apicerberus.admin_api_key",
  requestTimeoutMs: 15_000,
} as const;

export const WS_CONFIG = {
  url: import.meta.env.VITE_ADMIN_WS_URL ?? "",
  path: "/admin/api/v1/ws",
  reconnectInitialDelayMs: 500,
  reconnectMaxDelayMs: 10_000,
  reconnectBackoffMultiplier: 1.8,
} as const;

export const STORAGE_KEYS = {
  themeMode: "apicerberus.theme_mode",
} as const;

export const BREAKPOINTS = {
  sm: 640,
  md: 768,
  lg: 1024,
  xl: 1280,
  "2xl": 1536,
} as const;
