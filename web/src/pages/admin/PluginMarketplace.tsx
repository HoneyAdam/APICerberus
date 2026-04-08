import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Search,
  Filter,
  Grid3X3,
  List,
  Star,
  Download,
  Check,
  X,
  Package,
  Shield,
  Zap,
  Lock,
  BarChart3,
  Globe,
  Clock,
  ExternalLink,
  Sparkles,
} from "lucide-react";
import { adminApiRequest } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle, CardFooter } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type PluginCategory = "all" | "auth" | "security" | "traffic" | "analytics" | "transformation" | "utility";
type PluginStatus = "all" | "installed" | "available" | "update";
type SortOption = "popular" | "newest" | "rating" | "name";

interface Plugin {
  id: string;
  name: string;
  displayName: string;
  description: string;
  version: string;
  latestVersion: string;
  author: string;
  authorUrl?: string;
  category: PluginCategory;
  rating: number;
  reviewCount: number;
  downloadCount: number;
  installed: boolean;
  hasUpdate: boolean;
  tags: string[];
  icon?: string;
  readme?: string;
  configSchema?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
  license: string;
  homepage?: string;
  repository?: string;
}

const CATEGORY_ICONS: Record<PluginCategory, typeof Package> = {
  all: Package,
  auth: Lock,
  security: Shield,
  traffic: Zap,
  analytics: BarChart3,
  transformation: Globe,
  utility: Clock,
};

const CATEGORY_LABELS: Record<PluginCategory, string> = {
  all: "All Categories",
  auth: "Authentication",
  security: "Security",
  traffic: "Traffic Control",
  analytics: "Analytics",
  transformation: "Transformation",
  utility: "Utilities",
};

