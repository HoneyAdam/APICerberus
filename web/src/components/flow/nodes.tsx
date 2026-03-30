import { memo, type ReactNode } from "react";
import { Handle, Position, type NodeProps, type NodeTypes } from "@xyflow/react";
import { Boxes, Network, Puzzle, Server, Shield } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type NodeTone = "default" | "success" | "warning" | "danger";

type BaseFlowNodeData = {
  title: string;
  subtitle?: string;
  meta?: string;
  tone?: NodeTone;
};

export type PluginFlowNodeData = BaseFlowNodeData & {
  phase?: string;
  configSummary?: string;
  onEdit?: () => void;
};

function toneClass(tone: NodeTone | undefined) {
  switch (tone) {
    case "success":
      return "border-emerald-500/50 bg-emerald-500/10";
    case "warning":
      return "border-amber-500/50 bg-amber-500/10";
    case "danger":
      return "border-destructive/60 bg-destructive/10";
    default:
      return "border-border bg-card";
  }
}

function NodeShell({
  icon,
  data,
  children,
}: {
  icon: ReactNode;
  data: BaseFlowNodeData;
  children?: ReactNode;
}) {
  return (
    <div className={cn("min-w-[180px] rounded-xl border p-3 shadow-sm", toneClass(data.tone))}>
      <Handle type="target" position={Position.Left} className="!h-2.5 !w-2.5 !border-0 !bg-primary" />
      <div className="mb-2 flex items-center gap-2">
        <span className="inline-flex size-7 items-center justify-center rounded-lg bg-muted text-foreground">{icon}</span>
        <div>
          <p className="text-sm font-semibold leading-none">{data.title}</p>
          {data.subtitle ? <p className="mt-1 text-[11px] text-muted-foreground">{data.subtitle}</p> : null}
        </div>
      </div>
      {data.meta ? <p className="text-[11px] text-muted-foreground">{data.meta}</p> : null}
      {children}
      <Handle type="source" position={Position.Right} className="!h-2.5 !w-2.5 !border-0 !bg-primary" />
    </div>
  );
}

function GatewayNodeImpl({ data }: NodeProps) {
  return <NodeShell icon={<Shield className="size-4" />} data={data as BaseFlowNodeData} />;
}

function ServiceNodeImpl({ data }: NodeProps) {
  return <NodeShell icon={<Boxes className="size-4" />} data={data as BaseFlowNodeData} />;
}

function UpstreamNodeImpl({ data }: NodeProps) {
  return <NodeShell icon={<Network className="size-4" />} data={data as BaseFlowNodeData} />;
}

function ClusterNodeImpl({ data }: NodeProps) {
  return <NodeShell icon={<Server className="size-4" />} data={data as BaseFlowNodeData} />;
}

function PluginNodeImpl({ data }: NodeProps) {
  const pluginData = data as PluginFlowNodeData;
  return (
    <NodeShell icon={<Puzzle className="size-4" />} data={pluginData}>
      <div className="mt-2 space-y-2 border-t pt-2">
        {pluginData.phase ? <p className="text-[11px] text-muted-foreground">Phase: {pluginData.phase}</p> : null}
        {pluginData.configSummary ? <p className="text-[11px] text-muted-foreground">{pluginData.configSummary}</p> : null}
        <Button
          size="sm"
          variant="outline"
          className="h-7 w-full text-xs"
          onClick={(event) => {
            event.stopPropagation();
            pluginData.onEdit?.();
          }}
        >
          Edit Config
        </Button>
      </div>
    </NodeShell>
  );
}

export const GatewayNode = memo(GatewayNodeImpl);
export const ServiceNode = memo(ServiceNodeImpl);
export const UpstreamNode = memo(UpstreamNodeImpl);
export const PluginNode = memo(PluginNodeImpl);
export const ClusterNode = memo(ClusterNodeImpl);

export const flowNodeTypes: NodeTypes = {
  gatewayNode: GatewayNode,
  serviceNode: ServiceNode,
  upstreamNode: UpstreamNode,
  pluginNode: PluginNode,
  clusterNode: ClusterNode,
};
