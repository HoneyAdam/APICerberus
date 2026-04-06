import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { adminApiRequest } from "@/lib/api";
import type { User } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Shield, UserCog, User as UserIcon, AlertTriangle, CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/utils";

export type UserRole = "admin" | "manager" | "user" | "viewer";

export type Permission = {
  id: string;
  label: string;
  description: string;
  category: string;
};

export const PERMISSIONS: Permission[] = [
  // Gateway Management
  { id: "services:read", label: "View Services", description: "View service configurations", category: "Gateway" },
  { id: "services:write", label: "Manage Services", description: "Create, update, delete services", category: "Gateway" },
  { id: "routes:read", label: "View Routes", description: "View route configurations", category: "Gateway" },
  { id: "routes:write", label: "Manage Routes", description: "Create, update, delete routes", category: "Gateway" },
  { id: "upstreams:read", label: "View Upstreams", description: "View upstream configurations", category: "Gateway" },
  { id: "upstreams:write", label: "Manage Upstreams", description: "Create, update, delete upstreams", category: "Gateway" },
  { id: "plugins:read", label: "View Plugins", description: "View plugin configurations", category: "Gateway" },
  { id: "plugins:write", label: "Manage Plugins", description: "Configure plugins", category: "Gateway" },

  // User Management
  { id: "users:read", label: "View Users", description: "View user accounts", category: "Users" },
  { id: "users:write", label: "Manage Users", description: "Create, update, delete users", category: "Users" },
  { id: "users:impersonate", label: "Impersonate Users", description: "Login as another user", category: "Users" },

  // Credit Management
  { id: "credits:read", label: "View Credits", description: "View credit balances and transactions", category: "Credits" },
  { id: "credits:write", label: "Manage Credits", description: "Distribute and adjust credits", category: "Credits" },

  // System
  { id: "config:read", label: "View Config", description: "View system configuration", category: "System" },
  { id: "config:write", label: "Manage Config", description: "Modify system configuration", category: "System" },
  { id: "audit:read", label: "View Audit Logs", description: "Access audit log entries", category: "System" },
  { id: "analytics:read", label: "View Analytics", description: "Access analytics data", category: "System" },
  { id: "cluster:read", label: "View Cluster", description: "View cluster status", category: "System" },
  { id: "cluster:write", label: "Manage Cluster", description: "Manage cluster nodes", category: "System" },
  { id: "alerts:read", label: "View Alerts", description: "View alert rules and history", category: "System" },
  { id: "alerts:write", label: "Manage Alerts", description: "Configure alert rules", category: "System" },
];

export const ROLE_PERMISSIONS: Record<UserRole, string[]> = {
  admin: PERMISSIONS.map((p) => p.id),
  manager: [
    "services:read", "services:write",
    "routes:read", "routes:write",
    "upstreams:read", "upstreams:write",
    "plugins:read", "plugins:write",
    "users:read", "users:write",
    "credits:read", "credits:write",
    "config:read",
    "audit:read",
    "analytics:read",
    "alerts:read", "alerts:write",
  ],
  user: [
    "services:read",
    "routes:read",
    "upstreams:read",
    "plugins:read",
    "credits:read",
    "analytics:read",
  ],
  viewer: [
    "services:read",
    "routes:read",
    "upstreams:read",
    "plugins:read",
    "analytics:read",
  ],
};

const ROLE_DESCRIPTIONS: Record<UserRole, { label: string; description: string; icon: typeof Shield }> = {
  admin: { label: "Administrator", description: "Full system access", icon: Shield },
  manager: { label: "Manager", description: "Can manage gateway and users", icon: UserCog },
  user: { label: "User", description: "Standard user access", icon: UserIcon },
  viewer: { label: "Viewer", description: "Read-only access", icon: UserIcon },
};

type UserRoleManagerProps = {
  user: User;
  onUpdate?: () => void;
};

