export type Service = {
  id: string;
  name: string;
  protocol: string;
  upstream: string;
  connect_timeout?: string | number;
  read_timeout?: string | number;
  write_timeout?: string | number;
};

export type Route = {
  id: string;
  name: string;
  service: string;
  hosts?: string[];
  paths: string[];
  methods: string[];
  strip_path?: boolean;
  preserve_host?: boolean;
  priority?: number;
  plugins?: Array<{ name: string; enabled?: boolean; config?: Record<string, unknown> }>;
};

export type UpstreamTarget = {
  id: string;
  address: string;
  weight: number;
};

export type Upstream = {
  id: string;
  name: string;
  algorithm: string;
  targets: UpstreamTarget[];
  health_check?: Record<string, unknown>;
};

export type User = {
  id: string;
  email: string;
  name: string;
  company?: string;
  role: string;
  status: string;
  credit_balance: number;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type UserListResponse = {
  users: User[];
  total: number;
};

export type CreditOverview = {
  total_distributed: number;
  total_consumed: number;
  top_consumers: Array<{
    user_id: string;
    email: string;
    name: string;
    consumed: number;
  }>;
};

export type CreditTransaction = {
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

export type CreditTransactionList = {
  transactions: CreditTransaction[];
  total: number;
};

export type AuditEntry = {
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

export type AuditListResponse = {
  entries: AuditEntry[];
  total: number;
};

export type AnalyticsOverview = {
  from: string;
  to: string;
  total_requests: number;
  active_conns: number;
  error_rate: number;
  avg_latency_ms: number;
  credits_consumed: number;
};

export type AnalyticsTimeseriesBucket = {
  timestamp: string;
  requests: number;
  errors: number;
  avg_latency_ms: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  status_codes: Record<string, number>;
  bytes_in: number;
  bytes_out: number;
  credits_consumed: number;
};

export type AnalyticsTimeseries = {
  from: string;
  to: string;
  granularity: string;
  items: AnalyticsTimeseriesBucket[];
};

export type TopRoute = {
  route_id: string;
  route_name: string;
  count: number;
};

export type AnalyticsTopRoutes = {
  from: string;
  to: string;
  limit: number;
  routes: TopRoute[];
};
