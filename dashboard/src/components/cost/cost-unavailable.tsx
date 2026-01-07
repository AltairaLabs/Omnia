"use client";

import { ReactNode } from "react";
import { AlertTriangle } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface CostUnavailableProps {
  /** Whether cost data is available */
  available: boolean;
  /** Reason why cost data is unavailable */
  reason?: string;
  /** Content to render (will be grayed out if unavailable) */
  children: ReactNode;
  /** Additional class names */
  className?: string;
}

/**
 * Wrapper component that displays children normally when cost data is available,
 * or shows them grayed out with a tooltip explanation when unavailable.
 */
export function CostUnavailable({
  available,
  reason = "Prometheus not configured",
  children,
  className,
}: CostUnavailableProps) {
  if (available) {
    return <>{children}</>;
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            className={cn(
              "relative cursor-not-allowed",
              className
            )}
          >
            {/* Gray overlay */}
            <div className="pointer-events-none opacity-40 grayscale">
              {children}
            </div>
            {/* Unavailable indicator */}
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="bg-muted/80 backdrop-blur-sm rounded-lg px-4 py-2 flex items-center gap-2 text-muted-foreground">
                <AlertTriangle className="h-4 w-4" />
                <span className="text-sm font-medium">Cost tracking unavailable</span>
              </div>
            </div>
          </div>
        </TooltipTrigger>
        <TooltipContent>
          <p>{reason}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

/**
 * Banner component shown at the top of the costs page when Prometheus is not configured.
 */
export function CostUnavailableBanner({ reason }: { reason?: string }) {
  return (
    <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4 flex items-center gap-3">
      <AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-500 flex-shrink-0" />
      <div>
        <p className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
          Cost tracking is currently unavailable
        </p>
        <p className="text-sm text-yellow-700 dark:text-yellow-300">
          {reason || "Prometheus is not configured. Deploy Prometheus to enable cost tracking."}
        </p>
      </div>
    </div>
  );
}
