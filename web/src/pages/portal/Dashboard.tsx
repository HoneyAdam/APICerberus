import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Activity, AlertTriangle, CreditCard, Timer } from "lucide-react";
import { AreaChart } from "@/components/charts/AreaChart";
import { DataTable } from "@/components/shared/DataTable";
import { KPICard } from "@/components/shared/KPICard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { usePortalUsageOverview, usePortalUsageTimeseries, usePortalUsageTopEndpoints } from "@/hooks/use-portal";
import type { PortalTopEndpoint } from "@/lib/portal-types";

const TOP_ENDPOINT_COLUMNS: ColumnDef<PortalTopEndpoint>[] = [
  {
    accessorKey: "route_name",
    header: "Endpoint",
  },
  {
    accessorKey: "count",
    header: "Requests",
  },
];

export function PortalDashboardPage() {
  const overviewQuery = usePortalUsageOverview({ window: "24h" });
  const timeseriesQuery = usePortalUsageTimeseries({ window: "24h", granularity: "1h" });
  const topEndpointsQuery = usePortalUsageTopEndpoints({ window: "24h" });

  const chartData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item) => ({
        timestamp: item.timestamp,
        requests: item.requests,
        errors: item.errors,
      })),
    [timeseriesQuery.data?.items],
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Dashboard</h2>
        <p className="text-sm text-muted-foreground">Quick overview of request volume, reliability and credits.</p>
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <KPICard
          label="Credit Balance"
          value={overviewQuery.data?.credit_balance ?? 0}
          icon={CreditCard}
          description="Current available credits"
        />
        <KPICard
          label="Requests (24h)"
          value={overviewQuery.data?.total_requests ?? 0}
          icon={Activity}
          description="Total calls in selected window"
        />
        <KPICard
          label="Error Rate"
          value={`${((overviewQuery.data?.error_rate ?? 0) * 100).toFixed(2)}%`}
          icon={AlertTriangle}
          description="5xx request ratio"
        />
        <KPICard
          label="Avg Latency"
          value={`${(overviewQuery.data?.avg_latency_ms ?? 0).toFixed(1)} ms`}
          icon={Timer}
          description="Average response latency"
        />
      </div>

      <AreaChart title="Request & Error Trend" data={chartData} />

      <Card>
        <CardHeader>
          <CardTitle>Top Endpoints</CardTitle>
        </CardHeader>
        <CardContent>
          <DataTable<PortalTopEndpoint, unknown>
            data={topEndpointsQuery.data?.items ?? []}
            columns={TOP_ENDPOINT_COLUMNS}
            searchColumn="route_name"
            searchPlaceholder="Search endpoint..."
            initialPageSize={5}
          />
        </CardContent>
      </Card>
    </div>
  );
}
