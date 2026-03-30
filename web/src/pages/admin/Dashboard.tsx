import { useEffect, useMemo, useRef } from "react";
import { Activity, AlertTriangle, Coins, Users } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { AreaChart } from "@/components/charts/AreaChart";
import { DataTable } from "@/components/shared/DataTable";
import { KPICard } from "@/components/shared/KPICard";
import { TimeAgo } from "@/components/shared/TimeAgo";
import { useAnalyticsOverview, useAnalyticsTimeseries, useAnalyticsTopRoutes } from "@/hooks/use-analytics";
import { useRealtime } from "@/hooks/use-realtime";
import { useUsers } from "@/hooks/use-users";

type TopRouteRow = {
  route_name: string;
  count: number;
};

const TOP_ROUTE_COLUMNS: ColumnDef<TopRouteRow>[] = [
  {
    accessorKey: "route_name",
    header: "Route",
    cell: ({ row }) => <span className="font-medium">{row.original.route_name}</span>,
  },
  {
    accessorKey: "count",
    header: "Requests",
  },
];

export function DashboardPage() {
  const overviewQuery = useAnalyticsOverview({ window: "1h" });
  const timeseriesQuery = useAnalyticsTimeseries({ window: "1h", granularity: "1m" });
  const topRoutesQuery = useAnalyticsTopRoutes({ window: "1h", limit: 5 });
  const usersQuery = useUsers({ limit: 1 });
  const tailShellRef = useRef<HTMLDivElement | null>(null);

  const realtime = useRealtime({ autoConnect: true, requestTailSize: 24, eventTailSize: 120 });

  const chartData = useMemo(
    () => {
      const base = (timeseriesQuery.data?.items ?? []).map((item) => ({
        timestamp: item.timestamp,
        requests: item.requests,
        errors: item.errors,
      }));

      const byTimestamp = new Map<string, { timestamp: string; requests: number; errors: number }>();
      for (const point of base) {
        byTimestamp.set(point.timestamp, { ...point });
      }
      for (const point of realtime.trafficSeries) {
        const existing = byTimestamp.get(point.timestamp);
        if (existing) {
          existing.requests += point.requests;
          existing.errors += point.errors;
        } else {
          byTimestamp.set(point.timestamp, { ...point });
        }
      }

      return [...byTimestamp.values()].sort(
        (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
      );
    },
    [realtime.trafficSeries, timeseriesQuery.data?.items],
  );

  const topRoutes = useMemo(() => topRoutesQuery.data?.routes ?? [], [topRoutesQuery.data?.routes]);
  const requestTail = realtime.requestTail;

  useEffect(() => {
    const viewport = tailShellRef.current?.querySelector<HTMLElement>("[data-slot='scroll-area-viewport']");
    if (!viewport) {
      return;
    }
    viewport.scrollTop = viewport.scrollHeight;
  }, [requestTail.length]);

  return (
    <div className="space-y-5">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <KPICard
          label="Requests (1h)"
          value={overviewQuery.data?.total_requests ?? 0}
          icon={Activity}
          description="Recent traffic volume"
        />
        <KPICard
          label="Users"
          value={usersQuery.data?.total ?? 0}
          icon={Users}
          description="Registered user count"
        />
        <KPICard
          label="Credits Consumed"
          value={overviewQuery.data?.credits_consumed ?? 0}
          icon={Coins}
          description="Window-level credit usage"
        />
        <KPICard
          label="Error Rate"
          value={`${((overviewQuery.data?.error_rate ?? 0) * 100).toFixed(2)}%`}
          icon={AlertTriangle}
          trend={-((overviewQuery.data?.error_rate ?? 0) * 100)}
          description="Lower is better"
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-3">
        <AreaChart className="xl:col-span-2" data={chartData} />

        <Card>
          <CardHeader>
            <CardTitle>Live Request Tail</CardTitle>
            <CardDescription>
              WebSocket status: <Badge variant="outline">{realtime.status}</Badge>
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div ref={tailShellRef}>
            <ScrollArea className="h-[280px] rounded-md border">
              <div className="space-y-2 p-3">
                {requestTail.length ? (
                  requestTail.map((event, index) => (
                    <div key={`event-${event.timestamp}-${index}`} className="rounded-md border bg-muted/30 p-2 text-xs">
                      <div className="mb-1 flex items-center justify-between">
                        <span className="font-semibold uppercase">{event.method || "GET"} {event.path || "/"}</span>
                        <span className="text-muted-foreground">
                          <TimeAgo value={event.timestamp} />
                        </span>
                      </div>
                      <div className="flex items-center gap-2 text-muted-foreground">
                        <Badge variant="outline">{event.status_code}</Badge>
                        <span>{event.route_name || event.route_id || "unknown route"}</span>
                        <span>{event.latency_ms}ms</span>
                        <span>{event.bytes_out}B</span>
                      </div>
                    </div>
                  ))
                ) : (
                  <p className="text-sm text-muted-foreground">No live events yet.</p>
                )}
              </div>
            </ScrollArea>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Top Routes</CardTitle>
          <CardDescription>Most requested routes in the selected window.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable<TopRouteRow, unknown>
            columns={TOP_ROUTE_COLUMNS}
            data={topRoutes}
            searchColumn="route_name"
            searchPlaceholder="Search route..."
            fileName="top-routes"
          />
        </CardContent>
      </Card>
    </div>
  );
}
