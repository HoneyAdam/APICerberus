import { useMemo } from "react";
import { LineChart } from "@/components/charts/LineChart";
import { AreaChart } from "@/components/charts/AreaChart";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { usePortalUsageErrors, usePortalUsageTimeseries } from "@/hooks/use-portal";

export function PortalUsagePage() {
  const timeseriesQuery = usePortalUsageTimeseries({ window: "7d", granularity: "6h" });
  const errorsQuery = usePortalUsageErrors({ window: "7d" });

  const areaData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item) => ({
        timestamp: item.timestamp,
        requests: item.requests,
        errors: item.errors,
      })),
    [timeseriesQuery.data?.items],
  );

  const latencyData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item) => ({
        timestamp: item.timestamp,
        value: item.avg_latency_ms,
      })),
    [timeseriesQuery.data?.items],
  );

  const errorRows = useMemo(
    () => Object.entries(errorsQuery.data?.status_map ?? {}).sort((a, b) => Number(b[1]) - Number(a[1])),
    [errorsQuery.data?.status_map],
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Usage Analytics</h2>
        <p className="text-sm text-muted-foreground">Requests, latency and error behavior over time.</p>
      </div>

      <AreaChart title="Request Count & Errors" data={areaData} />
      <LineChart title="Average Latency" data={latencyData} />

      <Card>
        <CardHeader>
          <CardTitle>Error Rate by Status Code</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-2 md:grid-cols-3">
            {errorRows.map(([code, count]) => (
              <div key={code} className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">Status {code}</p>
                <p className="text-xl font-semibold">{count}</p>
              </div>
            ))}
            {errorRows.length === 0 ? <p className="text-sm text-muted-foreground">No errors in selected window.</p> : null}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
