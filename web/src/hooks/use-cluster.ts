import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

export type ClusterNodeRole = "leader" | "follower" | "candidate" | "unhealthy" | "standalone";

export interface ClusterNode {
  id: string;
  name: string;
  address: string;
  role: ClusterNodeRole;
  state: "healthy" | "unhealthy" | "joining" | "leaving";
  lastSeen: string;
  metadata?: Record<string, unknown>;
}

export interface ClusterEdge {
  from: string;
  to: string;
  type: "raft" | "rpc" | "heartbeat";
  status: "connected" | "disconnected" | "lagging";
  latencyMs?: number;
}

export interface ClusterStatus {
  enabled: boolean;
  mode: "standalone" | "raft";
  nodeId: string;
  leaderId?: string;
  term: number;
  commitIndex: number;
  appliedIndex: number;
  nodes: ClusterNode[];
  edges: ClusterEdge[];
}

const CLUSTER_QUERY_KEY = "cluster";

// Placeholder data for standalone mode
const STANDALONE_STATUS: ClusterStatus = {
  enabled: false,
  mode: "standalone",
  nodeId: "local",
  term: 0,
  commitIndex: 0,
  appliedIndex: 0,
  nodes: [
    {
      id: "local",
      name: "Local Node",
      address: "127.0.0.1:8080",
      role: "standalone",
      state: "healthy",
      lastSeen: new Date().toISOString(),
    },
  ],
  edges: [],
};

async function fetchClusterStatus(): Promise<ClusterStatus> {
  try {
    const response = await fetch("/admin/api/v1/cluster/status", {
      credentials: "same-origin",
    });

    if (!response.ok) {
      // Fallback to standalone if endpoint doesn't exist
      if (response.status === 404) {
        return STANDALONE_STATUS;
      }
      throw new Error(`Failed to fetch cluster status: ${response.statusText}`);
    }

    return response.json();
  } catch (error) {
    // Return standalone mode as fallback
    console.warn("Cluster API not available, using standalone mode", error);
    return STANDALONE_STATUS;
  }
}

export function useClusterStatus() {
  return useQuery({
    queryKey: [CLUSTER_QUERY_KEY],
    queryFn: fetchClusterStatus,
    refetchInterval: 5000, // Refresh every 5 seconds
    staleTime: 3000,
  });
}

export function useClusterRealtime() {
  const [status, setStatus] = useState<ClusterStatus>(STANDALONE_STATUS);
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout>;

    const connect = () => {
      const wsUrl = `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/admin/api/v1/ws`;

      try {
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
          setIsConnected(true);
          // Subscribe to cluster events
          ws?.send(JSON.stringify({ action: "subscribe", channel: "cluster" }));
        };

        ws.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data);
            if (message.type === "cluster" && message.payload) {
              setStatus((prev) => ({
                ...prev,
                ...message.payload,
              }));
            }
          } catch {
            // Ignore parse errors
          }
        };

        ws.onclose = () => {
          setIsConnected(false);
          // Reconnect after 3 seconds
          reconnectTimer = setTimeout(connect, 3000);
        };

        ws.onerror = () => {
          setIsConnected(false);
        };
      } catch {
        setIsConnected(false);
      }
    };

    connect();

    return () => {
      clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, []);

  return { status, isConnected };
}
