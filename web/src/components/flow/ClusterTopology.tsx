import { memo } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Badge } from "@/components/ui/badge";
import type { ClusterNodeRole } from "@/hooks/use-cluster";

export type ClusterMember = {
  id: string;
  name: string;
  role: ClusterNodeRole;
  address?: string;
  state?: string;
  lastSeen?: string;
};

type ClusterEdgeData = {
  from: string;
  to: string;
  type: "raft" | "rpc" | "heartbeat";
  status: "connected" | "disconnected" | "lagging";
  latencyMs?: number;
};

type ClusterTopologyProps = {
  members?: ClusterMember[];
  edges?: ClusterEdgeData[];
  mode?: "standalone" | "raft";
};

type ClusterNodeData = {
  title: string;
  subtitle: string;
  role: ClusterNodeRole;
  address?: string;
  state?: string;
  lastSeen?: string;
  isLeader?: boolean;
};

function roleClass(role: ClusterNodeRole) {
  switch (role) {
    case "leader":
      return "border-sky-500/60 bg-sky-500/12";
    case "follower":
      return "border-emerald-500/55 bg-emerald-500/10";
    case "candidate":
      return "border-amber-500/60 bg-amber-500/12";
    case "unhealthy":
      return "border-destructive/70 bg-destructive/12";
    case "standalone":
    default:
      return "border-border bg-card";
  }
}

function roleClassBadge(role: ClusterNodeRole) {
  switch (role) {
    case "leader":
      return "bg-sky-500/20 text-sky-700 border-sky-500/30";
    case "follower":
      return "bg-emerald-500/20 text-emerald-700 border-emerald-500/30";
    case "candidate":
      return "bg-amber-500/20 text-amber-700 border-amber-500/30";
    case "unhealthy":
      return "bg-destructive/20 text-destructive border-destructive/30";
    case "standalone":
    default:
      return "bg-muted text-muted-foreground";
  }
}

function roleLabel(role: ClusterNodeRole) {
  switch (role) {
    case "leader":
      return "Leader";
    case "follower":
      return "Follower";
    case "candidate":
      return "Candidate";
    case "unhealthy":
      return "Unhealthy";
    case "standalone":
    default:
      return "Standalone";
  }
}

function stateClass(state?: string) {
  switch (state) {
    case "healthy":
      return "text-emerald-600";
    case "unhealthy":
      return "text-destructive";
    case "joining":
    case "leaving":
      return "text-amber-600";
    default:
      return "text-muted-foreground";
  }
}

function ClusterRoleNode({ data }: NodeProps) {
  const nodeData = data as ClusterNodeData;
  return (
    <div className={`min-w-[240px] rounded-xl border p-4 shadow-sm ${roleClass(nodeData.role)}`}>
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-semibold">{nodeData.title}</p>
        <Badge variant="outline" className={`h-5 rounded-full px-2 text-[10px] uppercase tracking-wide ${roleClassBadge(nodeData.role)}`}>
          {roleLabel(nodeData.role)}
        </Badge>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{nodeData.subtitle}</p>
      {nodeData.address ? (
        <p className="mt-2 text-[11px] font-mono text-muted-foreground">{nodeData.address}</p>
      ) : null}
      {nodeData.state ? (
        <p className="mt-1 text-[11px] flex items-center gap-1">
          <span className={`inline-block w-1.5 h-1.5 rounded-full ${stateClass(nodeData.state).replace("text-", "bg-")}`} />
          <span className={stateClass(nodeData.state)}>{nodeData.state}</span>
        </p>
      ) : null}
      {nodeData.lastSeen ? (
        <p className="mt-1 text-[10px] text-muted-foreground">
          Last seen: {new Date(nodeData.lastSeen).toLocaleTimeString()}
        </p>
      ) : null}
    </div>
  );
}

const clusterNodeTypes: NodeTypes = {
  clusterLeaderNode: memo(ClusterRoleNode),
  clusterFollowerNode: memo(ClusterRoleNode),
  clusterCandidateNode: memo(ClusterRoleNode),
  clusterUnhealthyNode: memo(ClusterRoleNode),
  clusterStandaloneNode: memo(ClusterRoleNode),
};

function nodeTypeForRole(role: ClusterNodeRole): keyof typeof clusterNodeTypes {
  switch (role) {
    case "leader":
      return "clusterLeaderNode";
    case "follower":
      return "clusterFollowerNode";
    case "candidate":
      return "clusterCandidateNode";
    case "unhealthy":
      return "clusterUnhealthyNode";
    case "standalone":
    default:
      return "clusterStandaloneNode";
  }
}

