import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";
import { adminApiRequest } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";

type PluginItem = {
  name: string;
  enabled: boolean;
  scope: "global" | "route";
  config?: Record<string, unknown>;
};

const FALLBACK_PLUGINS: PluginItem[] = [
  { name: "auth_api_key", enabled: true, scope: "global", config: { key_names: ["X-API-Key"] } },
  { name: "rate_limit", enabled: true, scope: "global", config: { algorithm: "token_bucket", limit: 100 } },
  { name: "cors", enabled: false, scope: "global", config: { origins: ["*"] } },
];

export function PluginsPage() {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<PluginItem | null>(null);
  const [editor, setEditor] = useState("{}");

  const pluginsQuery = useQuery({
    queryKey: ["plugins"],
    queryFn: async () => {
      try {
        return await adminApiRequest<PluginItem[]>("/admin/api/v1/plugins");
      } catch {
        return FALLBACK_PLUGINS;
      }
    },
  });

  const updatePluginMutation = useMutation({
    mutationFn: async (payload: PluginItem) =>
      adminApiRequest(`/admin/api/v1/plugins/${payload.name}`, {
        method: "PUT",
        body: payload,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["plugins"] });
    },
  });

  const plugins = useMemo(() => pluginsQuery.data ?? FALLBACK_PLUGINS, [pluginsQuery.data]);

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Plugins</h2>
        <p className="text-sm text-muted-foreground">Enable/disable plugin stack and edit global config payloads.</p>
      </div>

      <div className="grid gap-3">
        {plugins.map((plugin) => (
          <Card key={plugin.name}>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">{plugin.name}</CardTitle>
              <CardDescription>{plugin.scope} scope</CardDescription>
            </CardHeader>
            <CardContent className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Switch
                  checked={plugin.enabled}
                  onCheckedChange={(checked) =>
                    updatePluginMutation.mutate({
                      ...plugin,
                      enabled: checked,
                    })
                  }
                />
                <span className="text-sm text-muted-foreground">{plugin.enabled ? "Enabled" : "Disabled"}</span>
              </div>
              <Button
                variant="outline"
                onClick={() => {
                  setSelected(plugin);
                  setEditor(JSON.stringify(plugin.config ?? {}, null, 2));
                }}
              >
                <Settings2 className="mr-2 size-4" />
                Edit Config
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>

      <Dialog
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) {
            setSelected(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{selected?.name}</DialogTitle>
            <DialogDescription>JSON configuration editor.</DialogDescription>
          </DialogHeader>
          <Textarea className="min-h-56 font-mono text-xs" value={editor} onChange={(event) => setEditor(event.target.value)} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setSelected(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => {
                if (!selected) {
                  return;
                }
                updatePluginMutation.mutate({
                  ...selected,
                  config: JSON.parse(editor),
                });
                setSelected(null);
              }}
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

