import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plus, Trash2, Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "sonner";

type PluginStep = {
  id: string;
  name: string;
  enabled: boolean;
  config: Record<string, unknown>;
};

type RouteBuilderState = {
  name: string;
  path: string;
  methods: string[];
  service: string;
  stripPath: boolean;
  preserveHost: boolean;
  plugins: PluginStep[];
};

const AVAILABLE_PLUGINS = [
  { name: "cors", label: "CORS", description: "Cross-origin resource sharing" },
  { name: "rate_limit", label: "Rate Limit", description: "Request throttling" },
  { name: "auth_api_key", label: "API Key Auth", description: "API key authentication" },
  { name: "auth_jwt", label: "JWT Auth", description: "JWT token validation" },
  { name: "ip_restriction", label: "IP Restriction", description: "Allow/block IP addresses" },
  { name: "request_transform", label: "Request Transform", description: "Modify request data" },
  { name: "response_transform", label: "Response Transform", description: "Modify response data" },
  { name: "cache", label: "Cache", description: "Response caching" },
  { name: "compression", label: "Compression", description: "Gzip/Brotli compression" },
  { name: "bot_detect", label: "Bot Detection", description: "Detect and block bots" },
  { name: "circuit_breaker", label: "Circuit Breaker", description: "Fault tolerance" },
  { name: "retry", label: "Retry", description: "Automatic retries" },
  { name: "timeout", label: "Timeout", description: "Request timeouts" },
  { name: "correlation_id", label: "Correlation ID", description: "Request tracing" },
  { name: "request_validator", label: "Request Validator", description: "Schema validation" },
];

const DEFAULT_PLUGIN_CONFIGS: Record<string, Record<string, unknown>> = {
  cors: {
    allowed_origins: ["*"],
    allowed_methods: ["GET", "POST", "PUT", "DELETE"],
    allowed_headers: ["Content-Type", "Authorization"],
    max_age: 86400,
  },
  rate_limit: {
    algorithm: "token_bucket",
    requests_per_second: 100,
    burst: 150,
  },
  auth_api_key: {
    key_names: ["X-API-Key"],
  },
  auth_jwt: {
    secret: "${JWT_SECRET}",
    issuer: "apicerberus",
  },
  ip_restriction: {
    whitelist: [],
    blacklist: [],
  },
  request_transform: {
    add_headers: {},
    remove_headers: [],
  },
  response_transform: {
    add_headers: {},
    remove_headers: [],
  },
  cache: {
    ttl: 300,
    key_strategy: "url",
  },
  compression: {
    min_size: 1024,
    algorithms: ["gzip", "br"],
  },
  bot_detect: {
    action: "block",
    allow_list: [],
  },
  circuit_breaker: {
    error_threshold: 0.5,
    volume_threshold: 20,
    sleep_window: 30,
  },
  retry: {
    max_retries: 3,
    base_delay: 100,
  },
  timeout: {
    timeout: 30000,
  },
  correlation_id: {
    header_name: "X-Request-ID",
  },
  request_validator: {
    schema: {},
  },
};

