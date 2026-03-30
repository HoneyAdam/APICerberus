import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { Save, Trash2 } from "lucide-react";
import { UpstreamMap, type HealthStatus, type UpstreamMapHistoryPoint, type UpstreamMapTarget } from "@/components/flow/UpstreamMap";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ErrorState } from "@/components/shared/ErrorState";
import { useDeleteUpstreamTarget, useUpdateUpstream, useUpstream, useUpstreamHealth } from "@/hooks/use-upstreams";

function hashSeed(value: string) {
  let hash = 0;
  for (let i = 0; i < value.length; i += 1) {
    hash = (hash << 5) - hash + value.charCodeAt(i);
    hash |= 0;
  }
  return Math.abs(hash);
}

function buildHistory(seed: number, healthy: boolean): UpstreamMapHistoryPoint[] {
  const now = Date.now();
  const points: UpstreamMapHistoryPoint[] = [];

  for (let i = 11; i >= 0; i -= 1) {
    const ts = now - i * 60 * 1000;
    const pulse = Math.sin((i + (seed % 7)) / 2.2) * 16;
    const jitter = ((seed + i * 13) % 22) - 10;
    const base = healthy ? 72 + (seed % 55) : 280 + (seed % 120);
    const latencyMS = Math.max(15, Math.round(base + pulse + jitter));

    points.push({
      timestamp: new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      latencyMS,
      healthy: healthy && latencyMS < 220,
    });
  }

  return points;
}

function statusFrom(healthy: boolean, latencyMS: number): HealthStatus {
  if (!healthy) {
    return "down";
  }
  if (latencyMS > 180) {
    return "degraded";
  }
  return "healthy";
}

function statusBadgeClass(status: HealthStatus) {
  switch (status) {
    case "healthy":
      return "border-emerald-500/60 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
    case "degraded":
      return "border-amber-500/60 bg-amber-500/10 text-amber-700 dark:text-amber-300";
    case "down":
      return "border-destructive/60 bg-destructive/10 text-destructive";
    default:
      return "";
  }
}

