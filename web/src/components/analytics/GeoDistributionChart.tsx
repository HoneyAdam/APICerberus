import { useMemo } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Globe, MapPin } from "lucide-react";
import { cn } from "@/lib/utils";

export type GeoDataPoint = {
  country: string;
  countryCode: string;
  region?: string;
  city?: string;
  requests: number;
  errors: number;
  avgLatencyMs: number;
  uniqueIps: number;
};

type GeoDistributionChartProps = {
  data: GeoDataPoint[];
  className?: string;
  title?: string;
  description?: string;
  maxItems?: number;
};

// Simple world map data for visualization
const WORLD_REGIONS = [
  { code: "NA", name: "North America", countries: ["US", "CA", "MX"] },
  { code: "EU", name: "Europe", countries: ["GB", "DE", "FR", "IT", "ES", "NL", "BE", "CH", "AT", "SE", "NO", "DK", "FI", "PL", "CZ", "HU", "RO", "BG", "HR", "SI", "SK", "LT", "LV", "EE", "IE", "PT", "GR", "CY", "MT", "LU"] },
  { code: "AS", name: "Asia", countries: ["CN", "JP", "IN", "KR", "SG", "TH", "VN", "MY", "ID", "PH", "TW", "HK", "MO"] },
  { code: "SA", name: "South America", countries: ["BR", "AR", "CL", "CO", "PE", "VE", "EC", "UY", "PY", "BO"] },
  { code: "AF", name: "Africa", countries: ["ZA", "EG", "NG", "KE", "MA", "TN", "GH", "UG", "TZ", "ZW"] },
  { code: "OC", name: "Oceania", countries: ["AU", "NZ", "FJ", "PG", "NC", "VU", "SB"] },
];

// Country code to flag emoji
function getFlagEmoji(countryCode: string): string {
  const codePoints = countryCode
    .toUpperCase()
    .slice(0, 2)
    .split("")
    .map((char) => 127397 + char.charCodeAt(0));
  return String.fromCodePoint(...codePoints);
}

