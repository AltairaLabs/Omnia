"use client";

import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { ProviderBindingInfo } from "@/hooks/arena";

interface ProviderBindingIndicatorProps {
  readonly bindingInfo: ProviderBindingInfo;
}

const STATUS_STYLES: Record<ProviderBindingInfo["status"], string> = {
  bound: "bg-success",
  stale: "bg-info",
  unbound: "bg-warning",
};

/**
 * Small colored dot indicator showing provider binding status.
 * Renders inline with a tooltip explaining the status.
 */
export function ProviderBindingIndicator({ bindingInfo }: ProviderBindingIndicatorProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-block h-2 w-2 rounded-full flex-shrink-0",
            STATUS_STYLES[bindingInfo.status]
          )}
        />
      </TooltipTrigger>
      <TooltipContent side="right">
        <p>{bindingInfo.message}</p>
      </TooltipContent>
    </Tooltip>
  );
}
