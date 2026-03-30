export const PORTAL_ROUTES = {
  base: "/portal",
  login: "/portal/login",
  dashboard: "/portal/dashboard",
  apiKeys: "/portal/api-keys",
  apis: "/portal/apis",
  playground: "/portal/playground",
  usage: "/portal/usage",
  logs: "/portal/logs",
  logDetail: (id: string) => `/portal/logs/${id}`,
  credits: "/portal/credits",
  security: "/portal/security",
  settings: "/portal/settings",
} as const;
