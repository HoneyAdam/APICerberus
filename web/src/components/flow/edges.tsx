import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps, type EdgeTypes } from "@xyflow/react";

function TrafficEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition }: EdgeProps) {
  const [edgePath] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });

  return <BaseEdge id={id} path={edgePath} className="flow-traffic-edge" style={{ strokeWidth: 2.2 }} />;
}

function RaftEdge({ id, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition }: EdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  });

  return (
    <>
      <BaseEdge id={id} path={edgePath} className="flow-raft-edge" style={{ strokeWidth: 1.7 }} />
      <EdgeLabelRenderer>
        <div
          style={{
            position: "absolute",
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: "none",
          }}
          className="rounded-md border border-border bg-background/90 px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground"
        >
          heartbeat
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

export const flowEdgeTypes: EdgeTypes = {
  trafficEdge: TrafficEdge,
  raftEdge: RaftEdge,
};
