import { useParams } from "react-router-dom";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { JSONViewer } from "@/components/editor/JSONViewer";
import { ErrorState } from "@/components/shared/ErrorState";
import { useAuditLog } from "@/hooks/use-audit-logs";

export function AuditLogDetailPage() {
  const { id = "" } = useParams();
  const detailQuery = useAuditLog(id);

  const entry = detailQuery.data;

  if (!id) {
    return <ErrorState message="Missing audit log id." />;
  }
  if (detailQuery.isError) {
    return <ErrorState message="Failed to load audit log detail." onRetry={() => detailQuery.refetch()} />;
  }
  if (!entry) {
    return <p className="text-sm text-muted-foreground">Loading audit entry...</p>;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Audit Log Detail</CardTitle>
        <CardDescription>
          {entry.method} {entry.path}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="request">
          <TabsList className="flex flex-wrap">
            <TabsTrigger value="request">Request</TabsTrigger>
            <TabsTrigger value="response">Response</TabsTrigger>
            <TabsTrigger value="timing">Timing</TabsTrigger>
            <TabsTrigger value="credits">Credits</TabsTrigger>
          </TabsList>

          <TabsContent value="request">
            <JSONViewer
              value={{
                method: entry.method,
                host: entry.host,
                path: entry.path,
                query: entry.query,
                request_headers: entry.request_headers,
                request_body: entry.request_body,
              }}
            />
          </TabsContent>

          <TabsContent value="response">
            <JSONViewer
              value={{
                status_code: entry.status_code,
                response_headers: entry.response_headers,
                response_body: entry.response_body,
                error_message: entry.error_message,
              }}
            />
          </TabsContent>

          <TabsContent value="timing">
            <JSONViewer
              value={{
                latency_ms: entry.latency_ms,
                bytes_in: entry.bytes_in,
                bytes_out: entry.bytes_out,
                created_at: entry.created_at,
              }}
            />
          </TabsContent>

          <TabsContent value="credits">
            <JSONViewer
              value={{
                note: "Credit details for single audit entry are not yet exposed by API.",
                request_id: entry.request_id,
                user_id: entry.user_id,
                consumer_name: entry.consumer_name,
              }}
            />
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}

