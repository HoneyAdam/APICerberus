import { ClusterTopology } from "@/components/flow/ClusterTopology";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { AlertCircle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useClusterStatus, useClusterRealtime } from "@/hooks/use-cluster";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

export function ClusterPage() {
  const { data: clusterStatus, isLoading, error, refetch } = useClusterStatus();
  const { status: realtimeStatus, isConnected } = useClusterRealtime();

  // Merge REST data with realtime updates
  const status = isConnected ? realtimeStatus : clusterStatus;

  const nodes = status?.nodes.map(node => ({
    id: node.id,
    name: node.name,
    role: node.role,
    address: node.address,
    state: node.state,
    lastSeen: node.lastSeen,
  })) || [];

  const edges = status?.edges.map(edge => ({
    from: edge.from,
    to: edge.to,
    type: edge.type,
    status: edge.status,
    latencyMs: edge.latencyMs,
  })) || [];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Cluster</h1>
          <p className="text-muted-foreground">
            Manage and monitor your APICerebrus cluster nodes
          </p>
        </div>
        <div className="flex items-center gap-2">
          {isConnected && (
            <Badge variant="outline" className="bg-emerald-500/10 text-emerald-700 border-emerald-500/20">
              Live
            </Badge>
          )}
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isLoading}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
        </div>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            Failed to load cluster status. Running in standalone mode.
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Mode</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold capitalize">{status?.mode || "standalone"}</div>
            <p className="text-xs text-muted-foreground">
              {status?.enabled ? "Clustering enabled" : "Standalone mode"}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Nodes</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{nodes.length}</div>
            <p className="text-xs text-muted-foreground">
              {nodes.filter(n => n.state === "healthy").length} healthy
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Leader</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold truncate">
              {status?.leaderId ? nodes.find(n => n.id === status.leaderId)?.name || status.leaderId : "-"}
            </div>
            <p className="text-xs text-muted-foreground">
              Term: {status?.term || 0}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Commit Index</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{status?.commitIndex?.toLocaleString() || "0"}</div>
            <p className="text-xs text-muted-foreground">
              Applied: {status?.appliedIndex?.toLocaleString() || "0"}
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Cluster Topology</CardTitle>
          <CardDescription>
            Real-time visualization of cluster nodes and their connections
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-[520px] w-full" />
          ) : (
            <ClusterTopology members={nodes} edges={edges} mode={status?.mode} />
          )}
        </CardContent>
      </Card>

      {nodes.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Node Details</CardTitle>
            <CardDescription>Detailed information about cluster nodes</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {nodes.map(node => (
                <div
                  key={node.id}
                  className="flex items-center justify-between rounded-lg border p-3"
                >
                  <div className="flex items-center gap-3">
                    <div className={`w-2 h-2 rounded-full ${
                      node.state === "healthy" ? "bg-emerald-500" :
                      node.state === "unhealthy" ? "bg-destructive" :
                      "bg-amber-500"
                    }`} />
                    <div>
                      <p className="font-medium">{node.name}</p>
                      <p className="text-sm text-muted-foreground">{node.address}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant={
                      node.role === "leader" ? "default" :
                      node.role === "follower" ? "secondary" :
                      node.role === "candidate" ? "outline" :
                      "destructive"
                    }>
                      {node.role}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {node.lastSeen ? new Date(node.lastSeen).toLocaleTimeString() : "-"}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
