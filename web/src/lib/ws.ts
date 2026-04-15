import { WS_CONFIG } from "./constants";

export type WebSocketStatus = "idle" | "connecting" | "open" | "reconnecting" | "closed";

type MessageListener<T> = (message: T, event: MessageEvent<string>) => void;
type StatusListener = (status: WebSocketStatus) => void;
type ErrorListener = (error: Event) => void;

function resolveWebSocketUrl(overrideUrl?: string) {
  let resolvedUrl = "";
  if (overrideUrl) {
    resolvedUrl = overrideUrl;
  } else if (WS_CONFIG.url) {
    resolvedUrl = WS_CONFIG.url;
  } else if (typeof window !== "undefined") {
    const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    resolvedUrl = `${scheme}//${host}${WS_CONFIG.path}`;
  }

  return resolvedUrl;
}

export class ReconnectingWebSocketClient<TMessage = unknown> {
  private readonly options: Required<Omit<{
    url?: string;
    protocols?: string | string[];
    reconnect?: boolean;
    reconnectInitialDelayMs?: number;
    reconnectMaxDelayMs?: number;
    reconnectBackoffMultiplier?: number;
    maxReconnectAttempts?: number;
  }, "url" | "protocols">> &
    Pick<{
      url?: string;
      protocols?: string | string[];
      reconnect?: boolean;
      reconnectInitialDelayMs?: number;
      reconnectMaxDelayMs?: number;
      reconnectBackoffMultiplier?: number;
      maxReconnectAttempts?: number;
    }, "url" | "protocols">;
  private ws: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private manualClose = false;
  private status: WebSocketStatus = "idle";
  private readonly messageListeners = new Set<MessageListener<TMessage>>();
  private readonly statusListeners = new Set<StatusListener>();
  private readonly errorListeners = new Set<ErrorListener>();

  constructor(options: {
    url?: string;
    protocols?: string | string[];
    reconnect?: boolean;
    reconnectInitialDelayMs?: number;
    reconnectMaxDelayMs?: number;
    reconnectBackoffMultiplier?: number;
    maxReconnectAttempts?: number;
  } = {}) {
    this.options = {
      reconnect: true,
      reconnectInitialDelayMs: WS_CONFIG.reconnectInitialDelayMs,
      reconnectMaxDelayMs: WS_CONFIG.reconnectMaxDelayMs,
      reconnectBackoffMultiplier: WS_CONFIG.reconnectBackoffMultiplier,
      maxReconnectAttempts: Infinity,
      ...options,
    };
  }

  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    const url = resolveWebSocketUrl(this.options.url);
    if (!url) {
      this.setStatus("closed");
      return;
    }

    // M-023: WebSocket origin validation.
    // NOTE: In browser contexts, WebSocket connections are subject to the Same-Origin Policy.
    // The APICerebrus gateway should validate the Origin header on WebSocket upgrade requests
    // and reject connections from untrusted origins. The admin API at /admin/api/v1/ws should
    // enforce origin checking — only allow origins that match the configured admin UI URL.
    // Cross-origin WebSocket connections from untrusted sites could be exploited for CSRF-style
    // attacks or to exfiltrate data via crafted WebSocket messages.

    this.clearReconnectTimer();
    this.manualClose = false;
    this.setStatus(this.reconnectAttempt > 0 ? "reconnecting" : "connecting");

    this.ws = new WebSocket(url, this.options.protocols);
    this.ws.addEventListener("open", this.handleOpen);
    this.ws.addEventListener("message", this.handleMessage);
    this.ws.addEventListener("error", this.handleError);
    this.ws.addEventListener("close", this.handleClose);
  }

  disconnect(code?: number, reason?: string) {
    this.manualClose = true;
    this.clearReconnectTimer();
    if (!this.ws) {
      this.setStatus("closed");
      return;
    }

    this.ws.removeEventListener("open", this.handleOpen);
    this.ws.removeEventListener("message", this.handleMessage);
    this.ws.removeEventListener("error", this.handleError);
    this.ws.removeEventListener("close", this.handleClose);
    this.ws.close(code, reason);
    this.ws = null;
    this.setStatus("closed");
  }

  send(data: string | object) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return false;
    }
    this.ws.send(typeof data === "string" ? data : JSON.stringify(data));
    return true;
  }

  subscribe(listener: MessageListener<TMessage>) {
    this.messageListeners.add(listener);
    return () => {
      this.messageListeners.delete(listener);
    };
  }

  onStatusChange(listener: StatusListener) {
    this.statusListeners.add(listener);
    listener(this.status);
    return () => {
      this.statusListeners.delete(listener);
    };
  }

  onError(listener: ErrorListener) {
    this.errorListeners.add(listener);
    return () => {
      this.errorListeners.delete(listener);
    };
  }

  getStatus() {
    return this.status;
  }

  private readonly handleOpen = () => {
    this.reconnectAttempt = 0;
    this.setStatus("open");
  };

  private readonly handleMessage = (event: MessageEvent<string>) => {
    let payload: unknown = event.data;
    if (typeof event.data === "string") {
      try {
        payload = JSON.parse(event.data);
      } catch {
        payload = event.data;
      }
    }
    for (const listener of this.messageListeners) {
      listener(payload as TMessage, event);
    }
  };

  private readonly handleError = (event: Event) => {
    for (const listener of this.errorListeners) {
      listener(event);
    }
  };

  private readonly handleClose = () => {
    this.ws?.removeEventListener("open", this.handleOpen);
    this.ws?.removeEventListener("message", this.handleMessage);
    this.ws?.removeEventListener("error", this.handleError);
    this.ws?.removeEventListener("close", this.handleClose);
    this.ws = null;

    if (this.manualClose || !this.options.reconnect) {
      this.setStatus("closed");
      return;
    }
    if (this.reconnectAttempt >= this.options.maxReconnectAttempts) {
      this.setStatus("closed");
      return;
    }

    this.reconnectAttempt += 1;
    const delay = Math.min(
      this.options.reconnectMaxDelayMs,
      this.options.reconnectInitialDelayMs * this.options.reconnectBackoffMultiplier ** (this.reconnectAttempt - 1),
    );
    this.setStatus("reconnecting");
    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  };

  private clearReconnectTimer() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private setStatus(nextStatus: WebSocketStatus) {
    this.status = nextStatus;
    for (const listener of this.statusListeners) {
      listener(nextStatus);
    }
  }
}
