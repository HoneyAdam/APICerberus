import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import type { User } from "@/lib/types";
import { DataTable } from "@/components/shared/DataTable";
import { CreditBadge } from "@/components/shared/CreditBadge";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { Button } from "@/components/ui/button";
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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useCreateUser, useUsers } from "@/hooks/use-users";
import { UserRoleManager, BulkUserActions } from "@/components/users/UserRoleManager";

const USER_COLUMNS: ColumnDef<User>[] = [
  {
    accessorKey: "name",
    header: "Name",
    cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
  },
  {
    accessorKey: "email",
    header: "Email",
  },
  {
    id: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    id: "credits",
    header: "Credits",
    cell: ({ row }) => <CreditBadge kind="balance" value={row.original.credit_balance ?? 0} />,
  },
];

export function UsersPage() {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState("all");
  const [search, setSearch] = useState("");
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>([]);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [initialCredits, setInitialCredits] = useState("0");

  const usersQuery = useUsers({
    status: tab === "all" ? undefined : tab,
    search: search || undefined,
    limit: 200,
    sort_by: "created_at",
    sort_desc: true,
  });
  const createUser = useCreateUser();

  const users = useMemo(() => usersQuery.data?.users ?? [], [usersQuery.data?.users]);

  const handleCreate = async () => {
    if (!email.trim() || !name.trim()) {
      return;
    }
    const user = await createUser.mutateAsync({
      email: email.trim(),
      name: name.trim(),
      password: password.trim(),
      initial_credits: Number(initialCredits) || 0,
    });
    setOpen(false);
    setEmail("");
    setName("");
    setPassword("");
    setInitialCredits("0");
    navigate(`/users/${user.id}`);
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold">Users</h2>
          <p className="text-sm text-muted-foreground">Create and manage portal users, status and balances.</p>
        </div>

        <div className="flex items-center gap-2">
          <Input
            placeholder="Search users..."
            className="w-60"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button>
                <Plus className="mr-2 size-4" />
                New User
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create User</DialogTitle>
                <DialogDescription>Provision a new portal account and optional starting credits.</DialogDescription>
              </DialogHeader>
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <Label htmlFor="user-email">Email</Label>
                  <Input id="user-email" value={email} onChange={(event) => setEmail(event.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="user-name">Name</Label>
                  <Input id="user-name" value={name} onChange={(event) => setName(event.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="user-password">Password</Label>
                  <Input
                    id="user-password"
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="user-initial-credits">Initial Credits</Label>
                  <Input
                    id="user-initial-credits"
                    value={initialCredits}
                    onChange={(event) => setInitialCredits(event.target.value)}
                  />
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setOpen(false)}>
                  Cancel
                </Button>
                <Button onClick={handleCreate} disabled={createUser.isPending}>
                  Create
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
          <TabsTrigger value="suspended">Suspended</TabsTrigger>
        </TabsList>
      </Tabs>

      <BulkUserActions
        selectedUserIds={selectedUserIds}
        onActionComplete={() => setSelectedUserIds([])}
      />

      <DataTable<User, unknown>
        data={users}
        columns={[
          {
            id: "select",
            header: ({ table }) => (
              <input
                type="checkbox"
                checked={table.getIsAllPageRowsSelected()}
                onChange={(e) => {
                  table.toggleAllPageRowsSelected(e.target.checked);
                  if (e.target.checked) {
                    setSelectedUserIds(users.map((u) => u.id));
                  } else {
                    setSelectedUserIds([]);
                  }
                }}
              />
            ),
            cell: ({ row }) => (
              <input
                type="checkbox"
                checked={selectedUserIds.includes(row.original.id)}
                onChange={(e) => {
                  if (e.target.checked) {
                    setSelectedUserIds((prev) => [...prev, row.original.id]);
                  } else {
                    setSelectedUserIds((prev) => prev.filter((id) => id !== row.original.id));
                  }
                }}
              />
            ),
          },
          ...USER_COLUMNS,
          {
            id: "role",
            header: "Role",
            cell: ({ row }) => (
              <span className="text-xs bg-muted px-2 py-1 rounded">{row.original.role}</span>
            ),
          },
          {
            id: "actions",
            header: "Actions",
            cell: ({ row }) => (
              <div className="flex items-center gap-2">
                <UserRoleManager user={row.original} />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => navigate(`/users/${row.original.id}`)}
                >
                  View
                </Button>
              </div>
            ),
          },
        ]}
        searchColumn="name"
        searchPlaceholder="Filter by name..."
        fileName="users"
        className="rounded-lg border bg-card p-3"
      />
    </div>
  );
}