export function ClusterTopology({ members = [], edges = [], mode = "standalone" }: ClusterTopologyProps) {
  const nodes: Node[] = [];
  const flowEdges: Edge[] = [];

  if (!members.length) {
    nodes.push({
      id: "cluster-standalone",
      type: "clusterStandaloneNode",
      position: { x: 320, y: 200 },
      draggable: false,
      data: {
        title: "Local Node",
        subtitle: "Standalone gateway mode",
        role: "standalone" as ClusterNodeRole,
        state: "healthy",
      } satisfies ClusterNodeData,
    });
  } else {
    // Calculate positions based on mode
    const centerX = 400;
    const centerY = 280;

    if (mode === "standalone" || members.length === 1) {
      // Single node in center
      const member = members[0];
      nodes.push({
        id: `cluster-${member.id}`,
        type: nodeTypeForRole(member.role),
        position: { x: centerX, y: centerY },
        draggable: false,
        data: {
          title: member.name,
          subtitle: "Standalone gateway",
          role: member.role,
          address: member.address,
          state: member.state,
          lastSeen: member.lastSeen,
        } satisfies ClusterNodeData,
      });
    } else {
      // Raft cluster layout - leader in center, followers in circle
      const leader = members.find(m => m.role === "leader");
      const others = members.filter(m => m.role !== "leader");

      // Place leader in center
      if (leader) {
        nodes.push({
          id: `cluster-${leader.id}`,
          type: nodeTypeForRole(leader.role),
          position: { x: centerX, y: centerY },
          draggable: false,
          data: {
            title: leader.name,
            subtitle: "Raft Leader",
            role: leader.role,
            address: leader.address,
            state: leader.state,
            lastSeen: leader.lastSeen,
            isLeader: true,
          } satisfies ClusterNodeData,
        });
      }

      // Place others in circle
      const radius = Math.max(180, others.length * 30);
      const startAngle = leader ? 0 : -Math.PI / 2;

      others.forEach((member, index) => {
        const angleOffset = leader ? 1 : 0; // Skip first position if leader exists
        const angle = startAngle + ((index + angleOffset) / Math.max(1, members.length)) * Math.PI * 2;
        const x = centerX + Math.cos(angle) * radius;
        const y = centerY + Math.sin(angle) * radius;

        nodes.push({
          id: `cluster-${member.id}`,
          type: nodeTypeForRole(member.role),
          position: { x, y },
          draggable: false,
          data: {
            title: member.name,
            subtitle: member.role === "candidate" ? "Candidate" : "Follower",
            role: member.role,
            address: member.address,
            state: member.state,
            lastSeen: member.lastSeen,
          } satisfies ClusterNodeData,
        });

        // Add edge to leader
        if (leader) {
          flowEdges.push({
            id: `edge-${member.id}-${leader.id}`,
            source: `cluster-${member.id}`,
            target: `cluster-${leader.id}`,
            type: "smoothstep",
            animated: member.state === "healthy",
            style: {
              stroke: member.state === "healthy" ? "#10b981" : member.state === "unhealthy" ? "#ef4444" : "#f59e0b",
              strokeWidth: 2,
            },
            label: edges.find(e => (e.from === member.id && e.to === leader.id) || (e.from === leader.id && e.to === member.id))?.latencyMs
              ? `${edges.find(e => (e.from === member.id && e.to === leader.id) || (e.from === leader.id && e.to === member.id))?.latencyMs}ms`
              : undefined,
          });
        }
      });
    }
  }

  // Add custom edges from props
  edges.forEach(edge => {
    if (!flowEdges.find(e =>
      (e.source === `cluster-${edge.from}` && e.target === `cluster-${edge.to}`) ||
      (e.source === `cluster-${edge.to}` && e.target === `cluster-${edge.from}`)
    )) {
      flowEdges.push({
        id: `edge-${edge.from}-${edge.to}`,
        source: `cluster-${edge.from}`,
        target: `cluster-${edge.to}`,
        type: "smoothstep",
        animated: edge.status === "connected",
        style: {
          stroke: edge.status === "connected" ? "#0ea5e9" : edge.status === "disconnected" ? "#ef4444" : "#f59e0b",
          strokeWidth: 2,
          strokeDasharray: edge.status === "lagging" ? "5,5" : undefined,
        },
        label: edge.latencyMs ? `${edge.latencyMs}ms` : edge.type,
      });
    }
  });

  return (
    <div className="relative h-[520px] w-full overflow-hidden rounded-xl border bg-card/60">
      <div className="absolute left-3 top-3 z-10 flex items-center gap-2 rounded-lg border bg-background/95 px-3 py-2 text-xs">
        <div className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-full bg-sky-500" />
          <span>Leader</span>
        </div>
        <div className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-full bg-emerald-500" />
          <span>Follower</span>
        </div>
        <div className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-full bg-amber-500" />
          <span>Candidate</span>
        </div>
        <div className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-full bg-destructive" />
          <span>Unhealthy</span>
        </div>
        <span className="text-muted-foreground ml-2">|</span>
        <span className="text-muted-foreground">{mode === "raft" ? "Raft Cluster" : "Standalone"}</span>
        <span className="text-muted-foreground">({members.length} node{members.length !== 1 ? "s" : ""})</span>
      </div>
      <ReactFlow
        nodes={nodes}
        edges={flowEdges}
        nodeTypes={clusterNodeTypes}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        fitView
        fitViewOptions={{ padding: 0.2 }}
      >
        <Background gap={16} size={1} className="opacity-65" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
