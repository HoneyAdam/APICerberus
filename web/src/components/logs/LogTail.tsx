import { useEffect, useMemo, useRef, useState } from "react";
import { useRealtime } from "@/hooks/use-realtime";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { TimeAgo } from "@/components/shared/TimeAgo";
import { Play, Pause, Trash2, Download, Filter, Terminal } from "lucide-react";
import { cn } from "@/lib/utils";

export type LogLevel = "all" | "info" | "warn" | "error" | "debug";
export type LogSource = "all" | "gateway" | "admin" | "plugin" | "raft";

export type LogEntry = {
  id: string;
  timestamp: string;
  level: LogLevel;
  source: LogSource;
  message: string;
  metadata?: Record<string, unknown>;
  requestId?: string;
  routeId?: string;
  userId?: string;
};

type LogTailProps = {
  maxLogs?: number;
  className?: string;
  showFilters?: boolean;
  showExport?: boolean;
  height?: string;
  title?: string;
};

function getLevelColor(level: LogLevel): string {
  switch (level) {
    case "error":
      return "bg-destructive/10 text-destructive border-destructive/20";
    case "warn":
      return "bg-amber-500/10 text-amber-700 border-amber-500/20";
    case "info":
      return "bg-blue-500/10 text-blue-700 border-blue-500/20";
    case "debug":
      return "bg-slate-500/10 text-slate-700 border-slate-500/20";
    default:
      return "bg-muted text-muted-foreground";
  }
}

function getSourceIcon(source: LogSource): string {
  switch (source) {
    case "gateway":
      return "GW";
    case "admin":
      return "AD";
    case "plugin":
      return "PL";
    case "raft":
      return "RA";
    default:
      return "??";
  }
}

