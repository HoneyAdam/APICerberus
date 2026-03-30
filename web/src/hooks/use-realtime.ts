import { useEffect, useMemo } from "react";
import { useRealtimeStore } from "@/stores/realtime";

type UseRealtimeOptions = {
  autoConnect?: boolean;
  url?: string;
  requestTailSize?: number;
  eventTailSize?: number;
};

export function useRealtime(options: UseRealtimeOptions = {}) {
  const {
    autoConnect = true,
    url,
    requestTailSize = 24,
    eventTailSize = 80,
  } = options;

  const status = useRealtimeStore((state) => state.status);
  const connected = useRealtimeStore((state) => state.connected);
  const error = useRealtimeStore((state) => state.error);
  const lastMessageAt = useRealtimeStore((state) => state.lastMessageAt);
  const events = useRealtimeStore((state) => state.events);
  const requestMetrics = useRealtimeStore((state) => state.requestMetrics);
  const trafficSeries = useRealtimeStore((state) => state.trafficSeries);
  const healthByTarget = useRealtimeStore((state) => state.healthByTarget);
  const connect = useRealtimeStore((state) => state.connect);
  const disconnect = useRealtimeStore((state) => state.disconnect);
  const clear = useRealtimeStore((state) => state.clear);

  useEffect(() => {
    if (!autoConnect) {
      return;
    }
    connect(url);
    return () => {
      disconnect();
    };
  }, [autoConnect, connect, disconnect, url]);

  const requestTail = useMemo(
    () => requestMetrics.slice(Math.max(0, requestMetrics.length - requestTailSize)),
    [requestMetrics, requestTailSize],
  );

  const eventTail = useMemo(
    () => events.slice(Math.max(0, events.length - eventTailSize)),
    [events, eventTailSize],
  );

  return {
    status,
    connected,
    error,
    lastMessageAt,
    events,
    eventTail,
    requestMetrics,
    requestTail,
    trafficSeries,
    healthByTarget,
    connect,
    disconnect,
    clear,
  };
}
