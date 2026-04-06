import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { adminApiRequest } from "@/lib/api";
import { AreaChart } from "@/components/charts/AreaChart";
import { PieChart } from "@/components/charts/PieChart";
import { HeatmapChart } from "@/components/charts/HeatmapChart";
import { DataTable } from "@/components/shared/DataTable";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useAnalyticsTimeseries, useAnalyticsTopRoutes } from "@/hooks/use-analytics";
import type { TopRoute } from "@/lib/types";
import { GeoDistributionChart } from "@/components/analytics/GeoDistributionChart";
import { RateLimitStatsCard } from "@/components/analytics/RateLimitStats";
import type { GeoDataPoint, RateLimitStats } from "@/components/analytics/types";

type TopConsumer = {
  user_id: string;
  consumer_name: string;
  count: number;
};

const TOP_ROUTE_COLUMNS: ColumnDef<TopRoute>[] = [
  { accessorKey: "route_name", header: "Route" },
  { accessorKey: "count", header: "Requests" },
];

const TOP_CONSUMER_COLUMNS: ColumnDef<TopConsumer>[] = [
  { accessorKey: "consumer_name", header: "Consumer" },
  { accessorKey: "count", header: "Requests" },
];

export function AnalyticsPage() {
  const timeseriesQuery = useAnalyticsTimeseries({ window: "24h", granularity: "15m" });
  const topRoutesQuery = useAnalyticsTopRoutes({ window: "24h", limit: 10 });

  const topConsumersQuery = useQuery({
    queryKey: ["analytics-top-consumers"],
    queryFn: () =>
      adminApiRequest<{ consumers: TopConsumer[] }>("/admin/api/v1/analytics/top-consumers", {
        query: { window: "24h", limit: 10 },
      }),
  });

  const statusCodesQuery = useQuery({
    queryKey: ["analytics-status-codes"],
    queryFn: () => adminApiRequest<{ status_codes: Record<string, number> }>("/admin/api/v1/analytics/status-codes"),
  });

  const areaData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item) => ({
        timestamp: item.timestamp,
        requests: item.requests,
        errors: item.errors,
      })),
    [timeseriesQuery.data?.items],
  );

  const heatmapData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item, index) => ({
        timestamp: item.timestamp,
        latencyMs: item.p95_latency_ms,
        bucket: (index % 10) + 1,
        label: `p95 @ ${new Date(item.timestamp).toLocaleTimeString()}`,
      })),
    [timeseriesQuery.data?.items],
  );

  const pieData = useMemo(
    () =>
      Object.entries(statusCodesQuery.data?.status_codes ?? {}).map(([name, value]) => ({
        name,
        value,
      })),
    [statusCodesQuery.data?.status_codes],
  );

  const topRoutes = useMemo(() => topRoutesQuery.data?.routes ?? [], [topRoutesQuery.data?.routes]);
  const topConsumers = useMemo(() => topConsumersQuery.data?.consumers ?? [], [topConsumersQuery.data?.consumers]);

  // Fetch geo distribution data
  const geoQuery = useQuery({
    queryKey: ["analytics-geo"],
    queryFn: () => adminApiRequest<{ countries: GeoDataPoint[] }>("/admin/api/v1/analytics/geo"),
  });

  // Fetch rate limiting stats
  const rateLimitQuery = useQuery({
    queryKey: ["analytics-rate-limits"],
    queryFn: () => adminApiRequest<RateLimitStats>("/admin/api/v1/analytics/rate-limits"),
  });

  const geoData = useMemo(() => geoQuery.data?.countries ?? [], [geoQuery.data]);
  const rateLimitData = rateLimitQuery.data;

  return (
    <div className="space-y-4">
      <AreaChart data={areaData} title="Traffic Time-Series" />

      <div className="grid gap-4 xl:grid-cols-2">
        <PieChart data={pieData} title="Status Code Distribution" />
        <HeatmapChart data={heatmapData} title="Latency Heatmap (p95)" />
      </div>

      {/* Advanced Analytics */}
      <div className="grid gap-4 xl:grid-cols-2">
        {geoData.length > 0 && (
          <GeoDistributionChart data={geoData} />
        )}
        {rateLimitData && (
          <RateLimitStatsCard data={rateLimitData} />
        )}
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Top Routes</CardTitle>
            <CardDescription>Highest volume route endpoints.</CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable<TopRoute, unknown>
              data={topRoutes}
              columns={TOP_ROUTE_COLUMNS}
              searchColumn="route_name"
              searchPlaceholder="Filter route..."
              fileName="analytics-top-routes"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Top Consumers</CardTitle>
            <CardDescription>Most active consumers in current window.</CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable<TopConsumer, unknown>
              data={topConsumers}
              columns={TOP_CONSUMER_COLUMNS}
              searchColumn="consumer_name"
              searchPlaceholder="Filter consumer..."
              fileName="analytics-top-consumers"
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

