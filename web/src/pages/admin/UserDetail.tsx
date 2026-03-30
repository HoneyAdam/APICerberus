import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { Save } from "lucide-react";
import { adminApiRequest } from "@/lib/api";
import { ErrorState } from "@/components/shared/ErrorState";
import { CreditBadge } from "@/components/shared/CreditBadge";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { TimeAgo } from "@/components/shared/TimeAgo";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAuditLogs } from "@/hooks/use-audit-logs";
import { useUpdateUser, useUser } from "@/hooks/use-users";
import { useUserCreditBalance, useUserCreditTransactions } from "@/hooks/use-credits";

export function UserDetailPage() {
  const { id = "" } = useParams();
  const userQuery = useUser(id);
  const updateUser = useUpdateUser();
  const balanceQuery = useUserCreditBalance(id);
  const transactionsQuery = useUserCreditTransactions(id, { limit: 25 });
  const logsQuery = useAuditLogs({ user_id: id, limit: 20 });

  const apiKeysQuery = useQuery({
    queryKey: ["user-api-keys", id],
    queryFn: () => adminApiRequest<Array<Record<string, unknown>>>(`/admin/api/v1/users/${id}/api-keys`),
    enabled: Boolean(id),
  });
  const permissionsQuery = useQuery({
    queryKey: ["user-permissions", id],
    queryFn: () => adminApiRequest<Array<Record<string, unknown>>>(`/admin/api/v1/users/${id}/permissions`),
    enabled: Boolean(id),
  });

  const user = userQuery.data;
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [status, setStatus] = useState("");

  useEffect(() => {
    if (!user) {
      return;
    }
    setName(user.name);
    setEmail(user.email);
    setStatus(user.status);
  }, [user]);

  if (!id) {
    return <ErrorState message="Missing user id." />;
  }
  if (userQuery.isError) {
    return <ErrorState message="Failed to load user details." onRetry={() => userQuery.refetch()} />;
  }

  const handleSaveProfile = async () => {
    await updateUser.mutateAsync({
      id,
      payload: {
        name,
        email,
        status,
      },
    });
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold">{user?.name ?? "User Detail"}</h2>
          <p className="text-sm text-muted-foreground">{user?.email}</p>
        </div>
        <StatusBadge status={status || user?.status || "pending"} />
      </div>

      <Tabs defaultValue="profile">
        <TabsList className="flex flex-wrap">
          <TabsTrigger value="profile">Profile</TabsTrigger>
          <TabsTrigger value="api-keys">API Keys</TabsTrigger>
          <TabsTrigger value="permissions">Permissions</TabsTrigger>
          <TabsTrigger value="credits">Credits</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="profile" className="space-y-3">
          <Card>
            <CardHeader>
              <CardTitle>Profile</CardTitle>
              <CardDescription>Basic account identity and state.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3 md:grid-cols-3">
              <div className="space-y-1.5">
                <Label htmlFor="detail-user-name">Name</Label>
                <Input id="detail-user-name" value={name} onChange={(event) => setName(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="detail-user-email">Email</Label>
                <Input id="detail-user-email" value={email} onChange={(event) => setEmail(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="detail-user-status">Status</Label>
                <Input id="detail-user-status" value={status} onChange={(event) => setStatus(event.target.value)} />
              </div>
              <div className="md:col-span-3">
                <Button onClick={handleSaveProfile} disabled={updateUser.isPending}>
                  <Save className="mr-2 size-4" />
                  Save Profile
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="api-keys" className="space-y-3">
          <Card>
            <CardHeader>
              <CardTitle>API Keys</CardTitle>
              <CardDescription>Active and revoked keys for this user.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {(apiKeysQuery.data ?? []).map((key) => (
                <div key={String(key.id)} className="rounded-md border p-3">
                  <p className="font-medium">{String(key.name ?? key.key_prefix ?? key.id)}</p>
                  <p className="text-xs text-muted-foreground">{String(key.mode ?? "default")} mode</p>
                </div>
              ))}
              {!apiKeysQuery.data?.length ? <p className="text-sm text-muted-foreground">No API keys found.</p> : null}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="permissions" className="space-y-3">
          <Card>
            <CardHeader>
              <CardTitle>Permissions</CardTitle>
              <CardDescription>Route-level grants assigned to this user.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {(permissionsQuery.data ?? []).map((permission) => (
                <div key={String(permission.id)} className="rounded-md border p-3">
                  <p className="font-medium">{String(permission.route_id ?? "unknown route")}</p>
                  <p className="text-xs text-muted-foreground">{String(permission.methods ?? "[]")}</p>
                </div>
              ))}
              {!permissionsQuery.data?.length ? (
                <p className="text-sm text-muted-foreground">No permissions assigned.</p>
              ) : null}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="credits" className="space-y-3">
          <Card>
            <CardHeader>
              <CardTitle>Credits</CardTitle>
              <CardDescription>Balance and recent transactions.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <CreditBadge kind="balance" value={balanceQuery.data?.balance ?? 0} />
              {(transactionsQuery.data?.transactions ?? []).slice(0, 10).map((transaction) => (
                <div key={transaction.id} className="rounded-md border p-3">
                  <div className="flex items-center justify-between">
                    <p className="font-medium">{transaction.type}</p>
                    <p className="text-sm tabular-nums">{transaction.amount}</p>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    <TimeAgo value={transaction.created_at} />
                  </p>
                </div>
              ))}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logs" className="space-y-3">
          <Card>
            <CardHeader>
              <CardTitle>Logs</CardTitle>
              <CardDescription>Recent request log activity.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {(logsQuery.data?.entries ?? []).map((entry) => (
                <div key={entry.id} className="rounded-md border p-3">
                  <div className="flex items-center justify-between">
                    <p className="font-medium">
                      {entry.method} {entry.path}
                    </p>
                    <p className="text-sm">{entry.status_code}</p>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    <TimeAgo value={entry.created_at} />
                  </p>
                </div>
              ))}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

