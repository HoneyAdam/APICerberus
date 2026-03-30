import { useMemo } from "react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { JSONViewer } from "@/components/editor/JSONViewer";
import type { PortalPlaygroundResponse } from "@/lib/portal-types";

type ResponseViewerProps = {
  response: PortalPlaygroundResponse | null;
};

function statusVariant(statusCode: number) {
  if (statusCode >= 200 && statusCode < 300) {
    return "bg-emerald-500/15 text-emerald-600 border-emerald-500/30";
  }
  if (statusCode >= 400 && statusCode < 500) {
    return "bg-amber-500/15 text-amber-600 border-amber-500/30";
  }
  if (statusCode >= 500) {
    return "bg-red-500/15 text-red-600 border-red-500/30";
  }
  return "bg-muted text-muted-foreground";
}

function parseBody(value: string) {
  if (!value.trim()) {
    return null;
  }
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return value;
  }
}

export function ResponseViewer({ response }: ResponseViewerProps) {
  const parsedBody = useMemo(() => parseBody(response?.response.body ?? ""), [response?.response.body]);

  if (!response) {
    return (
      <Card className="h-full">
        <CardHeader>
          <CardTitle>Response Viewer</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Send a request to see response payload, headers and timing.</p>
        </CardContent>
      </Card>
    );
  }

  const statusCode = response.response.status_code;
  return (
    <Card className="h-full">
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Response Viewer</CardTitle>
        <Badge variant="outline" className={statusVariant(statusCode)}>
          {statusCode}
        </Badge>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="body" className="space-y-3">
          <TabsList>
            <TabsTrigger value="body">Body</TabsTrigger>
            <TabsTrigger value="headers">Headers</TabsTrigger>
            <TabsTrigger value="timing">Timing</TabsTrigger>
          </TabsList>

          <TabsContent value="body">
            <JSONViewer value={parsedBody} minHeight={260} />
          </TabsContent>

          <TabsContent value="headers">
            <div className="rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/40 text-left text-xs uppercase text-muted-foreground">
                    <th className="p-2">Header</th>
                    <th className="p-2">Value</th>
                  </tr>
                </thead>
                <tbody>
                  {Object.entries(response.response.headers).map(([key, value]) => (
                    <tr key={key} className="border-b last:border-b-0">
                      <td className="p-2 font-mono text-xs">{key}</td>
                      <td className="p-2 break-all font-mono text-xs">{value}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </TabsContent>

          <TabsContent value="timing">
            <div className="grid gap-3 md:grid-cols-3">
              <div className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">Latency</p>
                <p className="text-lg font-semibold">{response.response.latency_ms} ms</p>
              </div>
              <div className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">Method</p>
                <p className="text-lg font-semibold">{response.request.method}</p>
              </div>
              <div className="rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">URL</p>
                <p className="truncate text-sm font-medium">{response.request.url}</p>
              </div>
            </div>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
