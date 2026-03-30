import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApiRequest } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

type BillingConfigPayload = {
  enabled: boolean;
  default_cost: number;
  zero_balance_action: string;
  method_multipliers: Record<string, number>;
};

export function SettingsPage() {
  const queryClient = useQueryClient();
  const billingConfigQuery = useQuery({
    queryKey: ["settings-billing-config"],
    queryFn: () => adminApiRequest<BillingConfigPayload>("/admin/api/v1/billing/config"),
  });

  const [portalEnabled, setPortalEnabled] = useState(true);
  const [portalBasePath, setPortalBasePath] = useState("/portal");
  const [billingEnabled, setBillingEnabled] = useState(true);
  const [defaultCost, setDefaultCost] = useState("1");
  const [retentionDays, setRetentionDays] = useState("30");
  const [retentionBatch, setRetentionBatch] = useState("500");

  useEffect(() => {
    if (!billingConfigQuery.data) {
      return;
    }
    setBillingEnabled(billingConfigQuery.data.enabled);
    setDefaultCost(String(billingConfigQuery.data.default_cost));
  }, [billingConfigQuery.data]);

  const saveBillingMutation = useMutation({
    mutationFn: async () => {
      const current = billingConfigQuery.data;
      if (!current) {
        return;
      }
      return adminApiRequest("/admin/api/v1/billing/config", {
        method: "PUT",
        body: {
          ...current,
          enabled: billingEnabled,
          default_cost: Number(defaultCost) || 1,
        },
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["settings-billing-config"] });
    },
  });

  const cleanupMutation = useMutation({
    mutationFn: () =>
      adminApiRequest("/admin/api/v1/audit-logs/cleanup", {
        method: "DELETE",
        query: {
          older_than_days: Number(retentionDays) || 30,
          batch_size: Number(retentionBatch) || 500,
        },
      }),
  });

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Portal Config</CardTitle>
          <CardDescription>User portal access and path setup.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between rounded-md border p-3">
            <div>
              <p className="font-medium">Portal Enabled</p>
              <p className="text-xs text-muted-foreground">Toggle portal availability.</p>
            </div>
            <Switch checked={portalEnabled} onCheckedChange={setPortalEnabled} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="portal-base-path">Portal Base Path</Label>
            <Input
              id="portal-base-path"
              value={portalBasePath}
              onChange={(event) => setPortalBasePath(event.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Billing Settings</CardTitle>
          <CardDescription>Default pricing controls.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between rounded-md border p-3">
            <div>
              <p className="font-medium">Billing Enabled</p>
              <p className="text-xs text-muted-foreground">Enforce credit pricing per request.</p>
            </div>
            <Switch checked={billingEnabled} onCheckedChange={setBillingEnabled} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="default-cost">Default Cost</Label>
            <Input id="default-cost" value={defaultCost} onChange={(event) => setDefaultCost(event.target.value)} />
          </div>
          <Button onClick={() => saveBillingMutation.mutate()} disabled={saveBillingMutation.isPending}>
            Save Billing
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Retention Policies</CardTitle>
          <CardDescription>Audit log cleanup policy controls.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="retention-days">Retention Days</Label>
            <Input
              id="retention-days"
              value={retentionDays}
              onChange={(event) => setRetentionDays(event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="retention-batch">Cleanup Batch Size</Label>
            <Input
              id="retention-batch"
              value={retentionBatch}
              onChange={(event) => setRetentionBatch(event.target.value)}
            />
          </div>
          <div className="md:col-span-2">
            <Button onClick={() => cleanupMutation.mutate()} disabled={cleanupMutation.isPending}>
              Run Cleanup
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

