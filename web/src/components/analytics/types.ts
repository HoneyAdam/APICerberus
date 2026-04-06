// Types for analytics components

export type GeoDataPoint = {
  country: string;
  countryCode: string;
  region?: string;
  city?: string;
  requests: number;
  errors: number;
  avgLatencyMs: number;
  uniqueIps: number;
};

export type RateLimitBucket = {
  window: string;
  allowed: number;
  throttled: number;
  blocked: number;
  avgLatencyMs: number;
};

export type RateLimitStats = {
  totalRequests: number;
  allowed: number;
  throttled: number;
  blocked: number;
  byWindow: RateLimitBucket[];
  byRoute: Array<{
    routeId: string;
    routeName: string;
    allowed: number;
    throttled: number;
    blocked: number;
  }>;
  byConsumer: Array<{
    consumerId: string;
    consumerName: string;
    allowed: number;
    throttled: number;
    blocked: number;
  }>;
};
