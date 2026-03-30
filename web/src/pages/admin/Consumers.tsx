import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import { adminApiRequest } from "@/lib/api";
import { DataTable } from "@/components/shared/DataTable";
import { Button } from "@/components/ui/button";
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

type Consumer = {
  id: string;
  name: string;
  api_keys?: Array<{ id: string; key_prefix?: string; name?: string }>;
  metadata?: Record<string, unknown>;
};

const CONSUMER_COLUMNS: ColumnDef<Consumer>[] = [
  {
    accessorKey: "name",
    header: "Consumer",
    cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
  },
  {
    id: "api_keys",
    header: "API Keys",
    cell: ({ row }) => row.original.api_keys?.length ?? 0,
  },
];

export function ConsumersPage() {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<Consumer | null>(null);
  const [open, setOpen] = useState(false);
  const [keyName, setKeyName] = useState("");

  const consumersQuery = useQuery({
    queryKey: ["consumers"],
    queryFn: async () => {
      try {
        return await adminApiRequest<Consumer[]>("/admin/api/v1/consumers");
      } catch {
        return [] as Consumer[];
      }
    },
  });

  const createKeyMutation = useMutation({
    mutationFn: async ({ consumerID, name }: { consumerID: string; name: string }) =>
      adminApiRequest(`/admin/api/v1/consumers/${consumerID}/api-keys`, {
        method: "POST",
        body: { name },
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["consumers"] });
    },
  });

  const revokeKeyMutation = useMutation({
    mutationFn: async ({ consumerID, keyID }: { consumerID: string; keyID: string }) =>
      adminApiRequest(`/admin/api/v1/consumers/${consumerID}/api-keys/${keyID}`, {
        method: "DELETE",
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["consumers"] });
    },
  });

  const consumers = useMemo(() => consumersQuery.data ?? [], [consumersQuery.data]);

  const handleCreateKey = async () => {
    if (!selected || !keyName.trim()) {
      return;
    }
    await createKeyMutation.mutateAsync({ consumerID: selected.id, name: keyName.trim() });
    setKeyName("");
  };

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Consumers</h2>
        <p className="text-sm text-muted-foreground">Consumer directory and API key management panel.</p>
      </div>

      <DataTable<Consumer, unknown>
        data={consumers}
        columns={[
          ...CONSUMER_COLUMNS,
          {
            id: "actions",
            header: "Manage",
            cell: ({ row }) => (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setSelected(row.original);
                  setOpen(true);
                }}
              >
                API Keys
              </Button>
            ),
          },
        ]}
        searchColumn="name"
        searchPlaceholder="Search consumer..."
        fileName="consumers"
      />

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>API Key Management</DialogTitle>
            <DialogDescription>{selected?.name ?? "No consumer selected"}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="consumer-key-name">New Key Name</Label>
              <Input
                id="consumer-key-name"
                value={keyName}
                onChange={(event) => setKeyName(event.target.value)}
                placeholder="production-key"
              />
            </div>
            <Button onClick={handleCreateKey} disabled={createKeyMutation.isPending}>
              <Plus className="mr-2 size-4" />
              Create API Key
            </Button>

            <div className="space-y-2">
              {(selected?.api_keys ?? []).map((apiKey) => (
                <div key={apiKey.id} className="flex items-center justify-between rounded-md border p-2">
                  <span className="text-sm">{apiKey.name ?? apiKey.key_prefix ?? apiKey.id}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      selected
                        ? revokeKeyMutation.mutate({ consumerID: selected.id, keyID: apiKey.id })
                        : undefined
                    }
                  >
                    Revoke
                  </Button>
                </div>
              ))}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
