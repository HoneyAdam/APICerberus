import { useState } from "react";
import {
  Area,
  AreaChart as RechartsAreaChart,
  CartesianGrid,
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
import { cn } from "@/lib/utils";
import { type TimeWindow, TIME_WINDOW_OPTIONS, filterDataByWindow, formatTimeTick } from "./chart-utils";

type AreaDataPoint = {
  timestamp: string | number | Date;
  requests: number;
  errors?: number;
};

type AreaChartProps = {
  data: AreaDataPoint[];
  title?: string;
  className?: string;
  height?: number;
  window?: TimeWindow;
  onWindowChange?: (window: TimeWindow) => void;
};

export function AreaChart({
  data,
  title = "Traffic (Real-time)",
  className,
  height = 280,
  window,
  onWindowChange,
}: AreaChartProps) {
  const [localWindow, setLocalWindow] = useState<TimeWindow>(window ?? "1h");
  const selectedWindow = window ?? localWindow;

  const filteredData = filterDataByWindow(data, selectedWindow, (item) => item.timestamp);

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
            <RechartsAreaChart data={filteredData}>
              <defs>
                <linearGradient id="requestsGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="hsl(var(--chart-1))" stopOpacity={0.45} />
                  <stop offset="95%" stopColor="hsl(var(--chart-1))" stopOpacity={0.05} />
                </linearGradient>
                <linearGradient id="errorsGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="hsl(var(--chart-5))" stopOpacity={0.35} />
                  <stop offset="95%" stopColor="hsl(var(--chart-5))" stopOpacity={0.05} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="3 3" vertical={false} />
              <XAxis
                dataKey="timestamp"
                tickFormatter={formatTimeTick}
                tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
              />
              <YAxis tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }} />
              <Tooltip
                cursor={{ stroke: "hsl(var(--border))" }}
                contentStyle={{
                  background: "hsl(var(--popover))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "var(--radius)",
                }}
                labelStyle={{ color: "hsl(var(--foreground))" }}
              />
              <Area
                type="monotone"
                dataKey="requests"
                stroke="hsl(var(--chart-1))"
                fillOpacity={1}
                fill="url(#requestsGradient)"
                strokeWidth={2}
              />
              <Area
                type="monotone"
                dataKey="errors"
                stroke="hsl(var(--chart-5))"
                fillOpacity={1}
                fill="url(#errorsGradient)"
                strokeWidth={1.5}
                className={cn(data.some((item) => typeof item.errors === "number") ? "" : "hidden")}
              />
            </RechartsAreaChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

