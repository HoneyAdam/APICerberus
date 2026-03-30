import {
  Background,
  MarkerType,
  ReactFlow,
  type Edge,
  type Node,
  type NodeMouseHandler,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Badge } from "@/components/ui/badge";
import { flowEdgeTypes } from "./edges";

export type HealthStatus = "healthy" | "degraded" | "down";

export type UpstreamMapHistoryPoint = {
  timestamp: string;
  latencyMS: number;
  healthy: boolean;
};

export type UpstreamMapTarget = {
  id: string;
  address: string;
  weight: number;
  status: HealthStatus;
  trafficRPS: number;
  latencyMS: number;
  history: UpstreamMapHistoryPoint[];
};

type UpstreamMapProps = {
  upstreamName: string;
  targets: UpstreamMapTarget[];
  selectedTargetID?: string;
  onTargetSelect?: (targetID: string) => void;
};

function statusColors(status: HealthStatus) {
  switch (status) {
    case "healthy":
      return {
        border: "hsl(var(--success) / 0.65)",
        background: "hsl(var(--success) / 0.14)",
        text: "hsl(var(--foreground))",
      };
    case "degraded":
      return {
        border: "hsl(var(--warning) / 0.7)",
        background: "hsl(var(--warning) / 0.16)",
        text: "hsl(var(--foreground))",
      };
    case "down":
      return {
        border: "hsl(var(--destructive) / 0.8)",
        background: "hsl(var(--destructive) / 0.14)",
        text: "hsl(var(--foreground))",
      };
    default:
      return {
        border: "hsl(var(--border))",
        background: "hsl(var(--card))",
        text: "hsl(var(--foreground))",
      };
  }
}

function targetNodeID(targetID: string) {
  return `target-${targetID}`;
}

export function UpstreamMap({ upstreamName, targets, selectedTargetID, onTargetSelect }: UpstreamMapProps) {
  const centerX = 390;
  const centerY = 220;
  const radius = Math.min(250, Math.max(160, targets.length * 26));
  const maxTraffic = Math.max(1, ...targets.map((target) => target.trafficRPS));

  const nodes: Node[] = [
    {
      id: "gateway",
      position: { x: centerX, y: centerY },
      draggable: false,
      data: {
        label: (
          <div className="space-y-1 text-center">
            <p className="text-sm font-semibold">Gateway</p>
            <p className="text-[11px] text-muted-foreground">{upstreamName || "upstream"}</p>
          </div>
        ),
      },
      style: {
        width: 180,
        borderRadius: 14,
        border: "1px solid hsl(var(--primary) / 0.75)",
        background: "hsl(var(--primary) / 0.12)",
        color: "hsl(var(--foreground))",
        boxShadow: "0 8px 28px rgb(0 0 0 / 10%)",
      },
    },
  ];

  const edges: Edge[] = [];

  for (let i = 0; i < targets.length; i += 1) {
    const target = targets[i];
    const angle = (i / Math.max(1, targets.length)) * Math.PI * 2 - Math.PI / 2;
    const x = centerX + Math.cos(angle) * radius;
    const y = centerY + Math.sin(angle) * radius;
    const colors = statusColors(target.status);
    const isSelected = selectedTargetID === target.id;

    nodes.push({
      id: targetNodeID(target.id),
      position: { x, y },
      draggable: false,
      data: {
        label: (
          <div className="space-y-1">
            <p className="text-xs font-semibold">{target.address}</p>
            <div className="flex items-center gap-1">
              <Badge variant="outline" className="h-5 rounded-full px-1.5 text-[10px] uppercase">
                {target.status}
              </Badge>
              <span className="text-[10px] text-muted-foreground">{target.latencyMS}ms</span>
            </div>
          </div>
        ),
      },
      style: {
        width: 196,
        borderRadius: 14,
        border: `1px solid ${isSelected ? "hsl(var(--primary))" : colors.border}`,
        background: colors.background,
        color: colors.text,
        boxShadow: isSelected ? "0 0 0 1px hsl(var(--primary) / 0.35)" : "none",
        cursor: "pointer",
      },
    });

    edges.push({
      id: `edge-gateway-${target.id}`,
      source: "gateway",
      target: targetNodeID(target.id),
      type: "trafficEdge",
      markerEnd: { type: MarkerType.ArrowClosed },
      style: {
        strokeWidth: 1.2 + (target.trafficRPS / maxTraffic) * 4.2,
        opacity: 0.95,
      },
    });
  }

  const handleNodeClick: NodeMouseHandler = (_event, node) => {
    if (!node.id.startsWith("target-")) {
      return;
    }
    onTargetSelect?.(node.id.slice("target-".length));
  };

  return (
    <div className="relative h-[520px] w-full overflow-hidden rounded-xl border bg-card/60">
      <div className="absolute left-3 top-3 z-10 rounded-lg border bg-background/85 px-2 py-1 text-[11px] text-muted-foreground">
        <p>Edge thickness = traffic volume</p>
      </div>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        edgeTypes={flowEdgeTypes}
        onNodeClick={handleNodeClick}
        nodesConnectable={false}
        nodesDraggable={false}
        elementsSelectable={false}
        fitView
        fitViewOptions={{ padding: 0.12 }}
      >
        <Background gap={16} size={1} className="opacity-65" />
      </ReactFlow>
    </div>
  );
}
