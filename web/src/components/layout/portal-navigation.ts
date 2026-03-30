import type { LucideIcon } from "lucide-react";
import {
  Activity,
  Banknote,
  Globe,
  KeyRound,
  LayoutDashboard,
  Logs,
  Play,
  Settings,
  Shield,
} from "lucide-react";
import { PORTAL_ROUTES } from "@/lib/portal-routes";

export type PortalNavItem = {
  title: string;
  path: string;
  icon: LucideIcon;
  description: string;
};

export const PORTAL_NAV_ITEMS: PortalNavItem[] = [
  {
    title: "Dashboard",
    path: PORTAL_ROUTES.dashboard,
    icon: LayoutDashboard,
    description: "Balance and request KPIs.",
  },
  {
    title: "API Keys",
    path: PORTAL_ROUTES.apiKeys,
    icon: KeyRound,
    description: "Generate and manage personal API keys.",
  },
  {
    title: "APIs",
    path: PORTAL_ROUTES.apis,
    icon: Globe,
    description: "Accessible APIs and endpoint policies.",
  },
  {
    title: "Playground",
    path: PORTAL_ROUTES.playground,
    icon: Play,
    description: "Build and test requests with your key.",
  },
  {
    title: "Usage",
    path: PORTAL_ROUTES.usage,
    icon: Activity,
    description: "Traffic and error trend analytics.",
  },
  {
    title: "Logs",
    path: PORTAL_ROUTES.logs,
    icon: Logs,
    description: "Request history and log exports.",
  },
  {
    title: "Credits",
    path: PORTAL_ROUTES.credits,
    icon: Banknote,
    description: "Balance, transactions and forecast.",
  },
  {
    title: "Security",
    path: PORTAL_ROUTES.security,
    icon: Shield,
    description: "IP whitelist and recent activity.",
  },
  {
    title: "Settings",
    path: PORTAL_ROUTES.settings,
    icon: Settings,
    description: "Profile and notification settings.",
  },
];
