import { useState } from "react";
import {
  CartesianGrid,
  Line,
  LineChart as RechartsLineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
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

type LineDataPoint = {
  timestamp: string | number | Date;
  p50?: number;
  p95?: number;
  p99?: number;
  value?: number;
};

type LineChartProps = {
  data: LineDataPoint[];
  title?: string;
  className?: string;
  height?: number;
  window?: TimeWindow;
  onWindowChange?: (window: TimeWindow) => void;
};

export function LineChart({
  data,
  title = "Latency Trend",
  className,
  height = 280,
  window,
  onWindowChange,
}: LineChartProps) {
  const [localWindow, setLocalWindow] = useState<TimeWindow>(window ?? "24h");
  const selectedWindow = window ?? localWindow;

  const filteredData = filterDataByWindow(data, selectedWindow, (item) => item.timestamp);

  const hasPercentiles = filteredData.some((item) => item.p95 !== undefined || item.p99 !== undefined);

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
            <RechartsLineChart data={filteredData}>
              <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="3 3" vertical={false} />
              <XAxis
                dataKey="timestamp"
                tickFormatter={formatTimeTick}
                tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
              />
              <YAxis tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }} />
              <Tooltip
                contentStyle={{
                  background: "hsl(var(--popover))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "var(--radius)",
                }}
              />
              <Line
                type="monotone"
                dataKey={hasPercentiles ? "p50" : "value"}
                stroke="hsl(var(--chart-1))"
                strokeWidth={2}
                dot={false}
              />
              {hasPercentiles ? (
                <>
                  <Line type="monotone" dataKey="p95" stroke="hsl(var(--chart-4))" strokeWidth={2} dot={false} />
                  <Line type="monotone" dataKey="p99" stroke="hsl(var(--chart-5))" strokeWidth={2} dot={false} />
                </>
              ) : null}
            </RechartsLineChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

