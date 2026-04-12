import { useState } from "react";
import {
  Cell,
  Pie,
  PieChart as RechartsPieChart,
  ResponsiveContainer,
  Tooltip,
  Legend,
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

type PieDataPoint = {
  name: string;
  value: number;
  timestamp?: string | number | Date;
};

type PieChartProps = {
  data: PieDataPoint[];
  title?: string;
  className?: string;
  height?: number;
  window?: TimeWindow;
  onWindowChange?: (window: TimeWindow) => void;
};

const CHART_COLORS = [
  "hsl(var(--chart-1))",
  "hsl(var(--chart-2))",
  "hsl(var(--chart-3))",
  "hsl(var(--chart-4))",
  "hsl(var(--chart-5))",
];

export function PieChart({
  data,
  title = "Status Code Distribution",
  className,
  height = 280,
  window,
  onWindowChange,
}: PieChartProps) {
  const [localWindow, setLocalWindow] = useState<TimeWindow>(window ?? "24h");
  const selectedWindow = window ?? localWindow;

  const filteredData = data.some((item) => item.timestamp)
    ? filterDataByWindow(data, selectedWindow, (item) => item.timestamp ?? Date.now())
    : data;

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
            <RechartsPieChart>
              <Pie data={filteredData} dataKey="value" nameKey="name" innerRadius={54} outerRadius={88} label>
                {filteredData.map((entry, index) => (
                  <Cell key={`pie-segment-${entry.name}`} fill={CHART_COLORS[index % CHART_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip
                contentStyle={{
                  background: "hsl(var(--popover))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "var(--radius)",
                }}
              />
              <Legend />
            </RechartsPieChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

