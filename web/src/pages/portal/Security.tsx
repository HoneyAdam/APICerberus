import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  usePortalActivity,
  usePortalAddIPs,
  usePortalIPWhitelist,
  usePortalRemoveIP,
} from "@/hooks/use-portal";

export function PortalSecurityPage() {
  const [ipInput, setIPInput] = useState("");

  const ipQuery = usePortalIPWhitelist();
  const activityQuery = usePortalActivity();
  const addIPMutation = usePortalAddIPs();
  const removeIPMutation = usePortalRemoveIP();

  const ips = useMemo(() => ipQuery.data?.ips ?? [], [ipQuery.data?.ips]);
  const activities = useMemo(() => activityQuery.data?.items ?? [], [activityQuery.data?.items]);

  const addIPs = async () => {
    const ipsToAdd = ipInput
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean);
    if (!ipsToAdd.length) {
      return;
    }
    try {
      await addIPMutation.mutateAsync({ ips: ipsToAdd });
      setIPInput("");
      toast.success("IP whitelist updated");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to add IPs");
    }
  };

  const removeIP = async (ip: string) => {
    try {
      await removeIPMutation.mutateAsync(ip);
      toast.success("IP removed");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to remove IP");
    }
  };

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Security</h2>
        <p className="text-sm text-muted-foreground">Manage IP whitelist and review recent activity.</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>IP Whitelist</CardTitle>
          <CardDescription>Add IP addresses separated by comma.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex gap-2">
            <Input value={ipInput} onChange={(event) => setIPInput(event.target.value)} placeholder="203.0.113.5, 198.51.100.12" />
            <Button onClick={addIPs} disabled={addIPMutation.isPending}>
              Add
            </Button>
          </div>

          <div className="space-y-2">
            {ips.map((ip) => (
              <div key={ip} className="flex items-center justify-between rounded-lg border p-2">
                <span className="font-mono text-sm">{ip}</span>
                <Button variant="ghost" size="sm" onClick={() => removeIP(ip)}>
                  Remove
                </Button>
              </div>
            ))}
            {ips.length === 0 ? <p className="text-sm text-muted-foreground">No IP restrictions configured.</p> : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Activity Log</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {activities.map((event, index) => (
            <div key={`${event.timestamp}-${index}`} className="rounded-lg border p-3 text-sm">
              <p className="font-medium">{event.type}</p>
              <p className="text-xs text-muted-foreground">{event.timestamp}</p>
              <p className="text-xs text-muted-foreground">{event.message ?? `${event.method ?? ""} ${event.path ?? ""}`}</p>
            </div>
          ))}
          {activities.length === 0 ? <p className="text-sm text-muted-foreground">No activity records found.</p> : null}
        </CardContent>
      </Card>
    </div>
  );
}
