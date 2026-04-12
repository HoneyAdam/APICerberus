import { useState } from "react";
import {
  Bar,
  BarChart as RechartsBarChart,
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
import { type TimeWindow, TIME_WINDOW_OPTIONS, filterDataByWindow } from "./chart-utils";

type BarDataPoint = {
  name: string;
  value: number;
  secondary?: number;
  timestamp?: string | number | Date;
};

type BarChartProps = {
  data: BarDataPoint[];
  title?: string;
  className?: string;
  height?: number;
  window?: TimeWindow;
  onWindowChange?: (window: TimeWindow) => void;
};

export function BarChart({
  data,
  title = "Credit Usage / Error Breakdown",
  className,
  height = 280,
  window,
  onWindowChange,
}: BarChartProps) {
  const [localWindow, setLocalWindow] = useState<TimeWindow>(window ?? "24h");
  const selectedWindow = window ?? localWindow;

  const filteredData = data.some((item) => item.timestamp)
    ? filterDataByWindow(data, selectedWindow, (item) => item.timestamp ?? Date.now())
    : data;

  const hasSecondary = filteredData.some((item) => typeof item.secondary === "number");

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
            <RechartsBarChart data={filteredData}>
              <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="name" tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }} />
              <YAxis tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }} />
              <Tooltip
                cursor={{ fill: "hsl(var(--muted) / 0.3)" }}
                contentStyle={{
                  background: "hsl(var(--popover))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "var(--radius)",
                }}
              />
              <Bar dataKey="value" fill="hsl(var(--chart-2))" radius={[8, 8, 0, 0]} />
              {hasSecondary ? <Bar dataKey="secondary" fill="hsl(var(--chart-5))" radius={[8, 8, 0, 0]} /> : null}
            </RechartsBarChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

