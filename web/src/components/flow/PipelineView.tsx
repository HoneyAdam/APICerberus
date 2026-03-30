import { useMemo } from "react";
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { cn } from "@/lib/utils";
import { flowEdgeTypes } from "./edges";
import { flowNodeTypes } from "./nodes";

export type PipelinePlugin = {
  name: string;
  enabled: boolean;
  config?: Record<string, unknown>;
};

type PipelineViewProps = {
  className?: string;
  routeName: string;
  serviceName: string;
  upstreamName?: string;
  plugins: PipelinePlugin[];
  onEditPlugin?: (pluginName: string) => void;
};

const phaseOrder: Record<string, number> = {
  preauth: 1,
  auth: 2,
  preproxy: 3,
  proxy: 4,
  postproxy: 5,
};

const pluginPhaseMap: Record<string, string> = {
  cors: "PreAuth",
  "correlation-id": "PreAuth",
  "bot-detect": "PreAuth",
  "ip-restrict": "PreAuth",
  "auth-apikey": "Auth",
  "auth-jwt": "Auth",
  "user-ip-whitelist": "Auth",
  "endpoint-permission": "Auth",
  "rate-limit": "PreProxy",
  "request-size-limit": "PreProxy",
  "request-validator": "PreProxy",
  "url-rewrite": "PreProxy",
  "request-transform": "PreProxy",
  "circuit-breaker": "Proxy",
  retry: "Proxy",
  timeout: "Proxy",
  "response-transform": "PostProxy",
  compression: "PostProxy",
  redirect: "PostProxy",
};

function normalizePhase(value: string) {
  return value.replace(/[_\s-]+/g, "").toLowerCase();
}

function normalizePluginName(name: string) {
  return name.trim().toLowerCase().replace(/[_\s]+/g, "-");
}

function pluginPhase(plugin: PipelinePlugin) {
  const configPhase = plugin.config?.phase;
  if (typeof configPhase === "string" && configPhase.trim().length > 0) {
    return configPhase.trim();
  }
  return pluginPhaseMap[normalizePluginName(plugin.name)] ?? "PreProxy";
}

