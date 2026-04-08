import { useEffect, useState, useCallback } from "react";
import { createPortal } from "react-dom";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { X, ChevronRight, ChevronLeft } from "lucide-react";
import { cn } from "@/lib/utils";

export interface TourStep {
  target: string;
  title: string;
  content: string;
  placement?: "top" | "bottom" | "left" | "right";
  action?: string;
}

interface TourTooltipProps {
  steps: TourStep[];
  isOpen: boolean;
  onClose: () => void;
  onComplete?: () => void;
}

export function TourTooltip({ steps, isOpen, onClose, onComplete }: TourTooltipProps) {
  const [currentStep, setCurrentStep] = useState(0);
  const [position, setPosition] = useState({ top: 0, left: 0 });
  const [targetElement, setTargetElement] = useState<HTMLElement | null>(null);

  const step = steps[currentStep];

  const updatePosition = useCallback(() => {
    if (!step) return;

    const element = document.querySelector(step.target) as HTMLElement;
    if (!element) {
      // Target not found, try next step
      if (currentStep < steps.length - 1) {
        setCurrentStep((prev) => prev + 1);
      }
      return;
    }

    setTargetElement(element);
    const rect = element.getBoundingClientRect();
    const tooltipWidth = 320;
    const tooltipHeight = 200;
    const offset = 16;

    let top = 0;
    let left = 0;

    switch (step.placement || "bottom") {
      case "top":
        top = rect.top - tooltipHeight - offset;
        left = rect.left + rect.width / 2 - tooltipWidth / 2;
        break;
      case "bottom":
        top = rect.bottom + offset;
        left = rect.left + rect.width / 2 - tooltipWidth / 2;
        break;
      case "left":
        top = rect.top + rect.height / 2 - tooltipHeight / 2;
        left = rect.left - tooltipWidth - offset;
        break;
      case "right":
        top = rect.top + rect.height / 2 - tooltipHeight / 2;
        left = rect.right + offset;
        break;
    }

    // Keep within viewport
    const padding = 16;
    top = Math.max(padding, Math.min(top, window.innerHeight - tooltipHeight - padding));
    left = Math.max(padding, Math.min(left, window.innerWidth - tooltipWidth - padding));

    setPosition({ top, left });

    // Highlight target element
    element.style.position = "relative";
    element.style.zIndex = "9999";
    element.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [step, currentStep, steps.length]);

  useEffect(() => {
    if (!isOpen) {
      // Cleanup highlights
      document.querySelectorAll("[data-tour-highlight]").forEach((el) => {
        (el as HTMLElement).style.position = "";
        (el as HTMLElement).style.zIndex = "";
        el.removeAttribute("data-tour-highlight");
      });
      return;
    }

    updatePosition();
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);

    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [isOpen, updatePosition]);

  useEffect(() => {
    if (targetElement) {
      targetElement.setAttribute("data-tour-highlight", "true");
    }
  }, [targetElement]);

  const handleNext = () => {
    if (currentStep < steps.length - 1) {
      // Cleanup current highlight
      if (targetElement) {
        targetElement.style.position = "";
        targetElement.style.zIndex = "";
        targetElement.removeAttribute("data-tour-highlight");
      }
      setCurrentStep((prev) => prev + 1);
    } else {
      handleComplete();
    }
  };

  const handlePrev = () => {
    if (currentStep > 0) {
      // Cleanup current highlight
      if (targetElement) {
        targetElement.style.position = "";
        targetElement.style.zIndex = "";
        targetElement.removeAttribute("data-tour-highlight");
      }
      setCurrentStep((prev) => prev - 1);
    }
  };

  const handleSkip = () => {
    localStorage.setItem("apicerberus.tour_completed", "skipped");
    onClose();
  };

  const handleComplete = () => {
    localStorage.setItem("apicerberus.tour_completed", "true");
    onComplete?.();
    onClose();
  };

  if (!isOpen || !step) return null;

  const tooltip = (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 bg-black/50 z-[9998] pointer-events-none"
        style={{
          clipPath: targetElement
            ? `polygon(
                0% 0%,
                0% 100%,
                ${targetElement.getBoundingClientRect().left}px 100%,
                ${targetElement.getBoundingClientRect().left}px ${targetElement.getBoundingClientRect().top}px,
                ${targetElement.getBoundingClientRect().right}px ${targetElement.getBoundingClientRect().top}px,
                ${targetElement.getBoundingClientRect().right}px ${targetElement.getBoundingClientRect().bottom}px,
                ${targetElement.getBoundingClientRect().left}px ${targetElement.getBoundingClientRect().bottom}px,
                ${targetElement.getBoundingClientRect().left}px 100%,
                100% 100%,
                100% 0%
              )`
            : undefined,
        }}
      />

      {/* Tooltip */}
      <Card
        className={cn(
          "fixed z-[9999] w-80 shadow-xl border-2 border-primary/20",
          "animate-in fade-in zoom-in-95 duration-200"
        )}
        style={{
          top: position.top,
          left: position.left,
        }}
      >
        <CardContent className="p-4">
          {/* Progress */}
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-1">
              {steps.map((_, index) => (
                <div
                  key={index}
                  className={cn(
                    "h-1.5 w-6 rounded-full transition-colors",
                    index <= currentStep ? "bg-primary" : "bg-muted"
                  )}
                />
              ))}
            </div>
            <Button variant="ghost" size="icon" className="h-6 w-6" onClick={handleSkip}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          {/* Content */}
          <div className="space-y-2">
            <h4 className="font-semibold text-lg">{step.title}</h4>
            <p className="text-sm text-muted-foreground">{step.content}</p>
            {step.action && (
              <p className="text-xs text-primary font-medium">{step.action}</p>
            )}
          </div>

          {/* Navigation */}
          <div className="flex items-center justify-between mt-4 pt-3 border-t">
            <Button
              variant="ghost"
              size="sm"
              onClick={handlePrev}
              disabled={currentStep === 0}
            >
              <ChevronLeft className="h-4 w-4 mr-1" />
              Back
            </Button>

            <span className="text-xs text-muted-foreground">
              {currentStep + 1} of {steps.length}
            </span>

            {currentStep === steps.length - 1 ? (
              <Button size="sm" onClick={handleComplete}>
                Finish
              </Button>
            ) : (
              <Button size="sm" onClick={handleNext}>
                Next
                <ChevronRight className="h-4 w-4 ml-1" />
              </Button>
            )}
          </div>
        </CardContent>
      </Card>
    </>
  );

  return createPortal(tooltip, document.body);
}

export const DEFAULT_TOUR_STEPS: TourStep[] = [
  {
    target: "[data-sidebar='sidebar']",
    title: "Navigation",
    content: "Use the sidebar to navigate between different sections of the admin dashboard.",
    placement: "right",
  },
  {
    target: "[href='/']",
    title: "Dashboard",
    content: "Get an overview of your API gateway's health, traffic, and performance metrics.",
    placement: "right",
  },
  {
    target: "[href='/services']",
    title: "Services",
    content: "Manage your backend services and upstream connections.",
    placement: "right",
  },
  {
    target: "[href='/routes']",
    title: "Routes",
    content: "Define public endpoints and configure routing rules.",
    placement: "right",
  },
  {
    target: "[href='/plugins']",
    title: "Plugins",
    content: "Add authentication, rate limiting, and other features to your routes.",
    placement: "right",
  },
  {
    target: "[href='/analytics']",
    title: "Analytics",
    content: "View detailed metrics and insights about your API traffic.",
    placement: "right",
  },
];

export function useTour() {
  const [isOpen, setIsOpen] = useState(false);

  const startTour = () => {
    setIsOpen(true);
  };

  const closeTour = () => {
    setIsOpen(false);
  };

  return {
    isOpen,
    startTour,
    closeTour,
  };
}
