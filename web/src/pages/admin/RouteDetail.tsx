import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { Save, Settings2 } from "lucide-react";
import { toast } from "sonner";
import { PipelineView } from "@/components/flow/PipelineView";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ErrorState } from "@/components/shared/ErrorState";
import { useRoute, useUpdateRoute } from "@/hooks/use-routes";
import { useServices } from "@/hooks/use-services";
import { useUpstreams } from "@/hooks/use-upstreams";

type EditablePlugin = {
  name: string;
  enabled: boolean;
  config?: Record<string, unknown>;
};

function pluginConfigPreview(config?: Record<string, unknown>) {
  if (!config) {
    return "No config";
  }
  const keys = Object.keys(config);
  if (!keys.length) {
    return "Empty config";
  }
  const shown = keys.slice(0, 3).join(", ");
  return keys.length > 3 ? `${shown} +${keys.length - 3}` : shown;
}

export function RouteDetailPage() {
  const { id = "" } = useParams();
  const routeQuery = useRoute(id);
  const servicesQuery = useServices();
  const upstreamsQuery = useUpstreams();
  const updateRoute = useUpdateRoute();

  const [name, setName] = useState("");
  const [service, setService] = useState("");
  const [pathsText, setPathsText] = useState("");
  const [methodsText, setMethodsText] = useState("GET");
  const [plugins, setPlugins] = useState<EditablePlugin[]>([]);
  const [editingPluginName, setEditingPluginName] = useState<string | null>(null);
  const [pluginEditor, setPluginEditor] = useState("{}");

  const route = routeQuery.data;
  const services = useMemo(() => servicesQuery.data ?? [], [servicesQuery.data]);
  const upstreams = useMemo(() => upstreamsQuery.data ?? [], [upstreamsQuery.data]);
  const selectedService = useMemo(() => services.find((item) => item.id === service), [services, service]);
  const selectedUpstream = useMemo(
    () => upstreams.find((item) => item.id === selectedService?.upstream),
    [upstreams, selectedService?.upstream],
  );
  const editingPlugin = useMemo(
    () => plugins.find((plugin) => plugin.name === editingPluginName) ?? null,
    [plugins, editingPluginName],
  );

  useEffect(() => {
    if (!route) {
      return;
    }
    setName(route.name);
    setService(route.service);
    setPathsText(route.paths.join(", "));
    setMethodsText(route.methods.join(", "));
    setPlugins(
      (route.plugins ?? []).map((plugin) => ({
        name: plugin.name,
        enabled: plugin.enabled ?? true,
        config: plugin.config,
      })),
    );
  }, [route]);

  if (!id) {
    return <ErrorState message="Missing route id." />;
  }
  if (routeQuery.isError) {
    return <ErrorState message="Failed to load route details." onRetry={() => routeQuery.refetch()} />;
  }

  const openPluginEditor = (pluginName: string) => {
    const plugin = plugins.find((item) => item.name === pluginName);
    if (!plugin) {
      return;
    }
    setEditingPluginName(plugin.name);
    setPluginEditor(JSON.stringify(plugin.config ?? {}, null, 2));
  };

  const handleSavePluginConfig = () => {
    if (!editingPlugin) {
      return;
    }
    try {
      const parsed = pluginEditor.trim()
        ? (JSON.parse(pluginEditor) as Record<string, unknown>)
        : ({} as Record<string, unknown>);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        toast.error("Plugin config must be a JSON object.");
        return;
      }
      setPlugins((current) =>
        current.map((plugin) => (plugin.name === editingPlugin.name ? { ...plugin, config: parsed } : plugin)),
      );
      setEditingPluginName(null);
    } catch {
      toast.error("Invalid JSON. Please check plugin config.");
    }
  };

  const handleSave = async () => {
    await updateRoute.mutateAsync({
      id,
      payload: {
        id,
        name,
        service,
        paths: pathsText
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
        methods: methodsText
          .split(",")
          .map((item) => item.trim().toUpperCase())
          .filter(Boolean),
        plugins: plugins.map((plugin) => ({ name: plugin.name, enabled: plugin.enabled, config: plugin.config })),
      },
    });
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Route Configuration</CardTitle>
          <CardDescription>Edit route matching and service assignment.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="route-detail-name">Name</Label>
            <Input id="route-detail-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="route-detail-service">Service</Label>
            <Input
              id="route-detail-service"
              list="service-options"
              value={service}
              onChange={(event) => setService(event.target.value)}
            />
            <datalist id="service-options">
              {services.map((svc) => (
                <option key={svc.id} value={svc.id}>
                  {svc.name}
                </option>
              ))}
            </datalist>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="route-detail-paths">Paths (comma separated)</Label>
            <Input id="route-detail-paths" value={pathsText} onChange={(event) => setPathsText(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="route-detail-methods">Methods (comma separated)</Label>
            <Input
              id="route-detail-methods"
              value={methodsText}
              onChange={(event) => setMethodsText(event.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Plugin Pipeline</CardTitle>
          <CardDescription>Execution flow from gateway ingress to upstream target.</CardDescription>
        </CardHeader>
        <CardContent>
          <PipelineView
            routeName={name || route?.name || "route"}
            serviceName={(selectedService?.name ?? service) || "Unassigned service"}
            upstreamName={selectedUpstream?.name ?? selectedService?.upstream ?? "Unassigned upstream"}
            plugins={plugins}
            onEditPlugin={openPluginEditor}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Plugin Configuration</CardTitle>
          <CardDescription>Toggle route-level plugins and keep execution list visible.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {plugins.length ? (
            plugins.map((plugin) => (
              <div key={plugin.name} className="flex items-center justify-between rounded-md border p-3">
                <div className="space-y-1">
                  <p className="font-medium">{plugin.name}</p>
                  <p className="text-xs text-muted-foreground">{pluginConfigPreview(plugin.config)}</p>
                  <Badge variant="outline">route plugin</Badge>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={plugin.enabled}
                    onCheckedChange={(checked) =>
                      setPlugins((current) =>
                        current.map((item) => (item.name === plugin.name ? { ...item, enabled: checked } : item)),
                      )
                    }
                  />
                  <Button variant="outline" size="sm" onClick={() => openPluginEditor(plugin.name)}>
                    <Settings2 className="mr-2 size-3.5" />
                    Edit Config
                  </Button>
                </div>
              </div>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">No plugins configured for this route.</p>
          )}
        </CardContent>
      </Card>

      <Button onClick={handleSave} disabled={updateRoute.isPending}>
        <Save className="mr-2 size-4" />
        Save Route
      </Button>

      <Dialog
        open={Boolean(editingPlugin)}
        onOpenChange={(next) => {
          if (!next) {
            setEditingPluginName(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingPlugin?.name}</DialogTitle>
            <DialogDescription>Edit JSON config for this route plugin.</DialogDescription>
          </DialogHeader>
          <Textarea
            className="min-h-56 font-mono text-xs"
            value={pluginEditor}
            onChange={(event) => setPluginEditor(event.target.value)}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingPluginName(null)}>
              Cancel
            </Button>
            <Button onClick={handleSavePluginConfig}>Save Config</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
