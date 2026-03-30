import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { Link } from "react-router-dom";
import { DataTable } from "@/components/shared/DataTable";
import { TimeAgo } from "@/components/shared/TimeAgo";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { JSONViewer } from "@/components/editor/JSONViewer";
import { useAuditLog, useAuditLogs } from "@/hooks/use-audit-logs";
import type { AuditEntry } from "@/lib/types";

const AUDIT_COLUMNS: ColumnDef<AuditEntry>[] = [
  {
    id: "method_path",
    header: "Request",
    cell: ({ row }) => (
      <div>
        <p className="font-medium">
          {row.original.method} {row.original.path}
        </p>
        <p className="text-xs text-muted-foreground">{row.original.client_ip}</p>
      </div>
    ),
  },
  {
    accessorKey: "status_code",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={String(row.original.status_code)} />,
  },
  {
    accessorKey: "latency_ms",
    header: "Latency",
    cell: ({ row }) => `${row.original.latency_ms} ms`,
  },
  {
    accessorKey: "created_at",
    header: "When",
    cell: ({ row }) => <TimeAgo value={row.original.created_at} />,
  },
];

export function AuditLogsPage() {
  const [method, setMethod] = useState("__all__");
  const [status, setStatus] = useState("__all__");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [open, setOpen] = useState(false);

  const logsQuery = useAuditLogs({
    method: method === "__all__" ? undefined : method,
    status_min: status === "__all__" ? undefined : Number(status),
    date_from: from || undefined,
    date_to: to || undefined,
    limit: 100,
  });
  const detailQuery = useAuditLog(selectedID);

  const entries = useMemo(() => logsQuery.data?.entries ?? [], [logsQuery.data?.entries]);

  const selected = detailQuery.data;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Audit Logs</CardTitle>
          <CardDescription>Search and inspect incoming request log records.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-4">
          <div className="space-y-1.5">
            <Label>Method</Label>
            <Select value={method} onValueChange={setMethod}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">All methods</SelectItem>
                {["GET", "POST", "PUT", "PATCH", "DELETE"].map((verb) => (
                  <SelectItem key={verb} value={verb}>
                    {verb}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label>Status ≥</Label>
            <Select value={status} onValueChange={setStatus}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">Any</SelectItem>
                <SelectItem value="200">200</SelectItem>
                <SelectItem value="400">400</SelectItem>
                <SelectItem value="500">500</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="audit-from">From</Label>
            <Input id="audit-from" type="datetime-local" value={from} onChange={(event) => setFrom(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="audit-to">To</Label>
            <Input id="audit-to" type="datetime-local" value={to} onChange={(event) => setTo(event.target.value)} />
          </div>
        </CardContent>
      </Card>

      <DataTable<AuditEntry, unknown>
        data={entries}
        columns={[
          ...AUDIT_COLUMNS,
          {
            id: "actions",
            header: "Detail",
            cell: ({ row }) => (
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setSelectedID(row.original.id);
                    setOpen(true);
                  }}
                >
                  Open
                </Button>
                <Button asChild variant="ghost" size="sm">
                  <Link to={`/audit-logs/${row.original.id}`}>Page</Link>
                </Button>
              </div>
            ),
          },
        ]}
        searchColumn="path"
        searchPlaceholder="Search path..."
        fileName="audit-logs"
        className="rounded-lg border bg-card p-3"
      />

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="w-full sm:max-w-xl">
          <SheetHeader>
            <SheetTitle>Request Detail</SheetTitle>
            <SheetDescription>Audit entry id: {selectedID || "-"}</SheetDescription>
          </SheetHeader>
          {selected ? (
            <Tabs defaultValue="request" className="mt-3">
              <TabsList>
                <TabsTrigger value="request">Request</TabsTrigger>
                <TabsTrigger value="response">Response</TabsTrigger>
              </TabsList>
              <TabsContent value="request">
                <JSONViewer
                  value={{
                    method: selected.method,
                    path: selected.path,
                    host: selected.host,
                    query: selected.query,
                    headers: selected.request_headers,
                    body: selected.request_body,
                  }}
                />
              </TabsContent>
              <TabsContent value="response">
                <JSONViewer
                  value={{
                    status_code: selected.status_code,
                    latency_ms: selected.latency_ms,
                    bytes_out: selected.bytes_out,
                    headers: selected.response_headers,
                    body: selected.response_body,
                    error_message: selected.error_message,
                  }}
                />
              </TabsContent>
            </Tabs>
          ) : (
            <p className="mt-4 text-sm text-muted-foreground">No entry selected.</p>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}

