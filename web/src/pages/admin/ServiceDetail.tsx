import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { BadgeInfo, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ErrorState } from "@/components/shared/ErrorState";
import { useRoutes } from "@/hooks/use-routes";
import { useService, useUpdateService } from "@/hooks/use-services";

export function ServiceDetailPage() {
  const { id = "" } = useParams();
  const serviceQuery = useService(id);
  const routesQuery = useRoutes();
  const updateService = useUpdateService();

  const service = serviceQuery.data;
  const [name, setName] = useState("");
  const [protocol, setProtocol] = useState("http");
  const [upstream, setUpstream] = useState("");

  useEffect(() => {
    if (!service) {
      return;
    }
    setName(service.name);
    setProtocol(service.protocol);
    setUpstream(service.upstream);
  }, [service]);

  if (!id) {
    return <ErrorState message="Missing service id." />;
  }

  if (serviceQuery.isError) {
    return <ErrorState message="Failed to load service detail." onRetry={() => serviceQuery.refetch()} />;
  }

  const associatedRoutes = (routesQuery.data ?? []).filter(
    (route) => route.service === service?.id || route.service === service?.name,
  );

  const handleSave = async () => {
    await updateService.mutateAsync({
      id,
      payload: {
        id,
        name,
        protocol,
        upstream,
      },
    });
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <BadgeInfo className="size-5 text-primary" />
            Service Configuration
          </CardTitle>
          <CardDescription>Edit core service settings and upstream binding.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <div className="space-y-1.5">
            <Label htmlFor="service-detail-name">Name</Label>
            <Input id="service-detail-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="service-detail-protocol">Protocol</Label>
            <Input
              id="service-detail-protocol"
              value={protocol}
              onChange={(event) => setProtocol(event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="service-detail-upstream">Upstream</Label>
            <Input
              id="service-detail-upstream"
              value={upstream}
              onChange={(event) => setUpstream(event.target.value)}
            />
          </div>
          <div className="md:col-span-3">
            <Button onClick={handleSave} disabled={updateService.isPending}>
              <Save className="mr-2 size-4" />
              Save Service
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Associated Routes</CardTitle>
          <CardDescription>Routes currently mapped to this service.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {associatedRoutes.length ? (
            associatedRoutes.map((route) => (
              <div key={route.id} className="flex items-center justify-between rounded-md border p-3">
                <div>
                  <p className="font-medium">{route.name}</p>
                  <p className="text-xs text-muted-foreground">{route.paths.join(", ")}</p>
                </div>
                <Badge variant="outline">{route.methods.join(", ")}</Badge>
              </div>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">No route attached to this service yet.</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
