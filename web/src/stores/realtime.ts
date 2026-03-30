import { create } from "zustand";
import { ReconnectingWebSocketClient, type WebSocketStatus } from "@/lib/ws";

const MAX_EVENTS = 300;
const MAX_REQUEST_METRICS = 400;
const MAX_TRAFFIC_POINTS = 480;

export type RealtimeEvent = {
  type: string;
  timestamp: string;
  payload: unknown;
};

export type RealtimeRequestMetric = {
  timestamp: string;
  route_id: string;
  route_name: string;
  service_name: string;
  user_id: string;
  method: string;
  path: string;
  status_code: number;
  latency_ms: number;
  bytes_in: number;
  bytes_out: number;
  credits_consumed: number;
  blocked: boolean;
  error: boolean;
};

export type RealtimeHealthChange = {
  upstream_id: string;
  upstream_name: string;
  target_id: string;
  healthy: boolean;
};

export type RealtimeTrafficPoint = {
  timestamp: string;
  requests: number;
  errors: number;
};

type RealtimeState = {
  status: WebSocketStatus;
  connected: boolean;
  error: string | null;
  lastMessageAt: string | null;
  connectedAt: string | null;
  events: RealtimeEvent[];
  requestMetrics: RealtimeRequestMetric[];
  trafficSeries: RealtimeTrafficPoint[];
  healthByTarget: Record<string, boolean>;
  connect: (url?: string) => void;
  disconnect: () => void;
  clear: () => void;
};

let client: ReconnectingWebSocketClient<unknown> | null = null;
let unsubscribeMessage: (() => void) | null = null;
let unsubscribeStatus: (() => void) | null = null;
let unsubscribeError: (() => void) | null = null;

function asString(value: unknown) {
  return typeof value === "string" ? value : "";
}

function asNumber(value: unknown) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return 0;
}

function asBoolean(value: unknown) {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    return normalized === "1" || normalized === "true" || normalized === "yes";
  }
  return false;
}

function appendTrimmed<T>(items: T[], item: T, maxItems: number) {
  const next = [...items, item];
  if (next.length <= maxItems) {
    return next;
  }
  return next.slice(next.length - maxItems);
}

function minuteBucket(timestamp: string) {
  const date = new Date(timestamp);
  const epoch = date.getTime();
  if (!Number.isFinite(epoch)) {
    return null;
  }
  date.setSeconds(0, 0);
  return date.toISOString();
}

function appendTrafficPoint(items: RealtimeTrafficPoint[], metric: RealtimeRequestMetric) {
  const key = minuteBucket(metric.timestamp);
  if (!key) {
    return items;
  }

  const next = [...items];
  const index = next.findIndex((item) => item.timestamp === key);
  if (index >= 0) {
    next[index] = {
      ...next[index],
      requests: next[index].requests + 1,
      errors: next[index].errors + (metric.error || metric.status_code >= 500 ? 1 : 0),
    };
  } else {
    next.push({
      timestamp: key,
      requests: 1,
      errors: metric.error || metric.status_code >= 500 ? 1 : 0,
    });
    next.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
  }

  if (next.length <= MAX_TRAFFIC_POINTS) {
    return next;
  }
  return next.slice(next.length - MAX_TRAFFIC_POINTS);
}

function toRealtimeEvent(input: unknown): RealtimeEvent {
  const now = new Date().toISOString();
  if (typeof input === "object" && input !== null) {
    const record = input as Record<string, unknown>;
    const type = typeof record.type === "string" ? record.type : "message";
    const timestamp = typeof record.timestamp === "string" ? record.timestamp : now;
    const payload = "payload" in record ? record.payload : record;
    return { type, timestamp, payload };
  }
  return {
    type: "message",
    timestamp: now,
    payload: input,
  };
}

function normalizeRequestMetric(payload: unknown): RealtimeRequestMetric | null {
  if (typeof payload !== "object" || payload === null) {
    return null;
  }
  const item = payload as Record<string, unknown>;
  const timestamp = asString(item.timestamp) || new Date().toISOString();
  return {
    timestamp,
    route_id: asString(item.route_id),
    route_name: asString(item.route_name),
    service_name: asString(item.service_name),
    user_id: asString(item.user_id),
    method: asString(item.method),
    path: asString(item.path),
    status_code: Math.round(asNumber(item.status_code)),
    latency_ms: Math.round(asNumber(item.latency_ms)),
    bytes_in: Math.round(asNumber(item.bytes_in)),
    bytes_out: Math.round(asNumber(item.bytes_out)),
    credits_consumed: Math.round(asNumber(item.credits_consumed)),
    blocked: asBoolean(item.blocked),
    error: asBoolean(item.error),
  };
}

