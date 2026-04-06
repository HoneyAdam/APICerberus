import type { LucideIcon } from "lucide-react";
import {
  BellRing,
  Boxes,
  FileCode2,
  GitBranch,
  LayoutDashboard,
  LineChart,
  Network,
  Puzzle,
  Route,
  ScrollText,
  Settings2,
  Terminal,
  UserCog,
  Users2,
  WalletCards,
} from "lucide-react";
import { ROUTES } from "@/lib/constants";

export type NavItem = {
  title: string;
  path: string;
  icon: LucideIcon;
  description: string;
};

export const NAV_ITEMS: NavItem[] = [
  {
    title: "Dashboard",
    path: ROUTES.dashboard,
    icon: LayoutDashboard,
    description: "Platform overview and live traffic.",
  },
  {
    title: "Services",
    path: ROUTES.services,
    icon: Boxes,
    description: "Manage upstream-facing services.",
  },
  {
    title: "Routes",
    path: ROUTES.routes,
    icon: Route,
    description: "Control routing, methods, and plugins.",
  },
  {
    title: "Upstreams",
    path: ROUTES.upstreams,
    icon: Network,
    description: "Targets, health checks, and balancing.",
  },
  {
    title: "Consumers",
    path: ROUTES.consumers,
    icon: Users2,
    description: "Consumer identities and auth metadata.",
  },
  {
    title: "Plugins",
    path: ROUTES.plugins,
    icon: Puzzle,
    description: "Global and route-level plugin controls.",
  },
  {
    title: "Users",
    path: ROUTES.users,
    icon: UserCog,
    description: "Portal users, keys, and permissions.",
  },
  {
    title: "Credits",
    path: ROUTES.credits,
    icon: WalletCards,
    description: "Credit usage and billing controls.",
  },
  {
    title: "Audit Logs",
    path: ROUTES.auditLogs,
    icon: ScrollText,
    description: "Security and request timeline data.",
  },
  {
    title: "Analytics",
    path: ROUTES.analytics,
    icon: LineChart,
    description: "Latency, throughput, and error analytics.",
  },
  {
    title: "Alerts",
    path: ROUTES.alerts,
    icon: BellRing,
    description: "Rule-based alerting and history.",
  },
  {
    title: "Cluster",
    path: ROUTES.cluster,
    icon: GitBranch,
    description: "Cluster topology and role view.",
  },
  {
    title: "Config",
    path: ROUTES.config,
    icon: FileCode2,
    description: "Live configuration and validation tools.",
  },
  {
    title: "Settings",
    path: ROUTES.settings,
    icon: Settings2,
    description: "System and platform preferences.",
  },
  {
    title: "System Logs",
    path: "/system-logs",
    icon: Terminal,
    description: "Real-time log streaming and analysis.",
  },
];
