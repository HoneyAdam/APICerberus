import { Shield } from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import { APP_NAME } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { NAV_ITEMS } from "./navigation";

function isPathActive(currentPath: string, routePath: string) {
  if (routePath === "/") {
    return currentPath === routePath;
  }
  return currentPath === routePath || currentPath.startsWith(`${routePath}/`);
}

export function AppSidebar() {
  const location = useLocation();

  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader className="border-sidebar-border/80 border-b p-3">
        <div className="flex items-center gap-2 px-1">
          <span className="inline-flex size-8 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Shield className="size-4" />
          </span>
          <div className="group-data-[collapsible=icon]:hidden">
            <p className="text-sm font-semibold leading-none">{APP_NAME}</p>
            <p className="mt-1 text-xs text-muted-foreground">Admin Panel</p>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Control Plane</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {NAV_ITEMS.map((item) => {
                const Icon = item.icon;
                const active = isPathActive(location.pathname, item.path);

                return (
                  <SidebarMenuItem key={item.path}>
                    <SidebarMenuButton asChild isActive={active} tooltip={item.title}>
                      <NavLink
                        to={item.path}
                        className={cn("transition-colors", active && "text-sidebar-primary")}
                        end={item.path === "/"}
                      >
                        <Icon className="size-4" />
                        <span>{item.title}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="border-sidebar-border/80 border-t p-3">
        <div className="group-data-[collapsible=icon]:hidden">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-xs text-muted-foreground">Gateway</span>
            <Badge variant="secondary">Healthy</Badge>
          </div>
          <p className="text-xs text-muted-foreground">Live policies, routing and analytics.</p>
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

