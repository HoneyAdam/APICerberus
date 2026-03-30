import { useMemo, useState } from "react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useCreatePortalAPIKey,
  usePortalAPIKeys,
  useRenamePortalAPIKey,
  useRevokePortalAPIKey,
} from "@/hooks/use-portal";

export function PortalAPIKeysPage() {
  const keysQuery = usePortalAPIKeys();
  const createMutation = useCreatePortalAPIKey();
  const renameMutation = useRenamePortalAPIKey();
  const revokeMutation = useRevokePortalAPIKey();

  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [newMode, setNewMode] = useState("test");
  const [createdToken, setCreatedToken] = useState("");
  const [editMap, setEditMap] = useState<Record<string, string>>({});

  const keys = useMemo(() => keysQuery.data?.items ?? [], [keysQuery.data?.items]);

  const createKey = async () => {
    try {
      const response = await createMutation.mutateAsync({
        name: newName.trim(),
        mode: newMode,
      });
      setCreatedToken(response.token);
      setNewName("");
      setNewMode("test");
      toast.success("API key created. Copy token now; it will not be shown again.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to create API key");
    }
  };

  const saveRename = async (id: string) => {
    const name = editMap[id]?.trim();
    if (!name) {
      return;
    }
    try {
      await renameMutation.mutateAsync({ id, name });
      toast.success("API key renamed");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to rename key");
    }
  };

  const revoke = async (id: string) => {
    try {
      await revokeMutation.mutateAsync(id);
      toast.success("API key revoked");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to revoke key");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold">API Keys</h2>
          <p className="text-sm text-muted-foreground">Generate, rename and revoke keys for your integrations.</p>
        </div>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger asChild>
            <Button>Generate Key</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create API Key</DialogTitle>
              <DialogDescription>Token is shown only once. Copy it immediately after generation.</DialogDescription>
            </DialogHeader>

            <div className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="portal-key-name">Name</Label>
                <Input
                  id="portal-key-name"
                  value={newName}
                  onChange={(event) => setNewName(event.target.value)}
                  placeholder="ci-key"
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="portal-key-mode">Mode</Label>
                <Input
                  id="portal-key-mode"
                  value={newMode}
                  onChange={(event) => setNewMode(event.target.value)}
                  placeholder="test"
                />
              </div>

              {createdToken ? (
                <div className="space-y-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3">
                  <p className="text-xs uppercase tracking-wide text-amber-700">Token (one-time view)</p>
                  <Input readOnly value={createdToken} className="font-mono text-xs" />
                </div>
              ) : null}
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateOpen(false)}>
                Close
              </Button>
              <Button onClick={createKey} disabled={createMutation.isPending || !newName.trim()}>
                Create
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <div className="grid gap-3">
        {keys.map((key) => (
          <Card key={key.id}>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center justify-between text-base">
                <span>{key.name}</span>
                <Badge variant="outline">{key.status}</Badge>
              </CardTitle>
              <CardDescription className="font-mono text-xs">{key.key_prefix}...</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="grid gap-2 md:grid-cols-[1fr_auto_auto]">
                <Input
                  value={editMap[key.id] ?? key.name}
                  onChange={(event) =>
                    setEditMap((current) => ({
                      ...current,
                      [key.id]: event.target.value,
                    }))
                  }
                />
                <Button variant="outline" onClick={() => saveRename(key.id)} disabled={renameMutation.isPending}>
                  Rename
                </Button>
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button variant="destructive">Revoke</Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Revoke API key?</AlertDialogTitle>
                      <AlertDialogDescription>
                        This action invalidates the key immediately and cannot be undone.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Cancel</AlertDialogCancel>
                      <AlertDialogAction onClick={() => revoke(key.id)} variant="destructive">
                        Revoke Key
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>

              <div className="text-xs text-muted-foreground">
                <p>Last used: {key.last_used_at ?? "Never"}</p>
                <p>Last IP: {key.last_used_ip ?? "-"}</p>
              </div>
            </CardContent>
          </Card>
        ))}
        {keys.length === 0 ? (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">No API keys created yet.</CardContent>
          </Card>
        ) : null}
      </div>
    </div>
  );
}
