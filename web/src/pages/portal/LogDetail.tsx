import { useMemo } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { JSONViewer } from "@/components/editor/JSONViewer";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { usePortalLogDetail } from "@/hooks/use-portal";

export function PortalLogDetailPage() {
  const navigate = useNavigate();
  const params = useParams<{ id: string }>();
  const logID = useMemo(() => params.id ?? "", [params.id]);
  const detailQuery = usePortalLogDetail(logID);

  if (!logID) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Log Detail</CardTitle>
        </CardHeader>
        <CardContent>Log id is missing.</CardContent>
      </Card>
    );
  }

  return (
    <Sheet open onOpenChange={() => navigate(-1)}>
      <SheetContent className="sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle>Log Detail</SheetTitle>
          <SheetDescription>Detailed request and response payload.</SheetDescription>
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
            </div>

            <JSONViewer value={detailQuery.data.request_headers ?? {}} minHeight={180} />
            <JSONViewer value={detailQuery.data.response_headers ?? {}} minHeight={180} />
            <JSONViewer value={detailQuery.data.request_body ?? ""} minHeight={160} />
            <JSONViewer value={detailQuery.data.response_body ?? ""} minHeight={160} />

            <Button variant="outline" onClick={() => navigate(-1)}>
              Close
            </Button>
          </div>
        ) : (
          <div className="p-4 text-sm text-muted-foreground">Loading...</div>
        )}
      </SheetContent>
    </Sheet>
  );
}
