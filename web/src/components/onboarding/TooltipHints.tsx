import { useState, useEffect } from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface HintProps {
  children: React.ReactNode;
  content: string;
  side?: "top" | "bottom" | "left" | "right";
  showIndicator?: boolean;
  storageKey?: string;
}

export function TooltipHint({
  children,
  content,
  side = "bottom",
  showIndicator = true,
  storageKey,
}: HintProps) {
  const [isDismissed, setIsDismissed] = useState(false);
  const [showPulse, setShowPulse] = useState(true);

  useEffect(() => {
    if (storageKey) {
      const dismissed = localStorage.getItem(`hint_${storageKey}`);
      if (dismissed) {
        setIsDismissed(true);
      }
    }

    // Stop pulsing after 5 seconds
    const timer = setTimeout(() => setShowPulse(false), 5000);
    return () => clearTimeout(timer);
  }, [storageKey]);

  const handleDismiss = () => {
    if (storageKey) {
      localStorage.setItem(`hint_${storageKey}`, "true");
    }
    setIsDismissed(true);
  };

  if (isDismissed) {
    return <>{children}</>;
  }

  return (
    <TooltipProvider delayDuration={100}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="relative inline-block">
            {children}
            {showIndicator && showPulse && (
              <span className="absolute -top-1 -right-1 flex h-3 w-3">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary opacity-75"></span>
                <span className="relative inline-flex rounded-full h-3 w-3 bg-primary"></span>
              </span>
            )}
          </div>
        </TooltipTrigger>
        <TooltipContent
          side={side}
          className={cn(
            "max-w-xs p-3",
            "bg-popover border-2 border-primary/20"
          )}
        >
          <p className="text-sm">{content}</p>
          {storageKey && (
            <button
              onClick={handleDismiss}
              className="text-xs text-muted-foreground hover:text-foreground mt-2 underline"
            >
              Don&apos;t show again
            </button>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

interface FeatureHighlightProps {
  children: React.ReactNode;
  title: string;
  description: string;
  isOpen: boolean;
  onClose: () => void;
}

export function FeatureHighlight({
  children,
  title,
  description,
  isOpen,
  onClose,
}: FeatureHighlightProps) {
  useEffect(() => {
    if (isOpen) {
      const timer = setTimeout(onClose, 5000);
      return () => clearTimeout(timer);
    }
  }, [isOpen, onClose]);

  return (
    <div className="relative">
      {children}
      {isOpen && (
        <div className="absolute z-50 mt-2 p-3 bg-popover border-2 border-primary/20 rounded-lg shadow-lg animate-in fade-in slide-in-from-top-2 max-w-xs">
          <h4 className="font-semibold text-sm">{title}</h4>
          <p className="text-xs text-muted-foreground mt-1">{description}</p>
          <button
            onClick={onClose}
            className="text-xs text-primary mt-2 hover:underline"
          >
            Got it
          </button>
        </div>
      )}
    </div>
  );
}

// Contextual hints for specific features
export const CONTEXTUAL_HINTS = {
  routeBuilder: {
    content: "Use the Route Builder to visually design your API routes with drag-and-drop simplicity.",
    storageKey: "route_builder_hint",
  },
  pluginMarketplace: {
    content: "Browse the Plugin Marketplace to extend your gateway with authentication, caching, and more.",
    storageKey: "plugin_marketplace_hint",
  },
  analytics: {
    content: "View detailed metrics and filter by time range, route, or consumer.",
    storageKey: "analytics_hint",
  },
  logViewer: {
    content: "Filter logs by level, source, status code, and more. Click any log to see full details.",
    storageKey: "log_viewer_hint",
  },
  cluster: {
    content: "Monitor your cluster nodes and their health status in real-time.",
    storageKey: "cluster_hint",
  },
};
