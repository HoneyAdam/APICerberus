import { useEffect, useMemo, useRef, useState } from "react";
import { useRealtime } from "@/hooks/use-realtime";
import { useMediaQuery } from "@/hooks/use-media-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { TimeAgo } from "@/components/shared/TimeAgo";
import {
  Play,
  Pause,
  Trash2,
  Download,
  Filter,
  Terminal,
  Calendar as CalendarIcon,
  X,
  FileJson,
  FileSpreadsheet,
  Search,
  
  Copy,
  Check,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { format, isWithinInterval, parseISO } from "date-fns";

export type LogLevel = "debug" | "info" | "warn" | "error" | "fatal";
export type LogSource = "gateway" | "admin" | "plugin" | "raft" | "audit" | "system";

export interface LogEntry {
  id: string;
  timestamp: string;
  level: LogLevel;
  source: LogSource;
  message: string;
  metadata?: Record<string, unknown>;
  requestId?: string;
  routeId?: string;
  routePath?: string;
  consumerId?: string;
  consumerName?: string;
  serviceId?: string;
  serviceName?: string;
  upstreamId?: string;
  statusCode?: number;
  method?: string;
  path?: string;
  latency?: number;
  userAgent?: string;
  clientIp?: string;
  error?: string;
  stackTrace?: string;
}

interface AdvancedLogViewerProps {
  maxLogs?: number;
  className?: string;
  showFilters?: boolean;
  showExport?: boolean;
  height?: string;
  title?: string;
  enableRealTime?: boolean;
  wsUrl?: string;
}

const LEVEL_COLORS: Record<LogLevel, string> = {
  debug: "bg-slate-500/10 text-slate-600 border-slate-500/20 dark:bg-slate-500/20 dark:text-slate-400",
  info: "bg-blue-500/10 text-blue-600 border-blue-500/20 dark:bg-blue-500/20 dark:text-blue-400",
  warn: "bg-amber-500/10 text-amber-600 border-amber-500/20 dark:bg-amber-500/20 dark:text-amber-400",
  error: "bg-red-500/10 text-red-600 border-red-500/20 dark:bg-red-500/20 dark:text-red-400",
  fatal: "bg-purple-500/10 text-purple-600 border-purple-500/20 dark:bg-purple-500/20 dark:text-purple-400",
};

const LEVEL_BADGE_VARIANTS: Record<LogLevel, "default" | "secondary" | "destructive" | "outline"> = {
  debug: "outline",
  info: "default",
  warn: "secondary",
  error: "destructive",
  fatal: "destructive",
};

const SOURCE_COLORS: Record<LogSource, string> = {
  gateway: "text-emerald-600 dark:text-emerald-400",
  admin: "text-blue-600 dark:text-blue-400",
  plugin: "text-amber-600 dark:text-amber-400",
  raft: "text-purple-600 dark:text-purple-400",
  audit: "text-pink-600 dark:text-pink-400",
  system: "text-slate-600 dark:text-slate-400",
};

const SOURCE_LABELS: Record<LogSource, string> = {
  gateway: "GW",
  admin: "ADM",
  plugin: "PLG",
  raft: "RAFT",
  audit: "AUD",
  system: "SYS",
};

function getStatusCodeColor(code: number): string {
  if (code >= 200 && code < 300) return "text-emerald-600 dark:text-emerald-400";
  if (code >= 300 && code < 400) return "text-blue-600 dark:text-blue-400";
  if (code >= 400 && code < 500) return "text-amber-600 dark:text-amber-400";
  if (code >= 500) return "text-red-600 dark:text-red-400";
  return "text-slate-600 dark:text-slate-400";
}

function formatDuration(ms: number): string {
  if (ms < 1) return "<1ms";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export function AdvancedLogViewer({
  maxLogs = 1000,
  className,
  showFilters = true,
  showExport = true,
  height = "h-[600px]",
  title = "Log Viewer",
  enableRealTime = true,
  wsUrl,
}: AdvancedLogViewerProps) {
  const isMobile = useMediaQuery("(max-width: 768px)");
  const [isPaused, setIsPaused] = useState(false);
  const [selectedLevels, setSelectedLevels] = useState<LogLevel[]>([]);
  const [selectedSources, setSelectedSources] = useState<LogSource[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [routeFilter, setRouteFilter] = useState("");
  const [consumerFilter, setConsumerFilter] = useState("");
  const [statusCodeFilter, setStatusCodeFilter] = useState<string>("");
  const [dateRange, setDateRange] = useState<{ from?: Date; to?: Date }>({});
  const [autoScroll, setAutoScroll] = useState(true);
  const [selectedLog, setSelectedLog] = useState<LogEntry | null>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
    const [copiedId, setCopiedId] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [showFiltersPanel, setShowFiltersPanel] = useState(!isMobile);

  const realtime = useRealtime({
    autoConnect: enableRealTime,
    url: wsUrl,
    eventTailSize: maxLogs,
  });

  // Convert realtime events to log entries
  useEffect(() => {
    if (isPaused || !enableRealTime) return;

    const newLogs: LogEntry[] = realtime.eventTail.map((event, index) => {
      const payload = event.payload as Record<string, unknown> | undefined;
      return {
        id: `log-${event.timestamp}-${index}`,
        timestamp: event.timestamp,
        level: (payload?.level as LogLevel) || getLevelFromEventType(event.type),
        source: (payload?.source as LogSource) || getSourceFromEventType(event.type),
        message: (payload?.message as string) || JSON.stringify(event.payload),
        metadata: payload?.metadata as Record<string, unknown> | undefined,
        requestId: payload?.request_id as string | undefined,
        routeId: payload?.route_id as string | undefined,
        routePath: payload?.route_path as string | undefined,
        consumerId: payload?.consumer_id as string | undefined,
        consumerName: payload?.consumer_name as string | undefined,
        serviceId: payload?.service_id as string | undefined,
        serviceName: payload?.service_name as string | undefined,
        upstreamId: payload?.upstream_id as string | undefined,
        statusCode: payload?.status_code as number | undefined,
        method: payload?.method as string | undefined,
        path: payload?.path as string | undefined,
        latency: payload?.latency_ms as number | undefined,
        userAgent: payload?.user_agent as string | undefined,
        clientIp: payload?.client_ip as string | undefined,
        error: payload?.error as string | undefined,
        stackTrace: payload?.stack_trace as string | undefined,
      };
    });

    setLogs((prev) => {
      const combined = [...prev, ...newLogs];
      const unique = combined.filter(
        (log, index, self) => index === self.findIndex((l) => l.id === log.id)
      );
      return unique.slice(-maxLogs);
    });
  }, [realtime.eventTail, isPaused, maxLogs, enableRealTime]);

  // Auto-scroll to bottom
  useEffect(() => {
    if (autoScroll && scrollRef.current && !isPaused) {
      const viewport = scrollRef.current.querySelector<HTMLElement>("[data-slot='scroll-area-viewport']");
      if (viewport) {
        viewport.scrollTop = viewport.scrollHeight;
      }
    }
  }, [logs, autoScroll, isPaused]);

  const filteredLogs = useMemo(() => {
    return logs.filter((log) => {
      // Level filter
      if (selectedLevels.length > 0 && !selectedLevels.includes(log.level)) return false;

      // Source filter
      if (selectedSources.length > 0 && !selectedSources.includes(log.source)) return false;

      // Search query
      if (searchQuery) {
        const search = searchQuery.toLowerCase();
        const matchesMessage = log.message.toLowerCase().includes(search);
        const matchesRequestId = log.requestId?.toLowerCase().includes(search);
        const matchesRouteId = log.routeId?.toLowerCase().includes(search);
        const matchesConsumer = log.consumerId?.toLowerCase().includes(search);
        const matchesPath = log.path?.toLowerCase().includes(search);
        const matchesError = log.error?.toLowerCase().includes(search);
        if (!matchesMessage && !matchesRequestId && !matchesRouteId && !matchesConsumer && !matchesPath && !matchesError) {
          return false;
        }
      }

      // Route filter
      if (routeFilter && !log.routePath?.toLowerCase().includes(routeFilter.toLowerCase()) &&
          !log.routeId?.toLowerCase().includes(routeFilter.toLowerCase())) {
        return false;
      }

      // Consumer filter
      if (consumerFilter && !log.consumerName?.toLowerCase().includes(consumerFilter.toLowerCase()) &&
          !log.consumerId?.toLowerCase().includes(consumerFilter.toLowerCase())) {
        return false;
      }

      // Status code filter
      if (statusCodeFilter) {
        const code = log.statusCode;
        if (!code) return false;
        switch (statusCodeFilter) {
          case "2xx":
            if (code < 200 || code >= 300) return false;
            break;
          case "3xx":
            if (code < 300 || code >= 400) return false;
            break;
          case "4xx":
            if (code < 400 || code >= 500) return false;
            break;
          case "5xx":
            if (code < 500) return false;
            break;
          default:
            if (code !== parseInt(statusCodeFilter, 10)) return false;
        }
      }

      // Date range filter
      if (dateRange.from || dateRange.to) {
        const logDate = parseISO(log.timestamp);
        if (dateRange.from && dateRange.to) {
          if (!isWithinInterval(logDate, { start: dateRange.from, end: dateRange.to })) {
            return false;
          }
        } else if (dateRange.from && logDate < dateRange.from) {
          return false;
        } else if (dateRange.to && logDate > dateRange.to) {
          return false;
        }
      }

      return true;
    });
  }, [logs, selectedLevels, selectedSources, searchQuery, routeFilter, consumerFilter, statusCodeFilter, dateRange]);

  const stats = useMemo(() => {
    const total = logs.length;
    const filtered = filteredLogs.length;
    const errors = logs.filter((l) => l.level === "error" || l.level === "fatal").length;
    const warnings = logs.filter((l) => l.level === "warn").length;
    const avgLatency = logs.filter((l) => l.latency).reduce((acc, l) => acc + (l.latency || 0), 0) / logs.filter((l) => l.latency).length || 0;

    return { total, filtered, errors, warnings, avgLatency };
  }, [logs, filteredLogs]);

  const toggleLevel = (level: LogLevel) => {
    setSelectedLevels((prev) =>
      prev.includes(level) ? prev.filter((l) => l !== level) : [...prev, level]
    );
  };

  const toggleSource = (source: LogSource) => {
    setSelectedSources((prev) =>
      prev.includes(source) ? prev.filter((s) => s !== source) : [...prev, source]
    );
  };

  const handleClear = () => {
    setLogs([]);
    realtime.clear();
  };

  const handleExportJSON = () => {
    const data = JSON.stringify(filteredLogs, null, 2);
    const blob = new Blob([data], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `logs-${format(new Date(), "yyyy-MM-dd-HHmmss")}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  const handleExportCSV = () => {
    const headers = [
      "timestamp",
      "level",
      "source",
      "message",
      "requestId",
      "routeId",
      "routePath",
      "consumerId",
      "consumerName",
      "serviceId",
      "statusCode",
      "method",
      "path",
      "latency",
      "clientIp",
      "error",
    ];
    const rows = filteredLogs.map((log) => [
      log.timestamp,
      log.level,
      log.source,
      `"${log.message.replace(/"/g, '""')}"`,
      log.requestId || "",
      log.routeId || "",
      log.routePath || "",
      log.consumerId || "",
      log.consumerName || "",
      log.serviceId || "",
      log.statusCode || "",
      log.method || "",
      log.path || "",
      log.latency || "",
      log.clientIp || "",
      log.error ? `"${log.error.replace(/"/g, '""')}"` : "",
    ]);
    const csv = [headers.join(","), ...rows.map((r) => r.join(","))].join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `logs-${format(new Date(), "yyyy-MM-dd-HHmmss")}.csv`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

    const activeFiltersCount = selectedLevels.length + selectedSources.length +
    (routeFilter ? 1 : 0) + (consumerFilter ? 1 : 0) + (statusCodeFilter ? 1 : 0) +
    (dateRange.from || dateRange.to ? 1 : 0);

  return (
    <Card className={className}>
      <CardHeader className="pb-3 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
              <Terminal className="h-5 w-5 text-primary" />
            </div>
            <div>
              <CardTitle>{title}</CardTitle>
              <CardDescription>
                {stats.filtered.toLocaleString()} of {stats.total.toLocaleString()} logs
                {stats.errors > 0 && (
                  <span className="ml-2 text-destructive">{stats.errors} errors</span>
                )}
              </CardDescription>
            </div>
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            <Badge variant={realtime.connected ? "default" : "secondary"}>
              {realtime.connected ? "● Live" : "○ Disconnected"}
            </Badge>

            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={() => setIsPaused(!isPaused)}
                  >
                    {isPaused ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{isPaused ? "Resume streaming" : "Pause streaming"}</TooltipContent>
              </Tooltip>
            </TooltipProvider>

            {showExport && (
              <Popover>
                <PopoverTrigger asChild>
                  <Button variant="outline" size="icon">
                    <Download className="h-4 w-4" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-48" align="end">
                  <div className="space-y-2">
                    <p className="text-sm font-medium">Export Logs</p>
                    <Button variant="outline" size="sm" className="w-full justify-start" onClick={handleExportJSON}>
                      <FileJson className="h-4 w-4 mr-2" />
                      Export as JSON
                    </Button>
                    <Button variant="outline" size="sm" className="w-full justify-start" onClick={handleExportCSV}>
                      <FileSpreadsheet className="h-4 w-4 mr-2" />
                      Export as CSV
                    </Button>
                  </div>
                </PopoverContent>
              </Popover>
            )}

            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="outline" size="icon" onClick={handleClear}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Clear logs</TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowFiltersPanel(!showFiltersPanel)}
              className={cn(activeFiltersCount > 0 && "border-primary text-primary")}
            >
              <Filter className="h-4 w-4 mr-2" />
              Filters
              {activeFiltersCount > 0 && (
                <Badge variant="secondary" className="ml-2 h-5 w-5 p-0 text-xs">
                  {activeFiltersCount}
                </Badge>
              )}
            </Button>
          </div>
        </div>

        {showFilters && showFiltersPanel && (
          <div className="space-y-4 pt-2 border-t">
            {/* Search and Quick Filters */}
            <div className="flex flex-col lg:flex-row gap-4">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search logs..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-9"
                />
                {searchQuery && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6"
                    onClick={() => setSearchQuery("")}
                  >
                    <X className="h-3 w-3" />
                  </Button>
                )}
              </div>

              <div className="flex gap-2 flex-wrap">
                <Select value={statusCodeFilter} onValueChange={setStatusCodeFilter}>
                  <SelectTrigger className="w-32">
                    <SelectValue placeholder="Status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="">All Status</SelectItem>
                    <SelectItem value="2xx">2xx Success</SelectItem>
                    <SelectItem value="3xx">3xx Redirect</SelectItem>
                    <SelectItem value="4xx">4xx Client Error</SelectItem>
                    <SelectItem value="5xx">5xx Server Error</SelectItem>
                  </SelectContent>
                </Select>

                <Popover>
                  <PopoverTrigger asChild>
                    <Button variant="outline" className={cn("w-auto", dateRange.from && "border-primary")}>
                      <CalendarIcon className="h-4 w-4 mr-2" />
                      {dateRange.from ? (
                        dateRange.to ? (
                          <>
                            {format(dateRange.from, "MMM d")} - {format(dateRange.to, "MMM d")}
                          </>
                        ) : (
                          format(dateRange.from, "MMM d, yyyy")
                        )
                      ) : (
                        "Date Range"
                      )}
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-auto p-0" align="end">
                    <Calendar
                      initialFocus
                      mode="range"
                      selected={{
                        from: dateRange.from,
                        to: dateRange.to,
                      }}
                      onSelect={(range) => {
                        setDateRange({
                          from: range?.from,
                          to: range?.to,
                        });
                      }}
                      numberOfMonths={2}
                    />
                  </PopoverContent>
                </Popover>

                <div className="flex items-center gap-2">
                  <Switch
                    id="auto-scroll"
                    checked={autoScroll}
                    onCheckedChange={setAutoScroll}
                  />
                  <label htmlFor="auto-scroll" className="text-sm text-muted-foreground cursor-pointer">
                    Auto-scroll
                  </label>
                </div>
              </div>
            </div>

            {/* Level and Source Filters */}
            <div className="flex flex-col lg:flex-row gap-4">
              <div className="flex-1">
                <p className="text-sm font-medium mb-2">Levels</p>
                <div className="flex flex-wrap gap-1">
                  {(["debug", "info", "warn", "error", "fatal"] as LogLevel[]).map((level) => (
                    <Button
                      key={level}
                      variant={selectedLevels.includes(level) ? "default" : "outline"}
                      size="sm"
                      className={cn(
                        "text-xs capitalize",
                        selectedLevels.includes(level) && LEVEL_COLORS[level]
                      )}
                      onClick={() => toggleLevel(level)}
                    >
                      {level}
                    </Button>
                  ))}
                </div>
              </div>

              <div className="flex-1">
                <p className="text-sm font-medium mb-2">Sources</p>
                <div className="flex flex-wrap gap-1">
                  {(["gateway", "admin", "plugin", "raft", "audit", "system"] as LogSource[]).map((source) => (
                    <Button
                      key={source}
                      variant={selectedSources.includes(source) ? "default" : "outline"}
                      size="sm"
                      className="text-xs capitalize"
                      onClick={() => toggleSource(source)}
                    >
                      {source}
                    </Button>
                  ))}
                </div>
              </div>
            </div>

            {/* Advanced Filters */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="relative">
                <Input
                  placeholder="Filter by route..."
                  value={routeFilter}
                  onChange={(e) => setRouteFilter(e.target.value)}
                />
                {routeFilter && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6"
                    onClick={() => setRouteFilter("")}
                  >
                    <X className="h-3 w-3" />
                  </Button>
                )}
              </div>
              <div className="relative">
                <Input
                  placeholder="Filter by consumer..."
                  value={consumerFilter}
                  onChange={(e) => setConsumerFilter(e.target.value)}
                />
                {consumerFilter && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6"
                    onClick={() => setConsumerFilter("")}
                  >
                    <X className="h-3 w-3" />
                  </Button>
                )}
              </div>
            </div>

            {activeFiltersCount > 0 && (
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setSelectedLevels([]);
                    setSelectedSources([]);
                    setSearchQuery("");
                    setRouteFilter("");
                    setConsumerFilter("");
                    setStatusCodeFilter("");
                    setDateRange({});
                  }}
                >
                  <X className="h-3 w-3 mr-1" />
                  Clear all filters
                </Button>
              </div>
            )}
          </div>
        )}
      </CardHeader>

      <CardContent>
        <ScrollArea ref={scrollRef} className={cn("rounded-md border", height)}>
          <div className="min-w-[800px]">
            {/* Header Row */}
            <div className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-muted/50 border-b text-xs font-medium text-muted-foreground">
              <div className="w-16">Level</div>
              <div className="w-12">Src</div>
              <div className="w-28">Time</div>
              <div className="w-16">Status</div>
              <div className="w-16">Method</div>
              <div className="flex-1 min-w-0">Message</div>
              <div className="w-20 text-right">Latency</div>
            </div>

            {/* Log Rows */}
            <div className="divide-y">
              {filteredLogs.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-64 text-center px-4">
                  <Terminal className="h-12 w-12 text-muted-foreground mb-4" />
                  <p className="text-muted-foreground">No logs match your filters</p>
                  {activeFiltersCount > 0 && (
                    <Button
                      variant="link"
                      size="sm"
                      onClick={() => {
                        setSelectedLevels([]);
                        setSelectedSources([]);
                        setSearchQuery("");
                        setRouteFilter("");
                        setConsumerFilter("");
                        setStatusCodeFilter("");
                        setDateRange({});
                      }}
                    >
                      Clear filters
                    </Button>
                  )}
                </div>
              ) : (
                filteredLogs.map((log) => (
                  <div
                    key={log.id}
                    className={cn(
                      "flex items-center gap-2 px-4 py-2 hover:bg-muted/50 cursor-pointer transition-colors",
                      log.level === "error" && "bg-red-500/5",
                      log.level === "warn" && "bg-amber-500/5",
                      selectedLog?.id === log.id && "bg-primary/5"
                    )}
                    onClick={() => setSelectedLog(log)}
                  >
                    <div className="w-16">
                      <Badge
                        variant={LEVEL_BADGE_VARIANTS[log.level]}
                        className={cn("text-[10px] uppercase", LEVEL_COLORS[log.level])}
                      >
                        {log.level}
                      </Badge>
                    </div>

                    <div className={cn("w-12 text-xs font-mono", SOURCE_COLORS[log.source])}>
                      {SOURCE_LABELS[log.source]}
                    </div>

                    <div className="w-28 text-xs text-muted-foreground">
                      <TimeAgo value={log.timestamp} />
                    </div>

                    <div className="w-16">
                      {log.statusCode && (
                        <span className={cn("text-xs font-mono", getStatusCodeColor(log.statusCode))}>
                          {log.statusCode}
                        </span>
                      )}
                    </div>

                    <div className="w-16">
                      {log.method && (
                        <Badge variant="outline" className="text-[10px]">
                          {log.method}
                        </Badge>
                      )}
                    </div>

                    <div className="flex-1 min-w-0">
                      <p className={cn(
                        "text-sm truncate",
                        log.level === "error" && "text-destructive",
                        log.level === "warn" && "text-amber-600 dark:text-amber-400"
                      )}>
                        {log.message}
                      </p>
                      {(log.routePath || log.consumerName) && (
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          {log.routePath && <span>{log.routePath}</span>}
                          {log.consumerName && (
                            <>
                              <span>•</span>
                              <span>{log.consumerName}</span>
                            </>
                          )}
                        </div>
                      )}
                    </div>

                    <div className="w-20 text-right">
                      {log.latency !== undefined && (
                        <span className="text-xs text-muted-foreground">
                          {formatDuration(log.latency)}
                        </span>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </ScrollArea>

        {isPaused && (
          <div className="mt-2 flex items-center justify-center gap-2 text-sm text-amber-600">
            <Pause className="h-4 w-4" />
            Log streaming paused. Click play to resume.
          </div>
        )}
      </CardContent>

      {/* Log Detail Modal */}
      <Dialog open={Boolean(selectedLog)} onOpenChange={() => setSelectedLog(null)}>
        <DialogContent className="max-w-4xl max-h-[90vh] overflow-hidden">
          {selectedLog && (
            <>
              <DialogHeader>
                <div className="flex items-center gap-2">
                  <Badge
                    variant={LEVEL_BADGE_VARIANTS[selectedLog.level]}
                    className={cn("uppercase", LEVEL_COLORS[selectedLog.level])}
                  >
                    {selectedLog.level}
                  </Badge>
                  <span className={cn("text-sm font-mono", SOURCE_COLORS[selectedLog.source])}>
                    {selectedLog.source}
                  </span>
                  <span className="text-sm text-muted-foreground">
                    {format(parseISO(selectedLog.timestamp), "MMM d, yyyy HH:mm:ss.SSS")}
                  </span>
                </div>
                <DialogTitle className="text-lg mt-2">{selectedLog.message}</DialogTitle>
                {selectedLog.error && (
                  <DialogDescription className="text-destructive">
                    {selectedLog.error}
                  </DialogDescription>
                )}
              </DialogHeader>

              <ScrollArea className="max-h-[60vh]">
                <div className="space-y-4 pr-4">
                  {/* Request Details */}
                  {(selectedLog.method || selectedLog.path || selectedLog.statusCode) && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Request Details</h4>
                      <div className="grid grid-cols-2 gap-2 text-sm">
                        {selectedLog.method && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Method</span>
                            <Badge variant="outline">{selectedLog.method}</Badge>
                          </div>
                        )}
                        {selectedLog.path && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Path</span>
                            <span className="font-mono">{selectedLog.path}</span>
                          </div>
                        )}
                        {selectedLog.statusCode && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Status</span>
                            <span className={cn("font-mono", getStatusCodeColor(selectedLog.statusCode))}>
                              {selectedLog.statusCode}
                            </span>
                          </div>
                        )}
                        {selectedLog.latency !== undefined && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Latency</span>
                            <span>{formatDuration(selectedLog.latency)}</span>
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Routing Info */}
                  {(selectedLog.routeId || selectedLog.serviceId || selectedLog.consumerId || selectedLog.upstreamId) && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Routing</h4>
                      <div className="space-y-1 text-sm">
                        {selectedLog.routeId && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Route</span>
                            <div className="flex items-center gap-2">
                              <span className="font-mono text-xs">{selectedLog.routeId}</span>
                              {selectedLog.routePath && (
                                <span className="text-muted-foreground">({selectedLog.routePath})</span>
                              )}
                            </div>
                          </div>
                        )}
                        {selectedLog.serviceId && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Service</span>
                            <div className="flex items-center gap-2">
                              <span className="font-mono text-xs">{selectedLog.serviceId}</span>
                              {selectedLog.serviceName && (
                                <span className="text-muted-foreground">({selectedLog.serviceName})</span>
                              )}
                            </div>
                          </div>
                        )}
                        {selectedLog.consumerId && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Consumer</span>
                            <div className="flex items-center gap-2">
                              <span className="font-mono text-xs">{selectedLog.consumerId}</span>
                              {selectedLog.consumerName && (
                                <span className="text-muted-foreground">({selectedLog.consumerName})</span>
                              )}
                            </div>
                          </div>
                        )}
                        {selectedLog.upstreamId && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">Upstream</span>
                            <span className="font-mono text-xs">{selectedLog.upstreamId}</span>
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Client Info */}
                  {(selectedLog.clientIp || selectedLog.userAgent) && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Client</h4>
                      <div className="space-y-1 text-sm">
                        {selectedLog.clientIp && (
                          <div className="flex justify-between">
                            <span className="text-muted-foreground">IP Address</span>
                            <span className="font-mono">{selectedLog.clientIp}</span>
                          </div>
                        )}
                        {selectedLog.userAgent && (
                          <div>
                            <span className="text-muted-foreground">User Agent</span>
                            <p className="text-xs mt-1 break-all">{selectedLog.userAgent}</p>
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Error Details */}
                  {selectedLog.error && (
                    <div>
                      <h4 className="text-sm font-medium mb-2 text-destructive">Error</h4>
                      <div className="bg-destructive/5 border border-destructive/20 rounded-md p-3">
                        <p className="text-sm text-destructive">{selectedLog.error}</p>
                        {selectedLog.stackTrace && (
                          <pre className="mt-2 text-xs text-destructive/80 overflow-x-auto">
                            {selectedLog.stackTrace}
                          </pre>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Request ID */}
                  {selectedLog.requestId && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Request ID</h4>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 bg-muted px-2 py-1 rounded text-xs font-mono">
                          {selectedLog.requestId}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          onClick={() => copyToClipboard(selectedLog.requestId!, `req-${selectedLog.id}`)}
                        >
                          {copiedId === `req-${selectedLog.id}` ? (
                            <Check className="h-4 w-4 text-green-500" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    </div>
                  )}

                  {/* Metadata */}
                  {selectedLog.metadata && Object.keys(selectedLog.metadata).length > 0 && (
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <h4 className="text-sm font-medium">Metadata</h4>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => copyToClipboard(JSON.stringify(selectedLog.metadata, null, 2), `meta-${selectedLog.id}`)}
                        >
                          {copiedId === `meta-${selectedLog.id}` ? (
                            <>
                              <Check className="h-3 w-3 mr-1" />
                              Copied
                            </>
                          ) : (
                            <>
                              <Copy className="h-3 w-3 mr-1" />
                              Copy JSON
                            </>
                          )}
                        </Button>
                      </div>
                      <pre className="text-xs bg-muted p-3 rounded-md overflow-x-auto">
                        {JSON.stringify(selectedLog.metadata, null, 2)}
                      </pre>
                    </div>
                  )}

                  {/* Raw JSON */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <h4 className="text-sm font-medium">Raw Log Entry</h4>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => copyToClipboard(JSON.stringify(selectedLog, null, 2), `raw-${selectedLog.id}`)}
                      >
                        {copiedId === `raw-${selectedLog.id}` ? (
                          <>
                            <Check className="h-3 w-3 mr-1" />
                            Copied
                          </>
                        ) : (
                          <>
                            <Copy className="h-3 w-3 mr-1" />
                            Copy JSON
                          </>
                        )}
                      </Button>
                    </div>
                    <pre className="text-xs bg-muted p-3 rounded-md overflow-x-auto">
                      {JSON.stringify(selectedLog, null, 2)}
                    </pre>
                  </div>
                </div>
              </ScrollArea>

              <div className="flex justify-end gap-2 pt-4 border-t">
                <Button variant="outline" onClick={() => setSelectedLog(null)}>
                  Close
                </Button>
                <Button
                  onClick={() => {
                    if (selectedLog.requestId) {
                      setSearchQuery(selectedLog.requestId);
                      setSelectedLog(null);
                    }
                  }}
                  disabled={!selectedLog.requestId}
                >
                  Find Related Logs
                </Button>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function getLevelFromEventType(type: string): LogLevel {
  if (type.includes("fatal")) return "fatal";
  if (type.includes("error")) return "error";
  if (type.includes("warn")) return "warn";
  if (type.includes("debug")) return "debug";
  return "info";
}

function getSourceFromEventType(type: string): LogSource {
  if (type.includes("gateway")) return "gateway";
  if (type.includes("admin")) return "admin";
  if (type.includes("plugin")) return "plugin";
  if (type.includes("raft")) return "raft";
  if (type.includes("audit")) return "audit";
  return "system";
}