const FALLBACK_PLUGINS: Plugin[] = [
  {
    id: "auth-api-key",
    name: "auth_api_key",
    displayName: "API Key Auth",
    description: "Authenticate requests using API keys in headers or query parameters.",
    version: "1.2.0",
    latestVersion: "1.2.0",
    author: "APICerebrus",
    category: "auth",
    rating: 4.8,
    reviewCount: 124,
    downloadCount: 15000,
    installed: true,
    hasUpdate: false,
    tags: ["auth", "api-key", "security"],
    license: "MIT",
    createdAt: "2024-01-15",
    updatedAt: "2024-03-20",
  },
  {
    id: "rate-limit",
    name: "rate_limit",
    displayName: "Rate Limiter",
    description: "Advanced rate limiting with token bucket, fixed window, and sliding window algorithms.",
    version: "2.1.0",
    latestVersion: "2.1.0",
    author: "APICerebrus",
    category: "traffic",
    rating: 4.9,
    reviewCount: 89,
    downloadCount: 12000,
    installed: true,
    hasUpdate: false,
    tags: ["rate-limit", "throttling", "traffic"],
    license: "MIT",
    createdAt: "2024-01-10",
    updatedAt: "2024-04-01",
  },
  {
    id: "cors",
    name: "cors",
    displayName: "CORS Handler",
    description: "Cross-Origin Resource Sharing support with configurable origins and headers.",
    version: "1.0.5",
    latestVersion: "1.1.0",
    author: "APICerebrus",
    category: "security",
    rating: 4.6,
    reviewCount: 56,
    downloadCount: 8500,
    installed: true,
    hasUpdate: true,
    tags: ["cors", "security", "headers"],
    license: "MIT",
    createdAt: "2024-02-01",
    updatedAt: "2024-03-15",
  },
  {
    id: "jwt-auth",
    name: "jwt_auth",
    displayName: "JWT Authentication",
    description: "JSON Web Token authentication with RS256/HS256 support and claims validation.",
    version: "1.5.0",
    latestVersion: "1.5.0",
    author: "Community",
    category: "auth",
    rating: 4.7,
    reviewCount: 203,
    downloadCount: 22000,
    installed: false,
    hasUpdate: false,
    tags: ["jwt", "auth", "oauth", "security"],
    license: "Apache-2.0",
    createdAt: "2024-01-20",
    updatedAt: "2024-04-05",
  },
  {
    id: "request-transformer",
    name: "request_transformer",
    displayName: "Request Transformer",
    description: "Transform requests by adding, removing, or modifying headers and body content.",
    version: "1.3.0",
    latestVersion: "1.3.0",
    author: "APICerebrus",
    category: "transformation",
    rating: 4.5,
    reviewCount: 78,
    downloadCount: 9800,
    installed: false,
    hasUpdate: false,
    tags: ["transform", "headers", "middleware"],
    license: "MIT",
    createdAt: "2024-02-10",
    updatedAt: "2024-03-25",
  },
  {
    id: "prometheus-metrics",
    name: "prometheus_metrics",
    displayName: "Prometheus Metrics",
    description: "Export metrics in Prometheus format for monitoring and alerting.",
    version: "2.0.0",
    latestVersion: "2.0.0",
    author: "Observability Team",
    category: "analytics",
    rating: 4.9,
    reviewCount: 145,
    downloadCount: 18000,
    installed: false,
    hasUpdate: false,
    tags: ["metrics", "prometheus", "monitoring"],
    license: "MIT",
    createdAt: "2024-01-05",
    updatedAt: "2024-04-10",
  },
  {
    id: "oauth2",
    name: "oauth2",
    displayName: "OAuth 2.0",
    description: "OAuth 2.0 authentication with support for multiple providers.",
    version: "1.1.0",
    latestVersion: "1.2.0",
    author: "Security Team",
    category: "auth",
    rating: 4.4,
    reviewCount: 67,
    downloadCount: 7200,
    installed: false,
    hasUpdate: false,
    tags: ["oauth", "auth", "security", "sso"],
    license: "MIT",
    createdAt: "2024-02-15",
    updatedAt: "2024-04-08",
  },
  {
    id: "ip-restriction",
    name: "ip_restriction",
    displayName: "IP Restriction",
    description: "Allow or deny requests based on IP address or CIDR ranges.",
    version: "1.0.0",
    latestVersion: "1.0.0",
    author: "APICerebrus",
    category: "security",
    rating: 4.3,
    reviewCount: 34,
    downloadCount: 5600,
    installed: false,
    hasUpdate: false,
    tags: ["ip", "security", "access-control"],
    license: "MIT",
    createdAt: "2024-03-01",
    updatedAt: "2024-03-01",
  },
  {
    id: "cache",
    name: "cache",
    displayName: "Response Cache",
    description: "Cache responses to improve performance and reduce backend load.",
    version: "1.4.0",
    latestVersion: "1.4.0",
    author: "Performance Team",
    category: "utility",
    rating: 4.7,
    reviewCount: 112,
    downloadCount: 13500,
    installed: false,
    hasUpdate: false,
    tags: ["cache", "performance", "redis"],
    license: "MIT",
    createdAt: "2024-01-25",
    updatedAt: "2024-04-02",
  },
  {
    id: "bot-detection",
    name: "bot_detection",
    displayName: "Bot Detection",
    description: "Detect and block automated traffic and bad bots.",
    version: "0.9.0",
    latestVersion: "1.0.0",
    author: "Security Team",
    category: "security",
    rating: 4.2,
    reviewCount: 45,
    downloadCount: 4800,
    installed: false,
    hasUpdate: false,
    tags: ["bot", "security", "waf"],
    license: "MIT",
    createdAt: "2024-03-10",
    updatedAt: "2024-04-05",
  },
  {
    id: "graphql-rate-limit",
    name: "graphql_rate_limit",
    displayName: "GraphQL Rate Limit",
    description: "Rate limiting specifically designed for GraphQL queries by complexity.",
    version: "1.0.0",
    latestVersion: "1.0.0",
    author: "GraphQL Team",
    category: "traffic",
    rating: 4.6,
    reviewCount: 28,
    downloadCount: 3200,
    installed: false,
    hasUpdate: false,
    tags: ["graphql", "rate-limit", "complexity"],
    license: "MIT",
    createdAt: "2024-03-15",
    updatedAt: "2024-03-15",
  },
  {
    id: "request-validator",
    name: "request_validator",
    displayName: "Request Validator",
    description: "Validate requests against JSON Schema or OpenAPI specifications.",
    version: "1.2.0",
    latestVersion: "1.2.0",
    author: "APICerebrus",
    category: "utility",
    rating: 4.5,
    reviewCount: 52,
    downloadCount: 6400,
    installed: false,
    hasUpdate: false,
    tags: ["validation", "schema", "openapi"],
    license: "MIT",
    createdAt: "2024-02-20",
    updatedAt: "2024-03-30",
  },
];

