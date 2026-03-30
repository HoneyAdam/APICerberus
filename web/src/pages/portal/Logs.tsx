import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { JSONViewer } from "@/components/editor/JSONViewer";
import { DataTable } from "@/components/shared/DataTable";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { usePortalLogDetail, usePortalLogs } from "@/hooks/use-portal";
import type { PortalLogEntry } from "@/lib/portal-types";

export function PortalLogsPage() {
  const [search, setSearch] = useState("");
  const [method, setMethod] = useState("");
  const [statusMin, setStatusMin] = useState("");
  const [selectedID, setSelectedID] = useState("");

  const logsQuery = usePortalLogs({
    q: search || undefined,
    method: method || undefined,
    status_min: statusMin ? Number(statusMin) : undefined,
    limit: 500,
  });

  const detailQuery = usePortalLogDetail(selectedID);
  const entries = useMemo(() => logsQuery.data?.entries ?? [], [logsQuery.data?.entries]);
  const columns = useMemo<ColumnDef<PortalLogEntry>[]>(
    () => [
      {
        accessorKey: "created_at",
        header: "Time",
      },
      {
        accessorKey: "method",
        header: "Method",
      },
      {
        accessorKey: "path",
        header: "Path",
      },
      {
        accessorKey: "status_code",
        header: "Status",
      },
      {
        accessorKey: "latency_ms",
        header: "Latency",
      },
      {
        id: "actions",
        header: "",
        cell: ({ row }) => (
          <Button variant="ghost" size="sm" onClick={() => setSelectedID(row.original.id)}>
            View
          </Button>
        ),
      },
    ],
    [],
  );

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Logs</h2>
        <p className="text-sm text-muted-foreground">Search and inspect your request logs.</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Filters</CardTitle>
          <CardDescription>Filter by text, method or status range.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <div className="space-y-1.5">
            <Label>Search</Label>
            <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Path or request id" />
          </div>
          <div className="space-y-1.5">
            <Label>Method</Label>
            <Input value={method} onChange={(event) => setMethod(event.target.value.toUpperCase())} placeholder="GET" />
          </div>
          <div className="space-y-1.5">
            <Label>Status Min</Label>
            <Input value={statusMin} onChange={(event) => setStatusMin(event.target.value)} placeholder="400" />
          </div>
        </CardContent>
      </Card>

      <DataTable<PortalLogEntry, unknown>
        data={entries}
        columns={columns}
        searchColumn="path"
        searchPlaceholder="Filter by path..."
        initialPageSize={10}
      />

      <Sheet open={Boolean(selectedID)} onOpenChange={(open) => (!open ? setSelectedID("") : null)}>
        <SheetContent className="sm:max-w-3xl">
          <SheetHeader>
            <SheetTitle>Log Detail</SheetTitle>
            <SheetDescription>Request/response payload and metadata.</SheetDescription>
          </SheetHeader>

          {detailQuery.data ? (
            <div className="space-y-3 overflow-y-auto p-4">
              <div className="grid gap-2 rounded-lg border p-3 text-xs md:grid-cols-2">
                <p>
                  <span className="text-muted-foreground">Request ID:</span> {detailQuery.data.request_id}
                </p>
                <p>
                  <span className="text-muted-foreground">Status:</span> {detailQuery.data.status_code}
                </p>
                <p>
                  <span className="text-muted-foreground">Method:</span> {detailQuery.data.method}
                </p>
                <p>
                  <span className="text-muted-foreground">Path:</span> {detailQuery.data.path}
                </p>
                <p>
                  <span className="text-muted-foreground">Latency:</span> {detailQuery.data.latency_ms} ms
                </p>
                <p>
                  <span className="text-muted-foreground">Client IP:</span> {detailQuery.data.client_ip}
                </p>
              </div>

              <div>
                <p className="mb-1 text-sm font-medium">Request Headers</p>
                <JSONViewer value={detailQuery.data.request_headers ?? {}} minHeight={180} />
              </div>

              <div>
                <p className="mb-1 text-sm font-medium">Response Headers</p>
                <JSONViewer value={detailQuery.data.response_headers ?? {}} minHeight={180} />
              </div>

              <div>
                <p className="mb-1 text-sm font-medium">Request Body</p>
                <JSONViewer value={detailQuery.data.request_body ?? ""} minHeight={160} />
              </div>

              <div>
                <p className="mb-1 text-sm font-medium">Response Body</p>
                <JSONViewer value={detailQuery.data.response_body ?? ""} minHeight={160} />
              </div>
            </div>
          ) : (
            <div className="p-4 text-sm text-muted-foreground">Loading log detail...</div>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}
