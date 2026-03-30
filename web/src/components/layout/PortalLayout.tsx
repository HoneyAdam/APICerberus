import { MoonStar, SunMedium, LogOut } from "lucide-react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { useTheme } from "@/components/layout/ThemeProvider";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { PORTAL_NAV_ITEMS } from "./portal-navigation";
import { usePortalLogout, usePortalMe } from "@/hooks/use-portal";

export function PortalLayout() {
  const navigate = useNavigate();
  const { resolvedMode, toggleMode } = useTheme();
  const meQuery = usePortalMe();
  const logoutMutation = usePortalLogout();

  const handleLogout = async () => {
    try {
      await logoutMutation.mutateAsync();
      navigate("/portal/login", { replace: true });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to logout");
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-b from-background via-background to-muted/20">
      <div className="mx-auto flex w-full max-w-[1440px] flex-col gap-4 p-3 md:flex-row md:gap-6 md:p-6">
        <aside className="w-full rounded-2xl border bg-card/70 p-3 backdrop-blur-sm md:sticky md:top-6 md:h-[calc(100vh-3rem)] md:w-72 md:p-4">
          <div className="mb-4 border-b pb-4">
            <p className="text-xs uppercase tracking-wide text-muted-foreground">API Cerberus</p>
            <h1 className="text-lg font-semibold">User Portal</h1>
            <p className="mt-1 text-xs text-muted-foreground">{meQuery.data?.user?.email ?? "Session loading..."}</p>
          </div>

          <nav className="grid gap-1">
            {PORTAL_NAV_ITEMS.map((item) => {
              const Icon = item.icon;
              return (
                <NavLink
                  key={item.path}
                  to={item.path}
                  className={({ isActive }) =>
                    cn(
                      "flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition",
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground",
                    )
                  }
                >
                  <Icon className="size-4" />
                  <span>{item.title}</span>
                </NavLink>
              );
            })}
          </nav>

          <div className="mt-4 flex items-center gap-2 border-t pt-4">
            <Button variant="outline" size="icon" onClick={toggleMode} aria-label="Toggle theme">
              {resolvedMode === "dark" ? <SunMedium className="size-4" /> : <MoonStar className="size-4" />}
            </Button>
            <Button variant="outline" className="w-full justify-start" onClick={handleLogout}>
              <LogOut className="mr-2 size-4" />
              Sign out
            </Button>
          </div>
        </aside>

        <main className="min-w-0 flex-1 rounded-2xl border bg-card/40 p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