export function LogTail({
  maxLogs = 500,
  className,
  showFilters = true,
  showExport = true,
  height = "h-[500px]",
  title = "Live Logs",
}: LogTailProps) {
  const [isPaused, setIsPaused] = useState(false);
  const [levelFilter, setLevelFilter] = useState<LogLevel>("all");
  const [sourceFilter, setSourceFilter] = useState<LogSource>("all");
  const [searchFilter, setSearchFilter] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [showMetadata, setShowMetadata] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);

  const realtime = useRealtime({ autoConnect: true, eventTailSize: maxLogs });

  // Convert realtime events to log entries
  useEffect(() => {
    if (isPaused) return;

    const newLogs: LogEntry[] = realtime.eventTail.map((event, index) => {
      const payload = event.payload as Record<string, unknown> | undefined;
      return {
        id: `log-${event.timestamp}-${index}`,
        timestamp: event.timestamp,
        level: (payload?.level as LogLevel) || getLevelFromEventType(event.type),
        source: (payload?.source as LogSource) || getSourceFromEventType(event.type),
        message: payload?.message as string || JSON.stringify(event.payload),
        metadata: payload?.metadata as Record<string, unknown> | undefined,
        requestId: payload?.request_id as string | undefined,
        routeId: payload?.route_id as string | undefined,
        userId: payload?.user_id as string | undefined,
      };
    });

    setLogs((prev) => {
      const combined = [...prev, ...newLogs];
      // Remove duplicates based on id
      const unique = combined.filter((log, index, self) =>
        index === self.findIndex((l) => l.id === log.id)
      );
      // Keep only maxLogs
      return unique.slice(-maxLogs);
    });
  }, [realtime.eventTail, isPaused, maxLogs]);

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
      if (levelFilter !== "all" && log.level !== levelFilter) return false;
      if (sourceFilter !== "all" && log.source !== sourceFilter) return false;
      if (searchFilter) {
        const search = searchFilter.toLowerCase();
        const matchesMessage = log.message.toLowerCase().includes(search);
        const matchesRequestId = log.requestId?.toLowerCase().includes(search);
        const matchesRouteId = log.routeId?.toLowerCase().includes(search);
        const matchesUserId = log.userId?.toLowerCase().includes(search);
        if (!matchesMessage && !matchesRequestId && !matchesRouteId && !matchesUserId) return false;
      }
      return true;
    });
  }, [logs, levelFilter, sourceFilter, searchFilter]);

  const handleClear = () => {
    setLogs([]);
    realtime.clear();
  };

  const handleExport = () => {
    const data = JSON.stringify(filteredLogs, null, 2);
    const blob = new Blob([data], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `logs-${new Date().toISOString()}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  // @ts-ignore
const handleExportCSV = () => {
    const headers = ["timestamp", "level", "source", "message", "requestId", "routeId", "userId"];
    const rows = filteredLogs.map((log) => [
      log.timestamp,
      log.level,
      log.source,
      `"${log.message.replace(/"/g, '""')}"`,
      log.requestId || "",
      log.routeId || "",
      log.userId || "",
    ]);
    const csv = [headers.join(","), ...rows.map((r) => r.join(","))].join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `logs-${new Date().toISOString()}.csv`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  const stats = useMemo(() => ({
    total: logs.length,
    error: logs.filter((l) => l.level === "error").length,
    warn: logs.filter((l) => l.level === "warn").length,
    info: logs.filter((l) => l.level === "info").length,
  }), [logs]);

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Terminal className="h-5 w-5" />
            <CardTitle>{title}</CardTitle>
            <Badge variant={realtime.connected ? "default" : "secondary"} className="ml-2">
              {realtime.connected ? "Live" : "Disconnected"}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
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
                <TooltipContent>{isPaused ? "Resume" : "Pause"}</TooltipContent>
              </Tooltip>
            </TooltipProvider>

            {showExport && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="outline" size="icon" onClick={handleExport}>
                      <Download className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Export JSON</TooltipContent>
                </Tooltip>
              </TooltipProvider>
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
          </div>
        </div>

        <CardDescription>
          <div className="flex items-center gap-4 mt-2">
            <span>Total: <strong>{stats.total}</strong></span>
            <span className="text-destructive">Errors: <strong>{stats.error}</strong></span>
            <span className="text-amber-600">Warnings: <strong>{stats.warn}</strong></span>
            <span className="text-blue-600">Info: <strong>{stats.info}</strong></span>
          </div>
        </CardDescription>

        {showFilters && (
          <div className="flex flex-wrap items-center gap-2 mt-4">
            <div className="flex items-center gap-2">
              <Filter className="h-4 w-4 text-muted-foreground" />
              <Select value={levelFilter} onValueChange={(v) => setLevelFilter(v as LogLevel)}>
                <SelectTrigger className="w-28">
                  <SelectValue placeholder="Level" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Levels</SelectItem>
                  <SelectItem value="error">Error</SelectItem>
                  <SelectItem value="warn">Warn</SelectItem>
                  <SelectItem value="info">Info</SelectItem>
                  <SelectItem value="debug">Debug</SelectItem>
                </SelectContent>
              </Select>

              <Select value={sourceFilter} onValueChange={(v) => setSourceFilter(v as LogSource)}>
                <SelectTrigger className="w-28">
                  <SelectValue placeholder="Source" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Sources</SelectItem>
                  <SelectItem value="gateway">Gateway</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                  <SelectItem value="plugin">Plugin</SelectItem>
                  <SelectItem value="raft">Raft</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <Input
              placeholder="Search logs..."
              value={searchFilter}
              onChange={(e) => setSearchFilter(e.target.value)}
              className="w-48"
            />

            <div className="flex items-center gap-4 ml-auto">
              <div className="flex items-center gap-2">
                <Switch
                  id="auto-scroll"
                  checked={autoScroll}
                  onCheckedChange={setAutoScroll}
                />
                <label htmlFor="auto-scroll" className="text-sm text-muted-foreground">
                  Auto-scroll
                </label>
              </div>

              <div className="flex items-center gap-2">
                <Switch
                  id="show-metadata"
                  checked={showMetadata}
                  onCheckedChange={setShowMetadata}
                />
                <label htmlFor="show-metadata" className="text-sm text-muted-foreground">
                  Metadata
                </label>
              </div>
            </div>
          </div>
        )}
      </CardHeader>

      <CardContent>
        <ScrollArea ref={scrollRef} className={cn("rounded-md border", height)}>
          <div className="p-2 space-y-1 font-mono text-sm">
            {filteredLogs.length === 0 ? (
              <div className="flex items-center justify-center h-32 text-muted-foreground">
                No logs to display
              </div>
            ) : (
              filteredLogs.map((log) => (
                <div
                  key={log.id}
                  className={cn(
                    "flex items-start gap-2 p-2 rounded hover:bg-muted/50 transition-colors",
                    log.level === "error" && "bg-destructive/5",
                    log.level === "warn" && "bg-amber-500/5"
                  )}
                >
                  <Badge variant="outline" className={cn("shrink-0 text-xs", getLevelColor(log.level))}>
                    {log.level.toUpperCase()}
                  </Badge>

                  <Badge variant="outline" className="shrink-0 text-xs w-10 justify-center">
                    {getSourceIcon(log.source)}
                  </Badge>

                  <span className="shrink-0 text-xs text-muted-foreground w-36">
                    <TimeAgo value={log.timestamp} />
                  </span>

                  <div className="flex-1 min-w-0">
                    <p className={cn(
                      "break-words",
                      log.level === "error" && "text-destructive",
                      log.level === "warn" && "text-amber-700"
                    )}>
                      {log.message}
                    </p>

                    {showMetadata && log.metadata && (
                      <pre className="mt-1 text-xs text-muted-foreground bg-muted p-1 rounded overflow-x-auto">
                        {JSON.stringify(log.metadata, null, 2)}
                      </pre>
                    )}

                    {(log.requestId || log.routeId || log.userId) && (
                      <div className="flex gap-2 mt-1 text-xs text-muted-foreground">
                        {log.requestId && <span>req: {log.requestId.slice(0, 8)}...</span>}
                        {log.routeId && <span>route: {log.routeId.slice(0, 8)}...</span>}
                        {log.userId && <span>user: {log.userId.slice(0, 8)}...</span>}
                      </div>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        </ScrollArea>

        {isPaused && (
          <div className="mt-2 text-center text-sm text-amber-600">
            Log streaming paused. Click play to resume.
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function getLevelFromEventType(type: string): LogLevel {
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
  return "all";
}