function normalizeHealthChange(payload: unknown): RealtimeHealthChange | null {
  if (typeof payload !== "object" || payload === null) {
    return null;
  }
  const item = payload as Record<string, unknown>;
  const targetID = asString(item.target_id);
  if (!targetID) {
    return null;
  }
  return {
    upstream_id: asString(item.upstream_id),
    upstream_name: asString(item.upstream_name),
    target_id: targetID,
    healthy: asBoolean(item.healthy),
  };
}

function shouldUseMetric(metric: RealtimeRequestMetric, connectedAt: string | null) {
  if (!connectedAt) {
    return true;
  }
  const metricTS = new Date(metric.timestamp).getTime();
  const connectedTS = new Date(connectedAt).getTime();
  if (!Number.isFinite(metricTS) || !Number.isFinite(connectedTS)) {
    return true;
  }
  return metricTS + 500 >= connectedTS;
}

function subscribeClient(set: (updater: (state: RealtimeState) => Partial<RealtimeState>) => void, url?: string) {
  if (!client) {
    client = new ReconnectingWebSocketClient({ url });
  }

  if (!unsubscribeMessage) {
    unsubscribeMessage = client.subscribe((message) => {
      const event = toRealtimeEvent(message);
      set((state) => {
        const next: Partial<RealtimeState> = {
          lastMessageAt: event.timestamp,
          error: null,
        };

        if (event.type === "connected") {
          next.connectedAt = event.timestamp;
          next.events = appendTrimmed(state.events, event, MAX_EVENTS);
          return next;
        }

        if (event.type === "request_metric") {
          const metric = normalizeRequestMetric(event.payload);
          if (metric && shouldUseMetric(metric, state.connectedAt)) {
            next.events = appendTrimmed(state.events, event, MAX_EVENTS);
            next.requestMetrics = appendTrimmed(state.requestMetrics, metric, MAX_REQUEST_METRICS);
            next.trafficSeries = appendTrafficPoint(state.trafficSeries, metric);
            return next;
          }
          return next;
        }

        if (event.type === "health_change") {
          const change = normalizeHealthChange(event.payload);
          if (change) {
            next.events = appendTrimmed(state.events, event, MAX_EVENTS);
            next.healthByTarget = {
              ...state.healthByTarget,
              [`${change.upstream_id || change.upstream_name}:${change.target_id}`]: change.healthy,
            };
            return next;
          }
          return next;
        }

        next.events = appendTrimmed(state.events, event, MAX_EVENTS);
        return next;
      });
    });
  }

  if (!unsubscribeStatus) {
    unsubscribeStatus = client.onStatusChange((status) => {
      set(() => ({
        status,
        connected: status === "open",
      }));
    });
  }

  if (!unsubscribeError) {
    unsubscribeError = client.onError(() => {
      set(() => ({
        error: "WebSocket connection error",
      }));
    });
  }
}

function clearClientSubscriptions() {
  unsubscribeMessage?.();
  unsubscribeStatus?.();
  unsubscribeError?.();
  unsubscribeMessage = null;
  unsubscribeStatus = null;
  unsubscribeError = null;
}

export const useRealtimeStore = create<RealtimeState>((set) => ({
  status: "idle",
  connected: false,
  error: null,
  lastMessageAt: null,
  connectedAt: null,
  events: [],
  requestMetrics: [],
  trafficSeries: [],
  healthByTarget: {},

  connect: (url) => {
    subscribeClient(set, url);
    client?.connect();
  },

  disconnect: () => {
    client?.disconnect(1000, "Client disconnect");
    clearClientSubscriptions();
    client = null;
    set(() => ({
      status: "closed",
      connected: false,
    }));
  },

  clear: () => {
    set(() => ({
      events: [],
      requestMetrics: [],
      trafficSeries: [],
      healthByTarget: {},
      lastMessageAt: null,
      connectedAt: null,
      error: null,
    }));
  },
}));
