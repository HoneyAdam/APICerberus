import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePortalLogin, usePortalMe } from "@/hooks/use-portal";
import { PORTAL_ROUTES } from "@/lib/portal-routes";

export function PortalLoginPage() {
  const navigate = useNavigate();
  const meQuery = usePortalMe();
  const loginMutation = usePortalLogin();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  useEffect(() => {
    if (meQuery.data?.user) {
      navigate(PORTAL_ROUTES.dashboard, { replace: true });
    }
  }, [meQuery.data?.user, navigate]);

  const onSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    try {
      await loginMutation.mutateAsync({
        email: email.trim(),
        password,
      });
      navigate(PORTAL_ROUTES.dashboard, { replace: true });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Login failed");
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-[radial-gradient(circle_at_top,_hsl(var(--primary)/0.15),_transparent_50%),linear-gradient(hsl(var(--background)),hsl(var(--muted)/0.4))] p-4">
      <Card className="w-full max-w-md border-primary/20 shadow-xl">
        <CardHeader>
          <CardTitle>User Portal Login</CardTitle>
          <CardDescription>Sign in with your portal credentials.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={onSubmit}>
            <div className="space-y-1.5">
              <Label htmlFor="portal-email">Email</Label>
              <Input
                id="portal-email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="portal-password">Password</Label>
              <Input
                id="portal-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
            </div>
            <Button type="submit" className="w-full" disabled={loginMutation.isPending || meQuery.isLoading}>
              {loginMutation.isPending ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
