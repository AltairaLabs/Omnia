"use client";

import { ReactNode } from "react";
import { Lock } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useReadOnly } from "@/hooks";
import { cn } from "@/lib/utils";

interface ReadOnlyTooltipProps {
  children: ReactNode;
  /** Optional override for the read-only message */
  message?: string;
  /** Additional className for the wrapper */
  className?: string;
}

/**
 * Wraps children with a tooltip explaining read-only mode.
 * When not in read-only mode, renders children directly.
 * When in read-only mode, wraps children in a tooltip and adds visual indicator.
 */
export function ReadOnlyTooltip({
  children,
  message,
  className,
}: ReadOnlyTooltipProps) {
  const { isReadOnly, message: defaultMessage } = useReadOnly();

  if (!isReadOnly) {
    return <>{children}</>;
  }

  const displayMessage = message || defaultMessage;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className={cn("relative inline-flex", className)}>
            <div className="opacity-50 cursor-not-allowed pointer-events-none">
              {children}
            </div>
            <Lock className="absolute -top-1 -right-1 h-3 w-3 text-muted-foreground" />
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs">
          <p className="text-sm">{displayMessage}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

/**
 * A simpler version that just disables the element without the lock icon.
 * Useful for inline controls where the lock icon would be too prominent.
 */
export function ReadOnlyWrapper({
  children,
  message,
  className,
}: ReadOnlyTooltipProps) {
  const { isReadOnly, message: defaultMessage } = useReadOnly();

  if (!isReadOnly) {
    return <>{children}</>;
  }

  const displayMessage = message || defaultMessage;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            className={cn(
              "opacity-50 cursor-not-allowed pointer-events-none",
              className
            )}
          >
            {children}
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs">
          <p className="text-sm">{displayMessage}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
