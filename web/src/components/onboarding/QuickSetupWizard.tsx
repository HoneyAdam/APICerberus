import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { adminApiRequest } from "@/lib/api";
import { toast } from "sonner";
import {
  CheckCircle2,
  ArrowRight,
  ArrowLeft,
  Server,
  Route,
  Network,
  Shield,
  Sparkles,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface QuickSetupWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type WizardStep = "upstream" | "service" | "route" | "plugins" | "review" | "success";

interface WizardData {
  upstream: {
    name: string;
    host: string;
    port: number;
    protocol: "http" | "https";
  } | null;
  service: {
    name: string;
    path: string;
    retries: number;
    timeout: number;
    upstreamId?: string;
  } | null;
  route: {
    name: string;
    paths: string[];
    methods: string[];
    stripPath: boolean;
    serviceId?: string;
  } | null;
  plugins: {
    rateLimit: boolean;
    auth: boolean;
    cors: boolean;
    logging: boolean;
  };
}

const DEFAULT_DATA: WizardData = {
  upstream: {
    name: "",
    host: "",
    port: 8080,
    protocol: "http",
  },
  service: {
    name: "",
    path: "/",
    retries: 3,
    timeout: 30000,
  },
  route: {
    name: "",
    paths: ["/api"],
    methods: ["GET", "POST"],
    stripPath: false,
  },
  plugins: {
    rateLimit: true,
    auth: false,
    cors: true,
    logging: true,
  },
};

const STEPS: { id: WizardStep; title: string; icon: typeof Server }[] = [
  { id: "upstream", title: "Upstream", icon: Network },
  { id: "service", title: "Service", icon: Server },
  { id: "route", title: "Route", icon: Route },
  { id: "plugins", title: "Plugins", icon: Shield },
  { id: "review", title: "Review", icon: CheckCircle2 },
];

export function QuickSetupWizard({ open, onOpenChange }: QuickSetupWizardProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [currentStep, setCurrentStep] = useState<WizardStep>("upstream");
  const [data, setData] = useState<WizardData>(DEFAULT_DATA);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const createUpstreamMutation = useMutation<{ id: string }, Error, NonNullable<WizardData['upstream']>>({
    mutationFn: (upstream: typeof data.upstream) =>
      adminApiRequest("/admin/api/v1/upstreams", {
        method: "POST",
        body: upstream,
      }),
  });

  const createServiceMutation = useMutation<{ id: string }, Error, NonNullable<WizardData['service']> & { upstreamId: string }>({
    mutationFn: (service: typeof data.service) =>
      adminApiRequest("/admin/api/v1/services", {
        method: "POST",
        body: service,
      }),
  });

  const createRouteMutation = useMutation<{ id: string }, Error, NonNullable<WizardData['route']> & { serviceId: string }>({
    mutationFn: (route: typeof data.route) =>
      adminApiRequest("/admin/api/v1/routes", {
        method: "POST",
        body: route,
      }),
  });

  const updateStep = <K extends keyof WizardData>(step: K, updates: Partial<WizardData[K]>) => {
    setData((prev) => ({
      ...prev,
      [step]: { ...prev[step], ...updates },
    }));
  };

  const canProceed = () => {
    switch (currentStep) {
      case "upstream":
        return data.upstream?.name && data.upstream?.host;
      case "service":
        return data.service?.name;
      case "route":
        return data.route?.name && (data.route?.paths ?? []).length > 0;
      default:
        return true;
    }
  };

  const handleNext = () => {
    const currentIndex = STEPS.findIndex((s) => s.id === currentStep);
    if (currentIndex < STEPS.length - 1) {
      setCurrentStep(STEPS[currentIndex + 1].id);
    }
  };

  const handleBack = () => {
    const currentIndex = STEPS.findIndex((s) => s.id === currentStep);
    if (currentIndex > 0) {
      setCurrentStep(STEPS[currentIndex - 1].id);
    }
  };

  const handleSubmit = async () => {
    setIsSubmitting(true);
    try {
      if (!data.upstream || !data.service || !data.route) {
        toast.error("Missing required configuration");
        return;
      }

      // Create upstream
      const upstream = await createUpstreamMutation.mutateAsync(data.upstream) as { id: string };
      toast.success(`Upstream "${data.upstream?.name ?? ''}" created`);

      // Create service with upstream reference
      const service = await createServiceMutation.mutateAsync({
        ...data.service,
        upstreamId: upstream.id,
      });
      toast.success(`Service "${data.service?.name ?? ''}" created`);

      // Create route with service reference
      const route = await createRouteMutation.mutateAsync({
        ...data.route,
        serviceId: service.id,
      });
      toast.success(`Route "${data.route?.name ?? ''}" created`);

      // Apply plugins if selected
      if (data.plugins.rateLimit) {
        await adminApiRequest("/admin/api/v1/plugins", {
          method: "POST",
          body: {
            name: "rate_limit",
            routeId: route.id,
            config: { algorithm: "token_bucket", limit: 100, window: 60 },
          },
        });
      }

      if (data.plugins.cors) {
        await adminApiRequest("/admin/api/v1/plugins", {
          method: "POST",
          body: {
            name: "cors",
            routeId: route.id,
            config: { origins: ["*"], methods: ["GET", "POST", "PUT", "DELETE"] },
          },
        });
      }

      // Invalidate queries
      queryClient.invalidateQueries({ queryKey: ["upstreams"] });
      queryClient.invalidateQueries({ queryKey: ["services"] });
      queryClient.invalidateQueries({ queryKey: ["routes"] });
      queryClient.invalidateQueries({ queryKey: ["plugins"] });

      setCurrentStep("success");
    } catch (error) {
      toast.error("Failed to create resources. Please try again.");
      console.error(error);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleFinish = () => {
    onOpenChange(false);
    navigate("/routes");
  };

  const renderStepContent = () => {
    switch (currentStep) {
      case "upstream":
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="upstream-name">Upstream Name *</Label>
              <Input
                id="upstream-name"
                placeholder="e.g., my-backend"
                value={data.upstream?.name ?? ''}
                onChange={(e) => updateStep("upstream", { name: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">
                A unique name to identify this upstream
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="upstream-host">Host *</Label>
                <Input
                  id="upstream-host"
                  placeholder="e.g., api.example.com"
                  value={data.upstream?.host ?? ""}
                  onChange={(e) => updateStep("upstream", { host: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="upstream-port">Port</Label>
                <Input
                  id="upstream-port"
                  type="number"
                  value={data.upstream?.port ?? 8080}
                  onChange={(e) => updateStep("upstream", { port: parseInt(e.target.value) })}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="upstream-protocol">Protocol</Label>
              <Select
                value={data.upstream?.protocol ?? "http"}
                onValueChange={(v) => updateStep("upstream", { protocol: v as "http" | "https" })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="http">HTTP</SelectItem>
                  <SelectItem value="https">HTTPS</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        );

      case "service":
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="service-name">Service Name *</Label>
              <Input
                id="service-name"
                placeholder="e.g., user-service"
                value={data.service?.name ?? ''}
                onChange={(e) => updateStep("service", { name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="service-path">Base Path</Label>
              <Input
                id="service-path"
                placeholder="e.g., /api/v1"
                value={data.service?.path ?? ''}
                onChange={(e) => updateStep("service", { path: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">
                Path prefix to add to all requests
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="service-retries">Retries</Label>
                <Input
                  id="service-retries"
                  type="number"
                  min={0}
                  max={10}
                  value={data.service?.retries ?? 3}
                  onChange={(e) => updateStep("service", { retries: parseInt(e.target.value) })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="service-timeout">Timeout (ms)</Label>
                <Input
                  id="service-timeout"
                  type="number"
                  step={1000}
                  value={data.service?.timeout ?? 30000}
                  onChange={(e) => updateStep("service", { timeout: parseInt(e.target.value) })}
                />
              </div>
            </div>
          </div>
        );

      case "route":
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="route-name">Route Name *</Label>
              <Input
                id="route-name"
                placeholder="e.g., users-api"
                value={data.route?.name ?? ''}
                onChange={(e) => updateStep("route", { name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label>Paths *</Label>
              <Input
                placeholder="e.g., /users"
                value={(data.route?.paths ?? [])[0] || ""}
                onChange={(e) => updateStep("route", { paths: [e.target.value] })}
              />
              <p className="text-xs text-muted-foreground">
                The path prefix that will trigger this route
              </p>
            </div>

            <div className="space-y-2">
              <Label>HTTP Methods</Label>
              <div className="flex flex-wrap gap-2">
                {["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"].map((method) => (
                  <label
                    key={method}
                    className="flex items-center space-x-2 px-3 py-1.5 rounded-md border cursor-pointer hover:bg-muted"
                  >
                    <Checkbox
                      checked={(data.route?.methods ?? []).includes(method)}
                      onCheckedChange={(checked) => {
                        if (checked) {
                          updateStep("route", { methods: [...data.route?.methods ?? [], method] });
                        } else {
                          updateStep("route", {
                            methods: (data.route?.methods ?? []).filter((m) => m !== method),
                          });
                        }
                      }}
                    />
                    <span className="text-sm">{method}</span>
                  </label>
                ))}
              </div>
            </div>

            <div className="flex items-center space-x-2">
              <Checkbox
                id="strip-path"
                checked={data.route?.stripPath ?? false}
                onCheckedChange={(checked) => updateStep("route", { stripPath: checked as boolean })}
              />
              <label htmlFor="strip-path" className="text-sm cursor-pointer">
                Strip path prefix before forwarding to upstream
              </label>
            </div>
          </div>
        );

      case "plugins":
        return (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Select plugins to enable for your new route:
            </p>

            <div className="space-y-3">
              <label className="flex items-start gap-3 p-3 rounded-lg border cursor-pointer hover:bg-muted/50 transition-colors">
                <Checkbox
                  checked={data.plugins.rateLimit}
                  onCheckedChange={(checked) =>
                    updateStep("plugins", { rateLimit: checked as boolean })
                  }
                  className="mt-1"
                />
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">Rate Limiting</span>
                    <span className="text-xs bg-primary/10 text-primary px-2 py-0.5 rounded">Recommended</span>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Limit requests to 100 per minute using token bucket algorithm
                  </p>
                </div>
              </label>

              <label className="flex items-start gap-3 p-3 rounded-lg border cursor-pointer hover:bg-muted/50 transition-colors">
                <Checkbox
                  checked={data.plugins.cors}
                  onCheckedChange={(checked) => updateStep("plugins", { cors: checked as boolean })}
                  className="mt-1"
                />
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">CORS</span>
                    <span className="text-xs bg-primary/10 text-primary px-2 py-0.5 rounded">Recommended</span>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Enable cross-origin requests from any origin
                  </p>
                </div>
              </label>

              <label className="flex items-start gap-3 p-3 rounded-lg border cursor-pointer hover:bg-muted/50 transition-colors">
                <Checkbox
                  checked={data.plugins.auth}
                  onCheckedChange={(checked) => updateStep("plugins", { auth: checked as boolean })}
                  className="mt-1"
                />
                <div className="flex-1">
                  <span className="font-medium">API Key Authentication</span>
                  <p className="text-sm text-muted-foreground">
                    Require API key for accessing this route
                  </p>
                </div>
              </label>

              <label className="flex items-start gap-3 p-3 rounded-lg border cursor-pointer hover:bg-muted/50 transition-colors">
                <Checkbox
                  checked={data.plugins.logging}
                  onCheckedChange={(checked) => updateStep("plugins", { logging: checked as boolean })}
                  className="mt-1"
                />
                <div className="flex-1">
                  <span className="font-medium">Request Logging</span>
                  <p className="text-sm text-muted-foreground">
                    Log all requests for monitoring and debugging
                  </p>
                </div>
              </label>
            </div>
          </div>
        );

      case "review":
        return (
          <div className="space-y-4">
            <div className="space-y-3">
              <div className="p-3 rounded-lg border">
                <div className="flex items-center gap-2 mb-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  <span className="font-medium">Upstream</span>
                </div>
                <div className="text-sm text-muted-foreground pl-6">
                  <p>Name: {data.upstream?.name ?? ''}</p>
                  <p>Host: {data.upstream?.protocol ?? 'http'}://{data.upstream?.host ?? ''}:{data.upstream?.port ?? 8080}</p>
                </div>
              </div>

              <div className="p-3 rounded-lg border">
                <div className="flex items-center gap-2 mb-2">
                  <Server className="h-4 w-4 text-muted-foreground" />
                  <span className="font-medium">Service</span>
                </div>
                <div className="text-sm text-muted-foreground pl-6">
                  <p>Name: {data.service?.name ?? ''}</p>
                  <p>Path: {data.service?.path ?? ''}</p>
                  <p>Timeout: {data.service?.timeout ?? 30000}ms, Retries: {data.service?.retries ?? 3}</p>
                </div>
              </div>

              <div className="p-3 rounded-lg border">
                <div className="flex items-center gap-2 mb-2">
                  <Route className="h-4 w-4 text-muted-foreground" />
                  <span className="font-medium">Route</span>
                </div>
                <div className="text-sm text-muted-foreground pl-6">
                  <p>Name: {data.route?.name ?? ''}</p>
                  <p>Paths: {data.route?.paths ?? [].join(", ")}</p>
                  <p>Methods: {data.route?.methods ?? [].join(", ")}</p>
                </div>
              </div>

              <div className="p-3 rounded-lg border">
                <div className="flex items-center gap-2 mb-2">
                  <Shield className="h-4 w-4 text-muted-foreground" />
                  <span className="font-medium">Plugins</span>
                </div>
                <div className="flex flex-wrap gap-1 pl-6">
                  {data.plugins.rateLimit && <Badge variant="secondary">Rate Limit</Badge>}
                  {data.plugins.cors && <Badge variant="secondary">CORS</Badge>}
                  {data.plugins.auth && <Badge variant="secondary">Auth</Badge>}
                  {data.plugins.logging && <Badge variant="secondary">Logging</Badge>}
                </div>
              </div>
            </div>
          </div>
        );

      case "success":
        return (
          <div className="text-center py-8">
            <div className="flex justify-center mb-4">
              <div className="h-16 w-16 rounded-full bg-green-100 flex items-center justify-center">
                <CheckCircle2 className="h-8 w-8 text-green-600" />
              </div>
            </div>
            <h3 className="text-xl font-semibold mb-2">Setup Complete!</h3>
            <p className="text-muted-foreground max-w-sm mx-auto">
              Your first API route is now configured and ready to handle requests.
              You can view and manage it in the Routes section.
            </p>
          </div>
        );
    }
  };

  const currentStepIndex = STEPS.findIndex((s) => s.id === currentStep);
  const isLastStep = currentStep === "review";
  const isSuccess = currentStep === "success";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl max-h-[90vh] overflow-hidden p-0 gap-0">
        {!isSuccess && (
          <div className="px-6 pt-6">
            <div className="flex items-center gap-2 mb-6">
              {STEPS.map((step, index) => {
                const StepIcon = step.icon;
                const isActive = step.id === currentStep;
                const isCompleted = index < currentStepIndex;

                return (
                  <div key={step.id} className="flex items-center">
                    <div
                      className={cn(
                        "flex items-center justify-center w-8 h-8 rounded-full text-xs font-medium transition-colors",
                        isActive && "bg-primary text-primary-foreground",
                        isCompleted && "bg-primary/20 text-primary",
                        !isActive && !isCompleted && "bg-muted text-muted-foreground"
                      )}
                    >
                      {isCompleted ? (
                        <CheckCircle2 className="h-4 w-4" />
                      ) : (
                        <StepIcon className="h-4 w-4" />
                      )}
                    </div>
                    {index < STEPS.length - 1 && (
                      <div
                        className={cn(
                          "w-8 h-0.5 mx-1",
                          isCompleted ? "bg-primary" : "bg-muted"
                        )}
                      />
                    )}
                  </div>
                );
              })}
            </div>

            <DialogHeader>
              <DialogTitle>{STEPS[currentStepIndex]?.title}</DialogTitle>
              <DialogDescription>
                {currentStep === "upstream" && "Configure your backend server"}
                {currentStep === "service" && "Define how to connect to your upstream"}
                {currentStep === "route" && "Set up the public endpoint"}
                {currentStep === "plugins" && "Add functionality to your route"}
                {currentStep === "review" && "Review your configuration before creating"}
              </DialogDescription>
            </DialogHeader>
          </div>
        )}

        <div className={cn("px-6", isSuccess ? "py-6" : "pb-6")}>
          {renderStepContent()}
        </div>

        {!isSuccess && (
          <DialogFooter className="px-6 py-4 border-t">
            <Button
              variant="ghost"
              onClick={handleBack}
              disabled={currentStepIndex === 0}
            >
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back
            </Button>
            <div className="flex-1" />
            {isLastStep ? (
              <Button onClick={handleSubmit} disabled={isSubmitting}>
                {isSubmitting ? (
                  <>Creating...</>
                ) : (
                  <>
                    Create Resources
                    <Sparkles className="h-4 w-4 ml-2" />
                  </>
                )}
              </Button>
            ) : (
              <Button onClick={handleNext} disabled={!canProceed()}>
                Next
                <ArrowRight className="h-4 w-4 ml-2" />
              </Button>
            )}
          </DialogFooter>
        )}

        {isSuccess && (
          <DialogFooter className="px-6 py-4 border-t">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Close
            </Button>
            <Button onClick={handleFinish}>
              View Routes
              <ArrowRight className="h-4 w-4 ml-2" />
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
