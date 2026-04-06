import { useMemo } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Gauge, Shield, AlertTriangle, CheckCircle2, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";

export type RateLimitBucket = {
  window: string;
  allowed: number;
  throttled: number;
  blocked: number;
  avgLatencyMs: number;
};

export type RateLimitStats = {
  totalRequests: number;
  allowed: number;
  throttled: number;
  blocked: number;
  byWindow: RateLimitBucket[];
  byRoute: Array<{
    routeId: string;
    routeName: string;
    allowed: number;
    throttled: number;
    blocked: number;
  }>;
  byConsumer: Array<{
    consumerId: string;
    consumerName: string;
    allowed: number;
    throttled: number;
    blocked: number;
  }>;
};

type RateLimitStatsCardProps = {
  data: RateLimitStats;
  className?: string;
};

export function RateLimitStatsCard({ data, className }: RateLimitStatsCardProps) {
  const total = data.totalRequests || 1;
  const allowedPct = (data.allowed / total) * 100;
  const throttledPct = (data.throttled / total) * 100;
  const blockedPct = (data.blocked / total) * 100;

  const topRoutes = useMemo(() => {
    return [...data.byRoute]
      .sort((a, b) => (b.throttled + b.blocked) - (a.throttled + a.blocked))
      .slice(0, 5);
  }, [data.byRoute]);

  const topConsumers = useMemo(() => {
    return [...data.byConsumer]
      .sort((a, b) => (b.throttled + b.blocked) - (a.throttled + a.blocked))
      .slice(0, 5);
  }, [data.byConsumer]);

  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Gauge className="h-5 w-5" />
            <CardTitle>Rate Limiting Statistics</CardTitle>
          </div>
          <Badge variant="outline">{data.totalRequests.toLocaleString()} total</Badge>
        </div>
        <CardDescription>Request throttling and blocking analysis</CardDescription>
      </CardHeader>

      <CardContent className="space-y-6">
        {/* Overview Stats */}
        <div className="grid grid-cols-3 gap-4">
          <StatCard
            icon={CheckCircle2}
            label="Allowed"
            value={data.allowed}
            percentage={allowedPct}
            color="success"
          />
          <StatCard
            icon={AlertTriangle}
            label="Throttled"
            value={data.throttled}
            percentage={throttledPct}
            color="amber"
          />
          <StatCard
            icon={XCircle}
            label="Blocked"
            value={data.blocked}
            percentage={blockedPct}
            color="destructive"
          />
        </div>

        {/* Time Window Breakdown */}
        {data.byWindow.length > 0 && (
          <div className="space-y-3">
            <h4 className="text-sm font-medium">By Time Window</h4>
            <div className="space-y-2">
              {data.byWindow.map((bucket) => {
                const bucketTotal = bucket.allowed + bucket.throttled + bucket.blocked;
                return (
                  <div key={bucket.window} className="space-y-1">
                    <div className="flex items-center justify-between text-sm">
                      <span className="font-medium">{bucket.window}</span>
                      <span className="text-muted-foreground">
                        {bucketTotal.toLocaleString()} requests
                      </span>
                    </div>
                    <div className="flex h-2 rounded-full overflow-hidden">
                      <div
                        className="bg-emerald-500"
                        style={{ width: `${(bucket.allowed / bucketTotal) * 100}%` }}
                      />
                      <div
                        className="bg-amber-500"
                        style={{ width: `${(bucket.throttled / bucketTotal) * 100}%` }}
                      />
                      <div
                        className="bg-destructive"
                        style={{ width: `${(bucket.blocked / bucketTotal) * 100}%` }}
                      />
                    </div>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground">
                      <span className="text-emerald-600">{bucket.allowed} allowed</span>
                      {bucket.throttled > 0 && (
                        <span className="text-amber-600">{bucket.throttled} throttled</span>
                      )}
                      {bucket.blocked > 0 && (
                        <span className="text-destructive">{bucket.blocked} blocked</span>
                      )}
                      <span className="ml-auto">avg {bucket.avgLatencyMs.toFixed(0)}ms</span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Top Affected Routes */}
        {topRoutes.length > 0 && (
          <div className="space-y-3">
            <h4 className="text-sm font-medium flex items-center gap-2">
              <Shield className="h-4 w-4" />
              Top Affected Routes
            </h4>
            <div className="space-y-2">
              {topRoutes.map((route) => {
                const routeTotal = route.allowed + route.throttled + route.blocked;
                const blockedRate = routeTotal > 0 ? ((route.throttled + route.blocked) / routeTotal) * 100 : 0;

                return (
                  <div key={route.routeId} className="flex items-center gap-3 p-2 rounded-lg bg-muted/50">
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-sm truncate">{route.routeName || route.routeId}</p>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{route.allowed} allowed</span>
                        {route.throttled > 0 && (
                          <span className="text-amber-600">{route.throttled} throttled</span>
                        )}
                        {route.blocked > 0 && (
                          <span className="text-destructive">{route.blocked} blocked</span>
                        )}
                      </div>
                    </div>
                    <Badge
                      variant={blockedRate > 10 ? "destructive" : blockedRate > 5 ? "secondary" : "outline"}
                    >
                      {blockedRate.toFixed(1)}%
                    </Badge>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Top Affected Consumers */}
        {topConsumers.length > 0 && (
          <div className="space-y-3">
            <h4 className="text-sm font-medium">Top Affected Consumers</h4>
            <div className="space-y-2">
              {topConsumers.map((consumer) => {
                const consumerTotal = consumer.allowed + consumer.throttled + consumer.blocked;
                const blockedRate = consumerTotal > 0 ? ((consumer.throttled + consumer.blocked) / consumerTotal) * 100 : 0;

                return (
                  <div key={consumer.consumerId} className="flex items-center gap-3 p-2 rounded-lg bg-muted/50">
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-sm truncate">{consumer.consumerName || consumer.consumerId}</p>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{consumer.allowed} allowed</span>
                        {consumer.throttled > 0 && (
                          <span className="text-amber-600">{consumer.throttled} throttled</span>
                        )}
                        {consumer.blocked > 0 && (
                          <span className="text-destructive">{consumer.blocked} blocked</span>
                        )}
                      </div>
                    </div>
                    <Badge
                      variant={blockedRate > 10 ? "destructive" : blockedRate > 5 ? "secondary" : "outline"}
                    >
                      {blockedRate.toFixed(1)}%
                    </Badge>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function StatCard({
  icon: Icon,
  label,
  value,
  percentage,
  color,
}: {
  icon: typeof CheckCircle2;
  label: string;
  value: number;
  percentage: number;
  color: "success" | "amber" | "destructive";
}) {
  const colorClasses = {
    success: "text-emerald-600 bg-emerald-500/10",
    amber: "text-amber-600 bg-amber-500/10",
    destructive: "text-destructive bg-destructive/10",
  };

  return (
    <div className={cn("p-3 rounded-lg border", colorClasses[color])}>
      <div className="flex items-center gap-2 mb-2">
        <Icon className="h-4 w-4" />
        <span className="text-sm font-medium">{label}</span>
      </div>
      <p className="text-2xl font-bold">{value.toLocaleString()}</p>
      <p className="text-xs opacity-80">{percentage.toFixed(1)}%</p>
    </div>
  );
}
