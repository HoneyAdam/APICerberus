import { useEffect, useMemo } from "react";
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
import { useRealtimeStore } from "@/stores/realtime";
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

  const realtimeStatus = useRealtimeStore((state) => state.status);
  const realtimeEvents = useRealtimeStore((state) => state.events);
  const connectRealtime = useRealtimeStore((state) => state.connect);
  const disconnectRealtime = useRealtimeStore((state) => state.disconnect);

  useEffect(() => {
    connectRealtime();
    return () => {
      disconnectRealtime();
    };
  }, [connectRealtime, disconnectRealtime]);

  const chartData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item) => ({
        timestamp: item.timestamp,
        requests: item.requests,
        errors: item.errors,
      })),
    [timeseriesQuery.data?.items],
  );

  const topRoutes = useMemo(() => topRoutesQuery.data?.routes ?? [], [topRoutesQuery.data?.routes]);
  const recentEvents = useMemo(() => realtimeEvents.slice(0, 20), [realtimeEvents]);

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
              WebSocket status: <Badge variant="outline">{realtimeStatus}</Badge>
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[280px] rounded-md border">
              <div className="space-y-2 p-3">
                {recentEvents.length ? (
                  recentEvents.map((event, index) => (
                    <div key={`event-${index}`} className="rounded-md border bg-muted/30 p-2 text-xs">
                      <div className="mb-1 flex items-center justify-between">
                        <span className="font-semibold uppercase">{event.type}</span>
                        <span className="text-muted-foreground">
                          <TimeAgo value={event.timestamp} />
                        </span>
                      </div>
                      <pre className="overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">
                        {JSON.stringify(event.payload, null, 2)}
                      </pre>
                    </div>
                  ))
                ) : (
                  <p className="text-sm text-muted-foreground">No live events yet.</p>
                )}
              </div>
            </ScrollArea>
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