function pluginPriority(plugin: PipelinePlugin) {
  const configPriority = plugin.config?.priority;
  if (typeof configPriority === "number" && Number.isFinite(configPriority)) {
    return configPriority;
  }
  if (typeof configPriority === "string") {
    const parsed = Number(configPriority);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return 50;
}

function formatConfigValue(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.length}]`;
  }
  if (value && typeof value === "object") {
    return "{...}";
  }
  if (typeof value === "string") {
    return value.length > 24 ? `${value.slice(0, 24)}...` : value;
  }
  return String(value);
}

function summarizeConfig(config?: Record<string, unknown>) {
  if (!config) {
    return "No config";
  }

  const entries = Object.entries(config).filter(([key]) => key !== "phase" && key !== "priority");
  if (!entries.length) {
    return "Custom config";
  }

  const preview = entries
    .slice(0, 2)
    .map(([key, value]) => `${key}: ${formatConfigValue(value)}`)
    .join(" | ");

  const overflow = entries.length - 2;
  return overflow > 0 ? `${preview} +${overflow} more` : preview;
}

function toneForPlugin(plugin: PipelinePlugin, phase: string): "default" | "success" | "warning" | "danger" {
  if (!plugin.enabled) {
    return "danger";
  }
  const normalized = normalizePhase(phase);
  if (normalized === "auth" || normalized === "preauth") {
    return "success";
  }
  if (normalized === "proxy") {
    return "warning";
  }
  return "default";
}

export function PipelineView({
  className,
  routeName,
  serviceName,
  upstreamName,
  plugins,
  onEditPlugin,
}: PipelineViewProps) {
  const orderedPlugins = useMemo(() => {
    return plugins
      .map((plugin, index) => ({
        plugin,
        index,
        phase: pluginPhase(plugin),
        priority: pluginPriority(plugin),
      }))
      .sort((a, b) => {
        const aPhase = phaseOrder[normalizePhase(a.phase)] ?? 99;
        const bPhase = phaseOrder[normalizePhase(b.phase)] ?? 99;
        if (aPhase !== bPhase) {
          return aPhase - bPhase;
        }
        if (a.priority !== b.priority) {
          return a.priority - b.priority;
        }
        return a.index - b.index;
      });
  }, [plugins]);

  const nodes = useMemo<Node[]>(() => {
    const startX = 40;
    const baseY = 170;
    const pluginGap = 220;
    const pluginStartX = startX + 220;
    const serviceX = pluginStartX + orderedPlugins.length * pluginGap;
    const upstreamX = serviceX + 260;
    const clusterX = upstreamX + 260;

    const built: Node[] = [
      {
        id: "gateway",
        type: "gatewayNode",
        position: { x: startX, y: baseY },
        draggable: false,
        data: {
          title: "Gateway",
          subtitle: "Ingress",
          meta: routeName,
          tone: "default",
        },
      },
      {
        id: "service",
        type: "serviceNode",
        position: { x: serviceX, y: baseY },
        draggable: false,
        data: {
          title: serviceName || "Service",
          subtitle: "Route target",
          meta: "service",
          tone: "default",
        },
      },
      {
        id: "upstream",
        type: "upstreamNode",
        position: { x: upstreamX, y: baseY },
        draggable: false,
        data: {
          title: upstreamName || "Upstream",
          subtitle: "Target pool",
          meta: "upstream",
          tone: "default",
        },
      },
      {
        id: "cluster",
        type: "clusterNode",
        position: { x: clusterX, y: 50 },
        draggable: false,
        data: {
          title: "Cluster",
          subtitle: "Control plane",
          meta: "heartbeat",
          tone: "warning",
        },
      },
    ];

    for (let i = 0; i < orderedPlugins.length; i += 1) {
      const entry = orderedPlugins[i];
      const nodeID = `plugin-${i}`;
      built.push({
        id: nodeID,
        type: "pluginNode",
        position: { x: pluginStartX + i * pluginGap, y: baseY },
        draggable: false,
        data: {
          title: entry.plugin.name,
          subtitle: entry.plugin.enabled ? "enabled" : "disabled",
          phase: entry.phase,
          configSummary: summarizeConfig(entry.plugin.config),
          tone: toneForPlugin(entry.plugin, entry.phase),
          onEdit: () => onEditPlugin?.(entry.plugin.name),
        },
      });
    }

    return built;
  }, [orderedPlugins, onEditPlugin, routeName, serviceName, upstreamName]);

  const edges = useMemo<Edge[]>(() => {
    const chain = ["gateway", ...orderedPlugins.map((_, index) => `plugin-${index}`), "service", "upstream"];
    const built: Edge[] = [];

    for (let i = 0; i < chain.length - 1; i += 1) {
      built.push({
        id: `traffic-${i}`,
        source: chain[i],
        target: chain[i + 1],
        type: "trafficEdge",
        markerEnd: { type: MarkerType.ArrowClosed },
      });
    }

    built.push({
      id: "raft-heartbeat",
      source: "cluster",
      target: "gateway",
      type: "raftEdge",
      markerEnd: { type: MarkerType.ArrowClosed },
    });

    return built;
  }, [orderedPlugins]);

  return (
    <div className={cn("h-[460px] w-full overflow-hidden rounded-xl border bg-card/60", className)}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={flowNodeTypes}
        edgeTypes={flowEdgeTypes}
        nodesConnectable={false}
        nodesDraggable={false}
        elementsSelectable={false}
        fitView
        fitViewOptions={{ padding: 0.1 }}
      >
        <Background gap={18} size={1} className="opacity-70" />
        <MiniMap
          zoomable
          pannable
          nodeStrokeWidth={3}
          maskColor="hsl(var(--background) / 0.65)"
          nodeColor={(node) => {
            if (node.type === "gatewayNode") {
              return "hsl(var(--primary))";
            }
            if (node.type === "pluginNode") {
              return "hsl(var(--chart-2))";
            }
            if (node.type === "clusterNode") {
              return "hsl(var(--chart-4))";
            }
            return "hsl(var(--chart-3))";
          }}
        />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
