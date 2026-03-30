import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
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
import { Switch } from "@/components/ui/switch";
import {
  usePortalChangePassword,
  usePortalProfile,
  usePortalUpdateNotifications,
  usePortalUpdateProfile,
} from "@/hooks/use-portal";

type ProfileFormValues = {
  name: string;
  company: string;
};

type NotificationPrefs = {
  email_reports: boolean;
  security_alerts: boolean;
  billing_updates: boolean;
};

function normalizeNotifications(value: unknown): NotificationPrefs {
  const source = (value as Record<string, unknown>) ?? {};
  return {
    email_reports: Boolean(source.email_reports),
    security_alerts: Boolean(source.security_alerts),
    billing_updates: Boolean(source.billing_updates),
  };
}

export function PortalSettingsPage() {
  const profileQuery = usePortalProfile();
  const updateProfileMutation = usePortalUpdateProfile();
  const updateNotificationsMutation = usePortalUpdateNotifications();
  const changePasswordMutation = usePortalChangePassword();

  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");

  const [notifications, setNotifications] = useState<NotificationPrefs>({
    email_reports: true,
    security_alerts: true,
    billing_updates: true,
  });

  const profileForm = useForm<ProfileFormValues>({
    defaultValues: {
      name: "",
      company: "",
    },
  });

  useEffect(() => {
    if (!profileQuery.data) {
      return;
    }
    profileForm.reset({
      name: profileQuery.data.name ?? "",
      company: profileQuery.data.company ?? "",
    });
    setNotifications(normalizeNotifications(profileQuery.data.metadata?.notifications));
  }, [profileForm, profileQuery.data]);

  const profileName = useMemo(() => profileQuery.data?.name ?? "", [profileQuery.data?.name]);

  const saveProfile = profileForm.handleSubmit(async (values) => {
    try {
      await updateProfileMutation.mutateAsync({
        name: values.name,
        company: values.company,
      });
      toast.success("Profile updated");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to update profile");
    }
  });

  const saveNotifications = async () => {
    try {
      await updateNotificationsMutation.mutateAsync({
        notifications,
      });
      toast.success("Notification settings updated");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to update notifications");
    }
  };

  const changePassword = async () => {
    if (!oldPassword.trim() || !newPassword.trim()) {
      return;
    }
    try {
      await changePasswordMutation.mutateAsync({
        old_password: oldPassword,
        new_password: newPassword,
      });
      setOldPassword("");
      setNewPassword("");
      setPasswordDialogOpen(false);
      toast.success("Password changed");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to change password");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-semibold">Settings</h2>
          <p className="text-sm text-muted-foreground">Profile and account preferences for {profileName || "your account"}.</p>
        </div>

        <Dialog open={passwordDialogOpen} onOpenChange={setPasswordDialogOpen}>
          <DialogTrigger asChild>
            <Button variant="outline">Change Password</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Change Password</DialogTitle>
              <DialogDescription>Use a strong password with at least 12 characters.</DialogDescription>
            </DialogHeader>

            <div className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="old-password">Current Password</Label>
                <Input
                  id="old-password"
                  type="password"
                  value={oldPassword}
                  onChange={(event) => setOldPassword(event.target.value)}
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="new-password">New Password</Label>
                <Input
                  id="new-password"
                  type="password"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                />
              </div>
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={() => setPasswordDialogOpen(false)}>
                Cancel
              </Button>
              <Button onClick={changePassword} disabled={changePasswordMutation.isPending}>
                Update Password
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Profile</CardTitle>
          <CardDescription>Manage your visible account information.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-3" onSubmit={saveProfile}>
            <div className="space-y-1.5">
              <Label htmlFor="profile-name">Name</Label>
              <Input id="profile-name" {...profileForm.register("name", { required: true })} />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="profile-company">Company</Label>
              <Input id="profile-company" {...profileForm.register("company")} />
            </div>

            <Button type="submit" disabled={updateProfileMutation.isPending}>
              Save Profile
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Notifications</CardTitle>
          <CardDescription>Choose which updates you want to receive.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div>
              <p className="font-medium">Email Reports</p>
              <p className="text-xs text-muted-foreground">Receive usage summaries by email.</p>
            </div>
            <Switch
              checked={notifications.email_reports}
              onCheckedChange={(checked) =>
                setNotifications((current) => ({
                  ...current,
                  email_reports: checked,
                }))
              }
            />
          </div>

          <div className="flex items-center justify-between rounded-lg border p-3">
            <div>
              <p className="font-medium">Security Alerts</p>
              <p className="text-xs text-muted-foreground">Login and API key activity alerts.</p>
            </div>
            <Switch
              checked={notifications.security_alerts}
              onCheckedChange={(checked) =>
                setNotifications((current) => ({
                  ...current,
                  security_alerts: checked,
                }))
              }
            />
          </div>

          <div className="flex items-center justify-between rounded-lg border p-3">
            <div>
              <p className="font-medium">Billing Updates</p>
              <p className="text-xs text-muted-foreground">Balance and credit purchase notifications.</p>
            </div>
            <Switch
              checked={notifications.billing_updates}
              onCheckedChange={(checked) =>
                setNotifications((current) => ({
                  ...current,
                  billing_updates: checked,
                }))
              }
            />
          </div>

          <Button onClick={saveNotifications} disabled={updateNotificationsMutation.isPending}>
            Save Notifications
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