export function RouteBuilderPage() {
  const navigate = useNavigate();
  const [copied, setCopied] = useState(false);
  const [route, setRoute] = useState<RouteBuilderState>({
    name: "",
    path: "/api/example",
    methods: ["GET"],
    service: "",
    stripPath: false,
    preserveHost: false,
    plugins: [],
  });

  const addPlugin = (pluginName: string) => {
    const pluginDef = AVAILABLE_PLUGINS.find((p) => p.name === pluginName);
    if (!pluginDef) return;

    const existing = route.plugins.find((p) => p.name === pluginName);
    if (existing) {
      toast.error(`${pluginDef.label} is already added`);
      return;
    }

    setRoute((prev) => ({
      ...prev,
      plugins: [
        ...prev.plugins,
        {
          id: Math.random().toString(36).substr(2, 9),
          name: pluginName,
          enabled: true,
          config: DEFAULT_PLUGIN_CONFIGS[pluginName] || {},
        },
      ],
    }));
    toast.success(`${pluginDef.label} added`);
  };

  const removePlugin = (id: string) => {
    setRoute((prev) => ({
      ...prev,
      plugins: prev.plugins.filter((p) => p.id !== id),
    }));
  };

  const updatePluginConfig = (id: string, config: Record<string, unknown>) => {
    setRoute((prev) => ({
      ...prev,
      plugins: prev.plugins.map((p) => (p.id === id ? { ...p, config } : p)),
    }));
  };

  const togglePlugin = (id: string) => {
    setRoute((prev) => ({
      ...prev,
      plugins: prev.plugins.map((p) => (p.id === id ? { ...p, enabled: !p.enabled } : p)),
    }));
  };

  const movePlugin = (id: string, direction: "up" | "down") => {
    const index = route.plugins.findIndex((p) => p.id === id);
    if (index === -1) return;
    if (direction === "up" && index === 0) return;
    if (direction === "down" && index === route.plugins.length - 1) return;

    const newPlugins = [...route.plugins];
    const swapIndex = direction === "up" ? index - 1 : index + 1;
    [newPlugins[index], newPlugins[swapIndex]] = [newPlugins[swapIndex], newPlugins[index]];

    setRoute((prev) => ({ ...prev, plugins: newPlugins }));
  };

  const generateConfig = () => {
    const config = {
      name: route.name || "new-route",
      paths: [route.path],
      methods: route.methods,
      service: route.service,
      strip_path: route.stripPath,
      preserve_host: route.preserveHost,
      plugins: route.plugins.map((p) => ({
        name: p.name,
        enabled: p.enabled,
        config: p.config,
      })),
    };
    return JSON.stringify(config, null, 2);
  };

  const copyConfig = () => {
    navigator.clipboard.writeText(generateConfig());
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
    toast.success("Configuration copied to clipboard");
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">Route Builder</h2>
        <p className="text-sm text-muted-foreground">Visually build routes with plugins using drag-and-drop.</p>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Route Configuration */}
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Route Configuration</CardTitle>
              <CardDescription>Define the basic route properties.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="name">Route Name</Label>
                <Input
                  id="name"
                  placeholder="my-api-route"
                  value={route.name}
                  onChange={(e) => setRoute({ ...route, name: e.target.value })}
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="path">Path</Label>
                <Input
                  id="path"
                  placeholder="/api/users"
                  value={route.path}
                  onChange={(e) => setRoute({ ...route, path: e.target.value })}
                />
              </div>

              <div className="grid gap-2">
                <Label>HTTP Methods</Label>
                <div className="flex flex-wrap gap-2">
                  {["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"].map((method) => (
                    <Badge
                      key={method}
                      variant={route.methods.includes(method) ? "default" : "outline"}
                      className="cursor-pointer"
                      onClick={() => {
                        setRoute((prev) => ({
                          ...prev,
                          methods: prev.methods.includes(method)
                            ? prev.methods.filter((m) => m !== method)
                            : [...prev.methods, method],
                        }));
                      }}
                    >
                      {method}
                    </Badge>
                  ))}
                </div>
              </div>

              <div className="grid gap-2">
                <Label htmlFor="service">Upstream Service</Label>
                <Input
                  id="service"
                  placeholder="user-service"
                  value={route.service}
                  onChange={(e) => setRoute({ ...route, service: e.target.value })}
                />
              </div>

              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label>Strip Path</Label>
                  <p className="text-xs text-muted-foreground">Remove matching path prefix before forwarding</p>
                </div>
                <Switch
                  checked={route.stripPath}
                  onCheckedChange={(checked) => setRoute({ ...route, stripPath: checked })}
                />
              </div>

              <div className="flex items-center justify-between">
                <div className="space-y-0.5">
                  <Label>Preserve Host</Label>
                  <p className="text-xs text-muted-foreground">Keep original Host header</p>
                </div>
                <Switch
                  checked={route.preserveHost}
                  onCheckedChange={(checked) => setRoute({ ...route, preserveHost: checked })}
                />
              </div>
            </CardContent>
          </Card>

          {/* Add Plugins */}
          <Card>
            <CardHeader>
              <CardTitle>Add Plugins</CardTitle>
              <CardDescription>Select plugins to add to this route.</CardDescription>
            </CardHeader>
            <CardContent>
              <Select onValueChange={addPlugin}>
                <SelectTrigger>
                  <SelectValue placeholder="Select a plugin..." />
                </SelectTrigger>
                <SelectContent>
                  {AVAILABLE_PLUGINS.map((plugin) => (
                    <SelectItem key={plugin.name} value={plugin.name}>
                      <div className="flex items-center gap-2">
                        <span>{plugin.label}</span>
                        <span className="text-xs text-muted-foreground">- {plugin.description}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </CardContent>
          </Card>
        </div>

        {/* Plugin Pipeline */}
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Plugin Pipeline</CardTitle>
              <CardDescription>Plugins execute in order from top to bottom.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {route.plugins.length === 0 ? (
                <div className="rounded-lg border border-dashed p-8 text-center">
                  <p className="text-sm text-muted-foreground">No plugins added yet.</p>
                  <p className="text-xs text-muted-foreground">Select a plugin from the dropdown to add it.</p>
                </div>
              ) : (
                route.plugins.map((plugin, index) => {
                  const pluginDef = AVAILABLE_PLUGINS.find((p) => p.name === plugin.name);
                  return (
                    <div
                      key={plugin.id}
                      className={`rounded-lg border p-4 transition-opacity ${
                        plugin.enabled ? "" : "opacity-50"
                      }`}
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <Badge variant="outline">{index + 1}</Badge>
                          <div>
                            <h4 className="font-medium">{pluginDef?.label || plugin.name}</h4>
                            <p className="text-xs text-muted-foreground">{pluginDef?.description}</p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <Switch
                            checked={plugin.enabled}
                            onCheckedChange={() => togglePlugin(plugin.id)}
                            size="sm"
                          />
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => movePlugin(plugin.id, "up")}
                            disabled={index === 0}
                          >
                            ↑
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => movePlugin(plugin.id, "down")}
                            disabled={index === route.plugins.length - 1}
                          >
                            ↓
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => removePlugin(plugin.id)}
                          >
                            <Trash2 className="size-4 text-destructive" />
                          </Button>
                        </div>
                      </div>

                      {plugin.enabled && (
                        <div className="mt-3">
                          <Textarea
                            value={JSON.stringify(plugin.config, null, 2)}
                            onChange={(e) => {
                              try {
                                const config = JSON.parse(e.target.value);
                                updatePluginConfig(plugin.id, config);
                              } catch {
                                // Invalid JSON, ignore
                              }
                            }}
                            className="min-h-[100px] font-mono text-xs"
                          />
                        </div>
                      )}
                    </div>
                  );
                })
              )}
            </CardContent>
          </Card>

          {/* Preview */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>Configuration Preview</CardTitle>
                <CardDescription>Copy this YAML to your config file.</CardDescription>
              </div>
              <Button variant="outline" size="sm" onClick={copyConfig}>
                {copied ? <Check className="mr-2 size-4" /> : <Copy className="mr-2 size-4" />}
                {copied ? "Copied!" : "Copy"}
              </Button>
            </CardHeader>
            <CardContent>
              <pre className="rounded-lg bg-muted p-4 text-xs overflow-auto max-h-[300px]">
                <code>{generateConfig()}</code>
              </pre>
            </CardContent>
          </Card>
        </div>
      </div>

      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={() => navigate("/routes")}>
          Cancel
        </Button>
        <Button onClick={() => toast.success("Route created successfully!")}>
          <Plus className="mr-2 size-4" />
          Create Route
        </Button>
      </div>
    </div>
  );
}