export function GeoDistributionChart({
  data,
  className,
  title = "Geographic Distribution",
  description = "Request distribution by country/region",
  maxItems = 10,
}: GeoDistributionChartProps) {
  const sortedData = useMemo(() => {
    return [...data].sort((a, b) => b.requests - a.requests).slice(0, maxItems);
  }, [data, maxItems]);

  const totalRequests = useMemo(() => data.reduce((sum, d) => sum + d.requests, 0), [data]);

  const regionStats = useMemo(() => {
    const stats: Record<string, { name: string; requests: number; countries: number }> = {};

    WORLD_REGIONS.forEach((region) => {
      stats[region.code] = { name: region.name, requests: 0, countries: 0 };
    });

    data.forEach((point) => {
      const region = WORLD_REGIONS.find((r) => r.countries.includes(point.countryCode));
      if (region) {
        stats[region.code].requests += point.requests;
        stats[region.code].countries += 1;
      }
    });

    return Object.entries(stats)
      .filter(([, stat]) => stat.requests > 0)
      .sort(([, a], [, b]) => b.requests - a.requests);
  }, [data]);

  const maxRequests = useMemo(() => {
    return Math.max(...sortedData.map((d) => d.requests), 1);
  }, [sortedData]);

  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Globe className="h-5 w-5" />
            <CardTitle>{title}</CardTitle>
          </div>
          <Badge variant="outline">{data.length} countries</Badge>
        </div>
        <CardDescription>{description}</CardDescription>
      </CardHeader>

      <CardContent className="space-y-6">
        {/* Region Summary */}
        <div className="grid grid-cols-3 gap-2">
          {regionStats.slice(0, 6).map(([code, stat]) => (
            <div
              key={code}
              className="p-2 rounded-lg border bg-muted/50 text-center"
            >
              <p className="text-xs text-muted-foreground">{stat.name}</p>
              <p className="text-lg font-semibold">{stat.requests.toLocaleString()}</p>
              <p className="text-xs text-muted-foreground">
                {((stat.requests / totalRequests) * 100).toFixed(1)}%
              </p>
            </div>
          ))}
        </div>

        {/* Country List */}
        <div className="space-y-2">
          <h4 className="text-sm font-medium flex items-center gap-2">
            <MapPin className="h-4 w-4" />
            Top Countries
          </h4>
          <div className="space-y-2">
            {sortedData.map((item) => {
              const percentage = (item.requests / totalRequests) * 100;
              const barWidth = (item.requests / maxRequests) * 100;
              const errorRate = item.requests > 0 ? (item.errors / item.requests) * 100 : 0;

              return (
                <div
                  key={item.countryCode}
                  className="group relative p-2 rounded-lg hover:bg-muted/50 transition-colors"
                >
                  {/* Progress bar background */}
                  <div
                    className="absolute left-0 top-0 bottom-0 bg-primary/5 rounded-lg transition-all"
                    style={{ width: `${barWidth}%` }}
                  />

                  <div className="relative flex items-center gap-3">
                    <span className="text-2xl" title={item.country}>
                      {getFlagEmoji(item.countryCode)}
                    </span>

                    <div className="flex-1 min-w-0">
                      <div className="flex items-center justify-between">
                        <span className="font-medium truncate">{item.country}</span>
                        <span className="text-sm text-muted-foreground">
                          {item.requests.toLocaleString()} ({percentage.toFixed(1)}%)
                        </span>
                      </div>

                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>avg {item.avgLatencyMs.toFixed(0)}ms</span>
                        <span>•</span>
                        <span>{item.uniqueIps} unique IPs</span>
                        {errorRate > 0 && (
                          <>
                            <span>•</span>
                            <span className={errorRate > 5 ? "text-destructive" : "text-amber-600"}>
                              {errorRate.toFixed(1)}% errors
                            </span>
                          </>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {sortedData.length === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            <Globe className="h-12 w-12 mx-auto mb-2 opacity-20" />
            <p>No geographic data available</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// Simplified world map visualization component
export function WorldMapVisualization({
  data,
  className,
}: {
  data: GeoDataPoint[];
  className?: string;
}) {
  const maxRequests = useMemo(() => {
    return Math.max(...data.map((d) => d.requests), 1);
  }, [data]);

  const getIntensity = (countryCode: string) => {
    const countryData = data.find((d) => d.countryCode === countryCode);
    if (!countryData) return 0;
    return countryData.requests / maxRequests;
  };

  return (
    <div className={cn("relative", className)}>
      <svg
        viewBox="0 0 1000 500"
        className="w-full h-auto"
        style={{ background: "hsl(var(--muted))" }}
      >
        {/* Simplified world map dots */}
        {WORLD_REGIONS.map((region) => {
          return (
            <g key={region.code}>
              {region.countries.map((country, idx) => {
                const countryIntensity = getIntensity(country);
                // Generate pseudo-random positions for demo
                const x = (idx * 37 + region.code.charCodeAt(0) * 13) % 900 + 50;
                const y = (idx * 23 + region.code.charCodeAt(0) * 7) % 400 + 50;

                return (
                  <circle
                    key={country}
                    cx={x}
                    cy={y}
                    r={3 + countryIntensity * 8}
                    fill={`hsl(var(--primary) / ${0.2 + countryIntensity * 0.8})`}
                    className="transition-all duration-500"
                  />
                );
              })}
            </g>
          );
        })}
      </svg>

      {/* Legend */}
      <div className="absolute bottom-2 right-2 bg-background/90 backdrop-blur p-2 rounded-lg border text-xs">
        <div className="flex items-center gap-2">
          <span>Low</span>
          <div className="w-20 h-2 rounded-full bg-gradient-to-r from-primary/20 to-primary" />
          <span>High</span>
        </div>
      </div>
    </div>
  );
}
