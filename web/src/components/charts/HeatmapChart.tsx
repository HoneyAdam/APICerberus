import { useMemo, useState } from "react";
import {
  CartesianGrid,
  ResponsiveContainer,
  Scatter,
  ScatterChart,
  Tooltip,
  XAxis,
  YAxis,
  ZAxis,
  Cell,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { type TimeWindow, TIME_WINDOW_OPTIONS, filterDataByWindow, formatTimeTick } from "./chart-utils";

type HeatmapDataPoint = {
  timestamp: string | number | Date;
  latencyMs: number;
  bucket?: number;
  label?: string;
};

type HeatmapChartProps = {
  data: HeatmapDataPoint[];
  title?: string;
  className?: string;
  height?: number;
  window?: TimeWindow;
  onWindowChange?: (window: TimeWindow) => void;
};

function latencyToColor(latency: number, maxLatency: number) {
  if (!maxLatency || !Number.isFinite(latency)) {
    return "hsl(var(--chart-2))";
  }
  const ratio = Math.max(0, Math.min(1, latency / maxLatency));
  if (ratio < 0.33) {
    return "hsl(var(--chart-2))";
  }
  if (ratio < 0.66) {
    return "hsl(var(--chart-4))";
  }
  return "hsl(var(--chart-5))";
}

export function HeatmapChart({
  data,
  title = "Latency Heatmap",
  className,
  height = 300,
  window,
  onWindowChange,
}: HeatmapChartProps) {
  const [localWindow, setLocalWindow] = useState<TimeWindow>(window ?? "24h");
  const selectedWindow = window ?? localWindow;

  const filteredData = filterDataByWindow(data, selectedWindow, (item) => item.timestamp);

  const scatterData = useMemo(
    () =>
      filteredData.map((item, index) => ({
        x: new Date(item.timestamp).getTime(),
        y: item.bucket ?? (index % 10) + 1,
        z: item.latencyMs,
        label: item.label ?? `Point ${index + 1}`,
      })),
    [filteredData],
  );

  const maxLatency = useMemo(
    () => scatterData.reduce((max, item) => Math.max(max, item.z), 0),
    [scatterData],
  );

  const handleWindowChange = (next: TimeWindow) => {
    if (window === undefined) {
      setLocalWindow(next);
    }
    onWindowChange?.(next);
  };

  return (
    <Card className={className}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <CardTitle>{title}</CardTitle>
        <Select value={selectedWindow} onValueChange={(value: TimeWindow) => handleWindowChange(value)}>
          <SelectTrigger className="h-8 w-20">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_WINDOW_OPTIONS.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col" style={{ height: `${height}px` }}>
          <ResponsiveContainer width="100%" height="100%">
            <ScatterChart margin={{ top: 8, right: 8, bottom: 4, left: 0 }}>
              <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="3 3" />
              <XAxis
                type="number"
                dataKey="x"
                domain={["dataMin", "dataMax"]}
                tickFormatter={(value) => formatTimeTick(value)}
                tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
              />
              <YAxis
                type="number"
                dataKey="y"
                tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
                width={32}
              />
              <ZAxis type="number" dataKey="z" range={[48, 420]} />
              <Tooltip
                cursor={{ strokeDasharray: "4 4" }}
                contentStyle={{
                  background: "hsl(var(--popover))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "var(--radius)",
                }}
                formatter={(value) => [`${value} ms`, "Latency"]}
                labelFormatter={(_, payload) => {
                  const row = payload?.[0]?.payload as { label?: string } | undefined;
                  return row?.label ?? "Request";
                }}
              />
              <Scatter data={scatterData}>
                {scatterData.map((point, index) => (
                  <Cell key={`heat-cell-${index}`} fill={latencyToColor(point.z, maxLatency)} fillOpacity={0.85} />
                ))}
              </Scatter>
            </ScatterChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

