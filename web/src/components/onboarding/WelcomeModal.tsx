import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Shield,
  Zap,
  LineChart,
  Users,
  ArrowRight,
  Sparkles,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface WelcomeModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStartTour?: () => void;
  onStartSetup?: () => void;
}

const FEATURES = [
  {
    icon: Shield,
    title: "Secure API Gateway",
    description: "Protect your APIs with authentication, rate limiting, and access control.",
    color: "text-emerald-500",
    bgColor: "bg-emerald-500/10",
  },
  {
    icon: Zap,
    title: "High Performance",
    description: "Built for speed with efficient routing, caching, and load balancing.",
    color: "text-amber-500",
    bgColor: "bg-amber-500/10",
  },
  {
    icon: LineChart,
    title: "Real-time Analytics",
    description: "Monitor traffic, latency, and errors with comprehensive dashboards.",
    color: "text-blue-500",
    bgColor: "bg-blue-500/10",
  },
  {
    icon: Users,
    title: "Multi-tenant",
    description: "Manage multiple consumers, services, and routes with ease.",
    color: "text-purple-500",
    bgColor: "bg-purple-500/10",
  },
];

export function WelcomeModal({ open, onOpenChange, onStartTour, onStartSetup }: WelcomeModalProps) {
  const [dontShowAgain, setDontShowAgain] = useState(false);
  const [step, setStep] = useState<"welcome" | "features">("welcome");

  const handleClose = () => {
    if (dontShowAgain) {
      localStorage.setItem("apicerberus.welcome_shown", "true");
    }
    onOpenChange(false);
  };

  const handleStartTour = () => {
    handleClose();
    onStartTour?.();
  };

  const handleStartSetup = () => {
    handleClose();
    onStartSetup?.();
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-hidden p-0 gap-0">
        {step === "welcome" ? (
          <>
            <div className="relative overflow-hidden bg-gradient-to-br from-primary/10 via-primary/5 to-background p-6 pb-8">
              <div className="absolute top-0 right-0 p-4 opacity-10">
                <Shield className="h-32 w-32" />
              </div>
              <DialogHeader className="relative z-10">
                <div className="flex items-center gap-2 mb-2">
                  <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/20">
                    <Sparkles className="h-5 w-5 text-primary" />
                  </div>
                  <span className="text-sm font-medium text-primary">Welcome to API Cerebrus</span>
                </div>
                <DialogTitle className="text-2xl sm:text-3xl">
                  Your API Gateway is Ready
                </DialogTitle>
                <DialogDescription className="text-base mt-2 max-w-md">
                  Thank you for choosing API Cerebrus. Let&apos;s get you started with a quick tour
                  or set up your first API route.
                </DialogDescription>
              </DialogHeader>
            </div>

            <div className="p-6 space-y-6">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <button
                  onClick={handleStartSetup}
                  className="group relative flex flex-col items-start p-4 rounded-xl border-2 border-primary/20 bg-primary/5 hover:border-primary/50 hover:bg-primary/10 transition-all text-left"
                >
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary text-primary-foreground mb-3">
                    <Zap className="h-5 w-5" />
                  </div>
                  <h3 className="font-semibold mb-1">Quick Setup</h3>
                  <p className="text-sm text-muted-foreground">
                    Create your first route, service, and upstream in just a few steps.
                  </p>
                  <ArrowRight className="absolute bottom-4 right-4 h-5 w-5 text-primary opacity-0 group-hover:opacity-100 transition-opacity" />
                </button>

                <button
                  onClick={handleStartTour}
                  className="group relative flex flex-col items-start p-4 rounded-xl border hover:border-primary/50 hover:bg-muted/50 transition-all text-left"
                >
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted mb-3">
                    <LineChart className="h-5 w-5" />
                  </div>
                  <h3 className="font-semibold mb-1">Take a Tour</h3>
                  <p className="text-sm text-muted-foreground">
                    Learn about the dashboard features and how to use them effectively.
                  </p>
                  <ArrowRight className="absolute bottom-4 right-4 h-5 w-5 opacity-0 group-hover:opacity-100 transition-opacity" />
                </button>
              </div>

              <div className="flex items-center justify-between pt-4 border-t">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="dont-show"
                    checked={dontShowAgain}
                    onCheckedChange={(checked) => setDontShowAgain(checked as boolean)}
                  />
                  <label
                    htmlFor="dont-show"
                    className="text-sm text-muted-foreground cursor-pointer"
                  >
                    Don&apos;t show this again
                  </label>
                </div>
                <div className="flex gap-2">
                  <Button variant="ghost" onClick={() => setStep("features")}>
                    Learn More
                  </Button>
                  <Button variant="outline" onClick={handleClose}>
                    Skip for Now
                  </Button>
                </div>
              </div>
            </div>
          </>
        ) : (
          <>
            <DialogHeader className="p-6 pb-0">
              <DialogTitle>What You Can Do</DialogTitle>
              <DialogDescription>
                Explore the powerful features of API Cerebrus
              </DialogDescription>
            </DialogHeader>

            <div className="p-6 space-y-4 overflow-y-auto max-h-[50vh]">
              {FEATURES.map((feature) => (
                <div
                  key={feature.title}
                  className="flex items-start gap-4 p-4 rounded-lg border hover:bg-muted/50 transition-colors"
                >
                  <div className={cn("flex h-10 w-10 shrink-0 items-center justify-center rounded-lg", feature.bgColor)}>
                    <feature.icon className={cn("h-5 w-5", feature.color)} />
                  </div>
                  <div>
                    <h3 className="font-semibold">{feature.title}</h3>
                    <p className="text-sm text-muted-foreground">{feature.description}</p>
                  </div>
                </div>
              ))}
            </div>

            <DialogFooter className="p-6 pt-4 border-t">
              <Button variant="ghost" onClick={() => setStep("welcome")}>
                Back
              </Button>
              <Button onClick={handleStartSetup}>
                Start Quick Setup
                <ArrowRight className="h-4 w-4 ml-2" />
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

export function useWelcomeModal() {
  const [showWelcome, setShowWelcome] = useState(false);

  useEffect(() => {
    const hasSeenWelcome = localStorage.getItem("apicerberus.welcome_shown");
    if (!hasSeenWelcome) {
      // Small delay to let the page load first
      const timer = setTimeout(() => {
        setShowWelcome(true);
      }, 500);
      return () => clearTimeout(timer);
    }
  }, []);

  return {
    showWelcome,
    setShowWelcome,
  };
}