export function UserRoleManager({ user, onUpdate }: UserRoleManagerProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [selectedRole, setSelectedRole] = useState<UserRole>((user.role as UserRole) || "user");
  const [customPermissions, setCustomPermissions] = useState<string[]>([]);
  const [useCustom, setUseCustom] = useState(false);
  const queryClient = useQueryClient();

  const updateRoleMutation = useMutation({
    mutationFn: async (role: UserRole) => {
      await adminApiRequest(`/admin/api/v1/users/${user.id}/role`, {
        method: "PUT",
        body: { role },
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      queryClient.invalidateQueries({ queryKey: ["user", user.id] });
      setIsOpen(false);
      onUpdate?.();
    },
  });

  const updatePermissionsMutation = useMutation({
    mutationFn: async (permissions: string[]) => {
      await adminApiRequest(`/admin/api/v1/users/${user.id}/permissions`, {
        method: "PUT",
        body: { permissions },
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setIsOpen(false);
      onUpdate?.();
    },
  });

  const handleRoleSelect = (role: UserRole) => {
    setSelectedRole(role);
    setCustomPermissions(ROLE_PERMISSIONS[role]);
    setUseCustom(false);
  };

  const togglePermission = (permissionId: string) => {
    setUseCustom(true);
    setCustomPermissions((prev) =>
      prev.includes(permissionId)
        ? prev.filter((p) => p !== permissionId)
        : [...prev, permissionId]
    );
  };

  const handleSave = () => {
    if (useCustom) {
      updatePermissionsMutation.mutate(customPermissions);
    } else {
      updateRoleMutation.mutate(selectedRole);
    }
  };

  const currentPermissions = useCustom ? customPermissions : ROLE_PERMISSIONS[selectedRole];
  const permissionsByCategory = PERMISSIONS.reduce((acc, perm) => {
    if (!acc[perm.category]) acc[perm.category] = [];
    acc[perm.category].push(perm);
    return acc;
  }, {} as Record<string, Permission[]>);

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setIsOpen(true)}>
        <Shield className="h-4 w-4 mr-1" />
        Manage Role
      </Button>

      <Dialog open={isOpen} onOpenChange={setIsOpen}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Manage User Role</DialogTitle>
            <DialogDescription>
              Configure role and permissions for {user.name} ({user.email})
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-6 py-4">
            {/* Role Selection */}
            <div className="space-y-3">
              <h4 className="text-sm font-medium">Select Role</h4>
              <div className="grid grid-cols-2 gap-3">
                {(Object.keys(ROLE_DESCRIPTIONS) as UserRole[]).map((role) => {
                  const config = ROLE_DESCRIPTIONS[role];
                  const Icon = config.icon;
                  return (
                    <button
                      key={role}
                      onClick={() => handleRoleSelect(role)}
                      className={cn(
                        "flex items-start gap-3 p-3 rounded-lg border text-left transition-colors",
                        selectedRole === role && !useCustom
                          ? "border-primary bg-primary/5"
                          : "border-muted hover:border-muted-foreground/50"
                      )}
                    >
                      <Icon className="h-5 w-5 mt-0.5 text-muted-foreground" />
                      <div>
                        <p className="font-medium">{config.label}</p>
                        <p className="text-xs text-muted-foreground">{config.description}</p>
                        <Badge variant="secondary" className="mt-1 text-xs">
                          {ROLE_PERMISSIONS[role].length} permissions
                        </Badge>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>

            <Separator />

            {/* Custom Permissions */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="text-sm font-medium">Permissions</h4>
                {useCustom && (
                  <Badge variant="outline">Custom</Badge>
                )}
              </div>

              <div className="space-y-4">
                {Object.entries(permissionsByCategory).map(([category, perms]) => (
                  <div key={category}>
                    <h5 className="text-xs font-semibold text-muted-foreground uppercase mb-2">
                      {category}
                    </h5>
                    <div className="grid gap-2">
                      {perms.map((perm) => (
                        <div
                          key={perm.id}
                          className="flex items-start gap-2 p-2 rounded hover:bg-muted/50"
                        >
                          <Checkbox
                            id={perm.id}
                            checked={currentPermissions.includes(perm.id)}
                            onCheckedChange={() => togglePermission(perm.id)}
                          />
                          <div className="flex-1">
                            <Label
                              htmlFor={perm.id}
                              className="cursor-pointer font-medium text-sm"
                            >
                              {perm.label}
                            </Label>
                            <p className="text-xs text-muted-foreground">{perm.description}</p>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setIsOpen(false)}>Cancel</Button>
            <Button
              onClick={handleSave}
              disabled={updateRoleMutation.isPending || updatePermissionsMutation.isPending}
            >
              {updateRoleMutation.isPending || updatePermissionsMutation.isPending ? (
                <>Saving...</>
              ) : (
                <>Save Changes</>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

// Bulk user actions component
export type BulkUserAction = "activate" | "suspend" | "delete" | "change-role";

type BulkUserActionsProps = {
  selectedUserIds: string[];
  onActionComplete?: () => void;
};

export function BulkUserActions({ selectedUserIds, onActionComplete }: BulkUserActionsProps) {
  const [action, setAction] = useState<BulkUserAction | null>(null);
  const [newRole, setNewRole] = useState<UserRole>("user");
  const queryClient = useQueryClient();

  const bulkActionMutation = useMutation({
    mutationFn: async ({ action, userIds, role }: { action: BulkUserAction; userIds: string[]; role?: UserRole }) => {
      await adminApiRequest("/admin/api/v1/users/bulk", {
        method: "POST",
        body: { action, user_ids: userIds, role },
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setAction(null);
      onActionComplete?.();
    },
  });

  if (selectedUserIds.length === 0) return null;

  return (
    <div className="flex items-center gap-2 p-2 bg-muted rounded-lg">
      <span className="text-sm text-muted-foreground">
        {selectedUserIds.length} selected
      </span>
      <Separator orientation="vertical" className="h-4" />

      <Button
        variant="ghost"
        size="sm"
        onClick={() => bulkActionMutation.mutate({ action: "activate", userIds: selectedUserIds })}
        disabled={bulkActionMutation.isPending}
      >
        <CheckCircle2 className="h-4 w-4 mr-1 text-success" />
        Activate
      </Button>

      <Button
        variant="ghost"
        size="sm"
        onClick={() => bulkActionMutation.mutate({ action: "suspend", userIds: selectedUserIds })}
        disabled={bulkActionMutation.isPending}
      >
        <AlertTriangle className="h-4 w-4 mr-1 text-amber-500" />
        Suspend
      </Button>

      <Button
        variant="ghost"
        size="sm"
        onClick={() => setAction("change-role")}
        disabled={bulkActionMutation.isPending}
      >
        <Shield className="h-4 w-4 mr-1" />
        Change Role
      </Button>

      {action === "change-role" && (
        <>
          <select
            value={newRole}
            onChange={(e) => setNewRole(e.target.value as UserRole)}
            className="h-8 px-2 rounded border"
          >
            <option value="admin">Admin</option>
            <option value="manager">Manager</option>
            <option value="user">User</option>
            <option value="viewer">Viewer</option>
          </select>
          <Button
            size="sm"
            onClick={() =>
              bulkActionMutation.mutate({
                action: "change-role",
                userIds: selectedUserIds,
                role: newRole,
              })
            }
            disabled={bulkActionMutation.isPending}
          >
            Apply
          </Button>
        </>
      )}
    </div>
  );
}