function StarRating({ rating }: { rating: number }) {
  return (
    <div className="flex items-center gap-0.5">
      {[1, 2, 3, 4, 5].map((star) => (
        <Star
          key={star}
          className={cn(
            "h-3.5 w-3.5",
            star <= Math.round(rating)
              ? "fill-amber-400 text-amber-400"
              : "fill-muted text-muted"
          )}
        />
      ))}
    </div>
  );
}

function PluginCard({
  plugin,
  viewMode,
  onInstall,
  onUninstall,
  onUpdate,
  onViewDetails,
  isPending,
}: {
  plugin: Plugin;
  viewMode: "grid" | "list";
  onInstall: (plugin: Plugin) => void;
  onUninstall: (plugin: Plugin) => void;
  onUpdate: (plugin: Plugin) => void;
  onViewDetails: (plugin: Plugin) => void;
  isPending: boolean;
}) {
  const CategoryIcon = CATEGORY_ICONS[plugin.category];

  if (viewMode === "list") {
    return (
      <Card className="hover:border-primary/50 transition-colors">
        <CardContent className="p-4">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
              <CategoryIcon className="h-6 w-6 text-primary" />
            </div>

            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="font-semibold truncate">{plugin.displayName}</h3>
                {plugin.installed && (
                  <Badge variant="default" className="text-xs">
                    <Check className="h-3 w-3 mr-1" />
                    Installed
                  </Badge>
                )}
                {plugin.hasUpdate && (
                  <Badge variant="secondary" className="text-xs">
                    <Sparkles className="h-3 w-3 mr-1" />
                    Update
                  </Badge>
                )}
              </div>
              <p className="text-sm text-muted-foreground line-clamp-1">
                {plugin.description}
              </p>
              <div className="flex items-center gap-3 mt-1 text-xs text-muted-foreground">
                <span className="flex items-center gap-1">
                  <StarRating rating={plugin.rating} />
                  <span>({plugin.reviewCount})</span>
                </span>
                <span className="flex items-center gap-1">
                  <Download className="h-3 w-3" />
                  {plugin.downloadCount.toLocaleString()}
                </span>
                <span>v{plugin.version}</span>
                <Badge variant="outline" className="text-xs">
                  {CATEGORY_LABELS[plugin.category]}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-2 shrink-0">
              <Button variant="ghost" size="sm" onClick={() => onViewDetails(plugin)}>
                Details
              </Button>
              {plugin.installed ? (
                plugin.hasUpdate ? (
                  <Button
                    size="sm"
                    onClick={() => onUpdate(plugin)}
                    disabled={isPending}
                  >
                    Update
                  </Button>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onUninstall(plugin)}
                    disabled={isPending}
                  >
                    <X className="h-4 w-4 mr-1" />
                    Uninstall
                  </Button>
                )
              ) : (
                <Button
                  size="sm"
                  onClick={() => onInstall(plugin)}
                  disabled={isPending}
                >
                  <Download className="h-4 w-4 mr-1" />
                  Install
                </Button>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="hover:border-primary/50 transition-colors flex flex-col">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
            <CategoryIcon className="h-5 w-5 text-primary" />
          </div>
          <div className="flex gap-1">
            {plugin.installed && (
              <Badge variant="default" className="text-xs">
                <Check className="h-3 w-3" />
              </Badge>
            )}
            {plugin.hasUpdate && (
              <Badge variant="secondary" className="text-xs">
                <Sparkles className="h-3 w-3" />
              </Badge>
            )}
          </div>
        </div>
        <CardTitle className="text-base mt-2">{plugin.displayName}</CardTitle>
        <CardDescription className="line-clamp-2">{plugin.description}</CardDescription>
      </CardHeader>
      <CardContent className="flex-1">
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <StarRating rating={plugin.rating} />
            <span>({plugin.reviewCount})</span>
          </span>
          <span className="flex items-center gap-1">
            <Download className="h-3 w-3" />
            {plugin.downloadCount.toLocaleString()}
          </span>
        </div>
        <div className="flex flex-wrap gap-1 mt-2">
          <Badge variant="outline" className="text-xs">
            {CATEGORY_LABELS[plugin.category]}
          </Badge>
          <Badge variant="outline" className="text-xs">
            v{plugin.version}
          </Badge>
        </div>
      </CardContent>
      <CardFooter className="pt-0 gap-2">
        <Button variant="ghost" size="sm" className="flex-1" onClick={() => onViewDetails(plugin)}>
          Details
        </Button>
        {plugin.installed ? (
          plugin.hasUpdate ? (
            <Button size="sm" className="flex-1" onClick={() => onUpdate(plugin)} disabled={isPending}>
              Update
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="flex-1"
              onClick={() => onUninstall(plugin)}
              disabled={isPending}
            >
              Uninstall
            </Button>
          )
        ) : (
          <Button size="sm" className="flex-1" onClick={() => onInstall(plugin)} disabled={isPending}>
            Install
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}

function PluginDetailsModal({
  plugin,
  open,
  onClose,
  onInstall,
  onUninstall,
  onUpdate,
  isPending,
}: {
  plugin: Plugin | null;
  open: boolean;
  onClose: () => void;
  onInstall: (plugin: Plugin) => void;
  onUninstall: (plugin: Plugin) => void;
  onUpdate: (plugin: Plugin) => void;
  isPending: boolean;
}) {
  if (!plugin) return null;

  const CategoryIcon = CATEGORY_ICONS[plugin.category];

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-2xl max-h-[90vh]">
        <DialogHeader>
          <div className="flex items-start gap-4">
            <div className="flex h-16 w-16 items-center justify-center rounded-xl bg-primary/10">
              <CategoryIcon className="h-8 w-8 text-primary" />
            </div>
            <div className="flex-1">
              <div className="flex items-center gap-2">
                <DialogTitle>{plugin.displayName}</DialogTitle>
                {plugin.installed && (
                  <Badge variant="default">
                    <Check className="h-3 w-3 mr-1" />
                    Installed
                  </Badge>
                )}
                {plugin.hasUpdate && (
                  <Badge variant="secondary">
                    <Sparkles className="h-3 w-3 mr-1" />
                    Update Available
                  </Badge>
                )}
              </div>
              <DialogDescription className="mt-1">{plugin.description}</DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <ScrollArea className="max-h-[50vh]">
          <div className="space-y-4 pr-4">
            <div className="flex flex-wrap gap-4 text-sm">
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Version:</span>
                <span className="font-medium">{plugin.version}</span>
                {plugin.hasUpdate && (
                  <span className="text-primary">→ {plugin.latestVersion}</span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Author:</span>
                <span className="font-medium">{plugin.author}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">License:</span>
                <span className="font-medium">{plugin.license}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Category:</span>
                <Badge variant="outline">{CATEGORY_LABELS[plugin.category]}</Badge>
              </div>
            </div>

            <Separator />

            <div className="flex items-center gap-6">
              <div>
                <div className="flex items-center gap-2">
                  <StarRating rating={plugin.rating} />
                  <span className="font-medium">{plugin.rating}</span>
                </div>
                <p className="text-xs text-muted-foreground">{plugin.reviewCount} reviews</p>
              </div>
              <div>
                <p className="font-medium">{plugin.downloadCount.toLocaleString()}</p>
                <p className="text-xs text-muted-foreground">Downloads</p>
              </div>
              <div>
                <p className="font-medium">{new Date(plugin.updatedAt).toLocaleDateString()}</p>
                <p className="text-xs text-muted-foreground">Last Updated</p>
              </div>
            </div>

            <Separator />

            <div>
              <h4 className="font-medium mb-2">Tags</h4>
              <div className="flex flex-wrap gap-2">
                {plugin.tags.map((tag) => (
                  <Badge key={tag} variant="secondary">
                    {tag}
                  </Badge>
                ))}
              </div>
            </div>

            <div>
              <h4 className="font-medium mb-2">About</h4>
              <p className="text-sm text-muted-foreground">
                This plugin provides {plugin.category} functionality for your API gateway.
                It is maintained by {plugin.author} and released under the {plugin.license} license.
              </p>
            </div>

            {plugin.configSchema && (
              <div>
                <h4 className="font-medium mb-2">Configuration Schema</h4>
                <pre className="text-xs bg-muted p-3 rounded-md overflow-x-auto">
                  {JSON.stringify(plugin.configSchema, null, 2)}
                </pre>
              </div>
            )}
          </div>
        </ScrollArea>

        <DialogFooter className="gap-2">
          {plugin.homepage && (
            <Button variant="outline" asChild className="mr-auto">
              <a href={plugin.homepage} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4 mr-2" />
                Documentation
              </a>
            </Button>
          )}
          {plugin.installed ? (
            plugin.hasUpdate ? (
              <>
                <Button variant="outline" onClick={() => onUninstall(plugin)} disabled={isPending}>
                  Uninstall
                </Button>
                <Button onClick={() => onUpdate(plugin)} disabled={isPending}>
                  <Sparkles className="h-4 w-4 mr-2" />
                  Update to v{plugin.latestVersion}
                </Button>
              </>
            ) : (
              <Button variant="outline" onClick={() => onUninstall(plugin)} disabled={isPending}>
                <X className="h-4 w-4 mr-2" />
                Uninstall
              </Button>
            )
          ) : (
            <Button onClick={() => onInstall(plugin)} disabled={isPending}>
              <Download className="h-4 w-4 mr-2" />
              Install Plugin
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function PluginMarketplacePage() {
  const queryClient = useQueryClient();
  const [searchQuery, setSearchQuery] = useState("");
  const [categoryFilter, setCategoryFilter] = useState<PluginCategory>("all");
  const [statusFilter, setStatusFilter] = useState<PluginStatus>("all");
  const [sortBy, setSortBy] = useState<SortOption>("popular");
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);

  const pluginsQuery = useQuery({
    queryKey: ["marketplace-plugins"],
    queryFn: async () => {
      try {
        return await adminApiRequest<Plugin[]>("/admin/api/v1/marketplace/plugins");
      } catch {
        return FALLBACK_PLUGINS;
      }
    },
  });

  const installMutation = useMutation({
    mutationFn: async (pluginId: string) =>
      adminApiRequest(`/admin/api/v1/marketplace/plugins/${pluginId}/install`, {
        method: "POST",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["marketplace-plugins"] });
      queryClient.invalidateQueries({ queryKey: ["plugins"] });
    },
  });

  const uninstallMutation = useMutation({
    mutationFn: async (pluginId: string) =>
      adminApiRequest(`/admin/api/v1/marketplace/plugins/${pluginId}/uninstall`, {
        method: "POST",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["marketplace-plugins"] });
      queryClient.invalidateQueries({ queryKey: ["plugins"] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async (pluginId: string) =>
      adminApiRequest(`/admin/api/v1/marketplace/plugins/${pluginId}/update`, {
        method: "POST",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["marketplace-plugins"] });
      queryClient.invalidateQueries({ queryKey: ["plugins"] });
    },
  });

  const plugins = pluginsQuery.data ?? FALLBACK_PLUGINS;

  const filteredPlugins = useMemo(() => {
    let result = [...plugins];

    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      result = result.filter(
        (p) =>
          p.displayName.toLowerCase().includes(query) ||
          p.description.toLowerCase().includes(query) ||
          p.tags.some((t) => t.toLowerCase().includes(query))
      );
    }

    if (categoryFilter !== "all") {
      result = result.filter((p) => p.category === categoryFilter);
    }

    if (statusFilter !== "all") {
      switch (statusFilter) {
        case "installed":
          result = result.filter((p) => p.installed);
          break;
        case "available":
          result = result.filter((p) => !p.installed);
          break;
        case "update":
          result = result.filter((p) => p.hasUpdate);
          break;
      }
    }

    result.sort((a, b) => {
      switch (sortBy) {
        case "popular":
          return b.downloadCount - a.downloadCount;
        case "newest":
          return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime();
        case "rating":
          return b.rating - a.rating;
        case "name":
          return a.displayName.localeCompare(b.displayName);
        default:
          return 0;
      }
    });

    return result;
  }, [plugins, searchQuery, categoryFilter, statusFilter, sortBy]);

  const stats = useMemo(() => {
    return {
      total: plugins.length,
      installed: plugins.filter((p) => p.installed).length,
      available: plugins.filter((p) => !p.installed).length,
      updates: plugins.filter((p) => p.hasUpdate).length,
    };
  }, [plugins]);

  const handleInstall = (plugin: Plugin) => {
    installMutation.mutate(plugin.id);
  };

  const handleUninstall = (plugin: Plugin) => {
    uninstallMutation.mutate(plugin.id);
  };

  const handleUpdate = (plugin: Plugin) => {
    updateMutation.mutate(plugin.id);
  };

  const handleViewDetails = (plugin: Plugin) => {
    setSelectedPlugin(plugin);
    setDetailsOpen(true);
  };

  const isPending = installMutation.isPending || uninstallMutation.isPending || updateMutation.isPending;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Plugin Marketplace</h1>
          <p className="text-muted-foreground">
            Discover and install plugins to extend your gateway functionality
          </p>
        </div>
        <div className="flex items-center gap-2">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex items-center gap-2 px-3 py-1.5 bg-muted rounded-md text-sm">
                  <Check className="h-4 w-4 text-primary" />
                  <span>{stats.installed} installed</span>
                </div>
              </TooltipTrigger>
              <TooltipContent>
                <p>{stats.installed} plugins installed</p>
                <p>{stats.updates} updates available</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <Card>
          <CardContent className="p-4">
            <p className="text-2xl font-bold">{stats.total}</p>
            <p className="text-sm text-muted-foreground">Total Plugins</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-2xl font-bold">{stats.installed}</p>
            <p className="text-sm text-muted-foreground">Installed</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-2xl font-bold">{stats.available}</p>
            <p className="text-sm text-muted-foreground">Available</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-2xl font-bold text-primary">{stats.updates}</p>
            <p className="text-sm text-muted-foreground">Updates</p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-center gap-2 flex-1">
              <Search className="h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search plugins..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="max-w-sm"
              />
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Select value={categoryFilter} onValueChange={(v) => setCategoryFilter(v as PluginCategory)}>
                <SelectTrigger className="w-40">
                  <Filter className="h-4 w-4 mr-2" />
                  <SelectValue placeholder="Category" />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(CATEGORY_LABELS).map(([key, label]) => (
                    <SelectItem key={key} value={key}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v as PluginStatus)}>
                <SelectTrigger className="w-36">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Status</SelectItem>
                  <SelectItem value="installed">Installed</SelectItem>
                  <SelectItem value="available">Available</SelectItem>
                  <SelectItem value="update">Updates</SelectItem>
                </SelectContent>
              </Select>

              <Select value={sortBy} onValueChange={(v) => setSortBy(v as SortOption)}>
                <SelectTrigger className="w-36">
                  <SelectValue placeholder="Sort by" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="popular">Most Popular</SelectItem>
                  <SelectItem value="newest">Newest</SelectItem>
                  <SelectItem value="rating">Highest Rated</SelectItem>
                  <SelectItem value="name">Name</SelectItem>
                </SelectContent>
              </Select>

              <div className="flex items-center border rounded-md">
                <Button
                  variant={viewMode === "grid" ? "secondary" : "ghost"}
                  size="icon"
                  className="h-9 w-9 rounded-none rounded-l-md"
                  onClick={() => setViewMode("grid")}
                >
                  <Grid3X3 className="h-4 w-4" />
                </Button>
                <Button
                  variant={viewMode === "list" ? "secondary" : "ghost"}
                  size="icon"
                  className="h-9 w-9 rounded-none rounded-r-md"
                  onClick={() => setViewMode("list")}
                >
                  <List className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        </CardHeader>

        <CardContent>
          {pluginsQuery.isLoading ? (
            <div className="flex items-center justify-center h-64">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
            </div>
          ) : filteredPlugins.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-64 text-center">
              <Package className="h-12 w-12 text-muted-foreground mb-4" />
              <h3 className="text-lg font-medium">No plugins found</h3>
              <p className="text-sm text-muted-foreground max-w-sm">
                Try adjusting your search or filters to find what you&apos;re looking for.
              </p>
            </div>
          ) : (
            <div
              className={cn(
                viewMode === "grid"
                  ? "grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4"
                  : "space-y-2"
              )}
            >
              {filteredPlugins.map((plugin) => (
                <PluginCard
                  key={plugin.id}
                  plugin={plugin}
                  viewMode={viewMode}
                  onInstall={handleInstall}
                  onUninstall={handleUninstall}
                  onUpdate={handleUpdate}
                  onViewDetails={handleViewDetails}
                  isPending={isPending}
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <PluginDetailsModal
        plugin={selectedPlugin}
        open={detailsOpen}
        onClose={() => setDetailsOpen(false)}
        onInstall={handleInstall}
        onUninstall={handleUninstall}
        onUpdate={handleUpdate}
        isPending={isPending}
      />
    </div>
  );
}
