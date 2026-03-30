import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { usePortalAPIs } from "@/hooks/use-portal";

export function PortalAPIsPage() {
  const apisQuery = usePortalAPIs();
  const items = apisQuery.data?.items ?? [];

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">APIs</h2>
        <p className="text-sm text-muted-foreground">Browse your allowed endpoints and request costs.</p>
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {items.map((api) => (
          <Card key={api.route_id}>
            <CardHeader>
              <CardTitle className="text-base">{api.route_name}</CardTitle>
              <CardDescription>{api.service_name}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div className="flex flex-wrap gap-1.5">
                {api.methods.map((method) => (
                  <Badge key={`${api.route_id}-${method}`} variant="outline">
                    {method}
                  </Badge>
                ))}
              </div>

              <div className="space-y-1 text-xs text-muted-foreground">
                <p>Paths: {api.paths.join(", ")}</p>
                <p>Hosts: {api.hosts.length ? api.hosts.join(", ") : "*"}</p>
              </div>

              <div className="flex items-center justify-between rounded-lg border bg-muted/20 p-2 text-xs">
                <span>Credit Cost</span>
                <Badge variant="secondary">{api.credit_cost}</Badge>
              </div>

              <div className="flex items-center justify-between rounded-lg border bg-muted/20 p-2 text-xs">
                <span>Rate Limit</span>
                <span className="font-medium">Configured per account</span>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