export function UpstreamDetailPage() {
  const { id = "" } = useParams();
  const upstreamQuery = useUpstream(id);
  const healthQuery = useUpstreamHealth(id);
  const updateUpstream = useUpdateUpstream();
  const deleteTarget = useDeleteUpstreamTarget();

  const [name, setName] = useState("");
  const [algorithm, setAlgorithm] = useState("round_robin");
  const [healthPath, setHealthPath] = useState("/health");
  const [selectedTargetID, setSelectedTargetID] = useState<string | null>(null);

  const upstream = upstreamQuery.data;
  const healthTargets = useMemo(
    () => ((healthQuery.data?.targets ?? []) as Array<Record<string, unknown>>) ?? [],
    [healthQuery.data?.targets],
  );

  const mapTargets = useMemo<UpstreamMapTarget[]>(() => {
    return (upstream?.targets ?? []).map((target, index) => {
      const health = healthTargets.find((item) => {
        const knownID = String(item.id ?? item.target_id ?? "");
        return knownID === target.id;
      });

      const healthy = Boolean(health?.healthy);
      const seed = hashSeed(`${target.id}-${target.address}-${index}`);
      const history = buildHistory(seed, healthy);
      const latencyMS = history[history.length - 1]?.latencyMS ?? (healthy ? 90 : 320);
      const status = statusFrom(healthy, latencyMS);
      const baseTraffic = 30 + (seed % 220);
      const trafficRPS =
        status === "down" ? Math.round(baseTraffic * 0.18) : status === "degraded" ? Math.round(baseTraffic * 0.62) : baseTraffic;

      return {
        id: target.id,
        address: target.address,
        weight: target.weight,
        status,
        latencyMS,
        trafficRPS,
        history,
      };
    });
  }, [upstream?.targets, healthTargets]);

  const selectedTarget = useMemo(
    () => mapTargets.find((target) => target.id === selectedTargetID) ?? null,
    [mapTargets, selectedTargetID],
  );

  useEffect(() => {
    if (!upstream) {
      return;
    }
    setName(upstream.name);
    setAlgorithm(upstream.algorithm);
    setHealthPath(String((upstream.health_check?.active as { path?: string } | undefined)?.path ?? "/health"));
  }, [upstream]);

  useEffect(() => {
    if (selectedTargetID && !mapTargets.some((target) => target.id === selectedTargetID)) {
      setSelectedTargetID(null);
    }
  }, [mapTargets, selectedTargetID]);

  if (!id) {
    return <ErrorState message="Missing upstream id." />;
  }
  if (upstreamQuery.isError) {
    return <ErrorState message="Failed to load upstream detail." onRetry={() => upstreamQuery.refetch()} />;
  }

  const handleSave = async () => {
    await updateUpstream.mutateAsync({
      id,
      payload: {
        id,
        name,
        algorithm,
        targets: upstream?.targets ?? [],
        health_check: {
          active: {
            ...(upstream?.health_check?.active ?? {}),
            path: healthPath,
          },
        },
      },
    });
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Upstream Configuration</CardTitle>
          <CardDescription>Set algorithm and health-check path configuration.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <div className="space-y-1.5">
            <Label htmlFor="upstream-detail-name">Name</Label>
            <Input id="upstream-detail-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label>Algorithm</Label>
            <Select value={algorithm} onValueChange={setAlgorithm}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {["round_robin", "weighted_round_robin", "least_conn", "least_latency"].map((value) => (
                  <SelectItem key={value} value={value}>
                    {value}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="upstream-health-path">Health Path</Label>
            <Input
              id="upstream-health-path"
              value={healthPath}
              onChange={(event) => setHealthPath(event.target.value)}
            />
          </div>
          <div className="md:col-span-3">
            <Button onClick={handleSave} disabled={updateUpstream.isPending}>
              <Save className="mr-2 size-4" />
              Save Upstream
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Upstream Health Map</CardTitle>
          <CardDescription>Gateway center topology with real-time target status colors and weighted traffic edges.</CardDescription>
        </CardHeader>
        <CardContent>
          <UpstreamMap
            upstreamName={name || upstream?.name || "upstream"}
            targets={mapTargets}
            selectedTargetID={selectedTargetID ?? undefined}
            onTargetSelect={setSelectedTargetID}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Targets</CardTitle>
          <CardDescription>Inspect and manage upstream target health.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {mapTargets.map((target) => (
            <div key={target.id} className="flex items-center justify-between rounded-md border p-3">
              <div>
                <p className="font-medium">{target.address}</p>
                <p className="text-xs text-muted-foreground">weight: {target.weight}</p>
              </div>
              <div className="flex items-center gap-2">
                <Badge variant="outline" className={statusBadgeClass(target.status)}>
                  {target.status}
                </Badge>
                <Badge variant="outline">{target.latencyMS}ms</Badge>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setSelectedTargetID(target.id)}
                >
                  Details
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => deleteTarget.mutate({ id, targetId: target.id })}
                  aria-label="Delete target"
                >
                  <Trash2 className="size-4 text-destructive" />
                </Button>
              </div>
            </div>
          ))}
          {!mapTargets.length ? <p className="text-sm text-muted-foreground">No targets configured.</p> : null}
        </CardContent>
      </Card>

      <Sheet
        open={Boolean(selectedTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedTargetID(null);
          }
        }}
      >
        <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
          <SheetHeader>
            <SheetTitle>{selectedTarget?.address ?? "Target detail"}</SheetTitle>
            <SheetDescription>Health history and latency profile for selected target.</SheetDescription>
          </SheetHeader>

          {selectedTarget ? (
            <div className="space-y-4 px-4 pb-6">
              <div className="grid grid-cols-2 gap-2">
                <div className="rounded-md border p-2">
                  <p className="text-[11px] text-muted-foreground">Status</p>
                  <Badge variant="outline" className={`mt-1 ${statusBadgeClass(selectedTarget.status)}`}>
                    {selectedTarget.status}
                  </Badge>
                </div>
                <div className="rounded-md border p-2">
                  <p className="text-[11px] text-muted-foreground">Traffic</p>
                  <p className="mt-1 text-sm font-semibold">{selectedTarget.trafficRPS} rps</p>
                </div>
                <div className="rounded-md border p-2">
                  <p className="text-[11px] text-muted-foreground">Latency</p>
                  <p className="mt-1 text-sm font-semibold">{selectedTarget.latencyMS} ms</p>
                </div>
                <div className="rounded-md border p-2">
                  <p className="text-[11px] text-muted-foreground">Weight</p>
                  <p className="mt-1 text-sm font-semibold">{selectedTarget.weight}</p>
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-sm font-medium">Latency Chart</p>
                <div className="h-40 rounded-md border bg-muted/20 p-2">
                  <div className="flex h-full items-end gap-1">
                    {selectedTarget.history.map((point) => {
                      const peak = Math.max(1, ...selectedTarget.history.map((item) => item.latencyMS));
                      const height = Math.max(8, Math.round((point.latencyMS / peak) * 100));
                      return (
                        <div key={`${point.timestamp}-${point.latencyMS}`} className="flex-1">
                          <div
                            className="w-full rounded-t-sm"
                            style={{
                              height: `${height}%`,
                              background: point.healthy ? "hsl(var(--chart-2))" : "hsl(var(--destructive))",
                              opacity: point.healthy ? 0.85 : 0.9,
                            }}
                            title={`${point.timestamp} - ${point.latencyMS}ms`}
                          />
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-sm font-medium">Health History</p>
                {[...selectedTarget.history]
                  .reverse()
                  .map((point) => (
                    <div key={`history-${point.timestamp}-${point.latencyMS}`} className="flex items-center justify-between rounded-md border px-2 py-1.5 text-xs">
                      <span className="text-muted-foreground">{point.timestamp}</span>
                      <span>{point.latencyMS} ms</span>
                      <Badge variant="outline" className={statusBadgeClass(point.healthy ? "healthy" : "down")}>
                        {point.healthy ? "healthy" : "down"}
                      </Badge>
                    </div>
                  ))}
              </div>
            </div>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  );
}
