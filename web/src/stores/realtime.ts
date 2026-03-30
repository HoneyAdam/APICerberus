import { create } from "zustand";
import { ReconnectingWebSocketClient, type WebSocketStatus } from "@/lib/ws";

const MAX_EVENTS = 200;

export type RealtimeEvent = {
  type: string;
  timestamp: string;
  payload: unknown;
};

type RealtimeState = {
  status: WebSocketStatus;
  connected: boolean;
  error: string | null;
  lastMessageAt: string | null;
  events: RealtimeEvent[];
  connect: (url?: string) => void;
  disconnect: () => void;
  clear: () => void;
};

let client: ReconnectingWebSocketClient<unknown> | null = null;
let unsubscribeMessage: (() => void) | null = null;
let unsubscribeStatus: (() => void) | null = null;
let unsubscribeError: (() => void) | null = null;

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

function subscribeClient(set: (updater: (state: RealtimeState) => Partial<RealtimeState>) => void, url?: string) {
  if (!client) {
    client = new ReconnectingWebSocketClient({ url });
  }

  if (!unsubscribeMessage) {
    unsubscribeMessage = client.subscribe((message) => {
      const event = toRealtimeEvent(message);
      set((state) => ({
        events: [event, ...state.events].slice(0, MAX_EVENTS),
        lastMessageAt: event.timestamp,
        error: null,
      }));
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
  events: [],

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
      lastMessageAt: null,
      error: null,
    }));
  },
}));

