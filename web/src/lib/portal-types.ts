export type PortalUser = {
  id: string;
  email: string;
  name: string;
  company?: string;
  role: string;
  status: string;
  credit_balance: number;
  rate_limits?: Record<string, unknown>;
  ip_whitelist?: string[];
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type PortalSession = {
  id: string;
  expires_at: string;
};

export type PortalAuthResponse = {
  user: PortalUser;
  session: PortalSession;
};

export type PortalMeResponse = {
  user: PortalUser;
};

export type PortalAPIKey = {
  id: string;
  name: string;
  key_prefix: string;
  status: string;
  expires_at?: string;
  last_used_at?: string;
  last_used_ip?: string;
  created_at?: string;
  updated_at?: string;
};

export type PortalAPIKeyListResponse = {
  items: PortalAPIKey[];
  total: number;
};

export type PortalCreateAPIKeyResponse = {
  token: string;
  key: PortalAPIKey;
};

export type PortalAPIItem = {
  route_id: string;
  route_name: string;
  service_id: string;
  service_name: string;
  methods: string[];
  paths: string[];
  hosts: string[];
  strip_path: boolean;
  priority: number;
  credit_cost: number;
};

export type PortalAPIListResponse = {
  items: PortalAPIItem[];
  total: number;
};

export type PortalAPIDetailResponse = {
  route: {
    id: string;
    name: string;
    hosts: string[];
    paths: string[];
    methods: string[];
    strip_path: boolean;
    priority: number;
    credit_cost: number;
  };
  service: {
    id: string;
    name: string;
    protocol: string;
    upstream: string;
  };
  permission?: {
    id: string;
    route_id: string;
    methods: string[];
    allowed: boolean;
  };
};

export type PortalPlaygroundRequestPayload = {
  method: string;
  path: string;
  query: Record<string, string>;
  headers: Record<string, string>;
  body: string;
  api_key: string;
  timeout_ms?: number;
};

export type PortalPlaygroundResponse = {
  request: {
    method: string;
    url: string;
  };
  response: {
    status_code: number;
    headers: Record<string, string>;
    body: string;
    latency_ms: number;
  };
};

export type PlaygroundTemplate = {
  id: string;
  user_id: string;
  name: string;
  method: string;
  path: string;
  query: Record<string, string>;
  headers: Record<string, string>;
  body: string;
  created_at: string;
  updated_at: string;
};

export type PlaygroundTemplateListResponse = {
  items: PlaygroundTemplate[];
  total: number;
};

export type PortalUsageOverview = {
  from: string;
  to: string;
  total_requests: number;
  error_requests: number;
  error_rate: number;
  avg_latency_ms: number;
  credit_balance: number;
};

export type PortalUsageTimeseriesItem = {
  timestamp: string;
  requests: number;
  errors: number;
  avg_latency_ms: number;
};

export type PortalUsageTimeseries = {
  from: string;
  to: string;
  granularity: string;
  items: PortalUsageTimeseriesItem[];
};

export type PortalTopEndpoint = {
  route_id: string;
  route_name: string;
  count: number;
};

export type PortalUsageTopEndpoints = {
  from: string;
  to: string;
  items: PortalTopEndpoint[];
};

export type PortalUsageErrors = {
  from: string;
  to: string;
  status_map: Record<string, number>;
};

export type PortalLogEntry = {
  id: string;
  request_id: string;
  route_id: string;
  route_name: string;
  service_name: string;
  user_id: string;
  consumer_name: string;
  method: string;
  host: string;
  path: string;
  query: string;
  status_code: number;
  latency_ms: number;
  bytes_in: number;
  bytes_out: number;
  client_ip: string;
  user_agent: string;
  blocked: boolean;
  block_reason: string;
  request_headers?: Record<string, unknown>;
  request_body?: string;
  response_headers?: Record<string, unknown>;
  response_body?: string;
  error_message?: string;
  created_at: string;
};

export type PortalLogListResponse = {
  entries: PortalLogEntry[];
  total: number;
};

export type PortalBalance = {
  user_id: string;
  balance: number;
};

export type PortalCreditTransaction = {
  id: string;
  user_id: string;
  type: string;
  amount: number;
  balance_before: number;
  balance_after: number;
  description: string;
  request_id?: string;
  route_id?: string;
  created_at: string;
};

export type PortalTransactionList = {
  transactions: PortalCreditTransaction[];
  total: number;
};

export type PortalForecast = {
  balance: number;
  average_daily_consumption: number;
  projected_days_remaining: number;
  consumption_days_considered: number;
};

export type PortalPurchaseResponse = {
  purchased: number;
  new_balance: number;
};

export type PortalIPListResponse = {
  user_id: string;
  ips: string[];
};

export type PortalActivityEvent = {
  type: string;
  timestamp: string;
  message?: string;
  client_ip?: string;
  user_agent?: string;
  method?: string;
  path?: string;
  status_code?: number;
  latency_ms?: number;
};

export type PortalActivityListResponse = {
  items: PortalActivityEvent[];
  total: number;
};

export type PortalNotificationsResponse = {
  updated: boolean;
  notifications: unknown;
};
