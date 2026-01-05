"use client";

import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { calculateCost, formatCost, formatTokens, getModelPricing } from "@/lib/pricing";
import { cn } from "@/lib/utils";

interface CostBadgeProps {
  inputTokens: number;
  outputTokens: number;
  model: string;
  showTokens?: boolean;
  className?: string;
}

export function CostBadge({
  inputTokens,
  outputTokens,
  model,
  showTokens = false,
  className,
}: CostBadgeProps) {
  const cost = calculateCost(model, inputTokens, outputTokens);
  const pricing = getModelPricing(model);
  const totalTokens = inputTokens + outputTokens;

  // No usage
  if (totalTokens === 0) {
    return (
      <Badge variant="outline" className={cn("text-muted-foreground", className)}>
        No usage
      </Badge>
    );
  }

  const content = showTokens ? formatTokens(totalTokens) : formatCost(cost);

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge
            variant="secondary"
            className={cn("font-mono cursor-help", className)}
          >
            {content}
          </Badge>
        </TooltipTrigger>
        <TooltipContent side="top" className="text-xs">
          <div className="space-y-1">
            <div className="font-medium">
              {pricing?.displayName || model}
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-muted-foreground">Input:</span>
              <span>{formatTokens(inputTokens)}</span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-muted-foreground">Output:</span>
              <span>{formatTokens(outputTokens)}</span>
            </div>
            <div className="border-t pt-1 mt-1 flex justify-between gap-4 font-medium">
              <span>Est. Cost:</span>
              <span>{formatCost(cost)}</span>
            </div>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
