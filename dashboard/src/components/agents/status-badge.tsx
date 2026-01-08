"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { AgentRuntimePhase, PromptPackPhase, ToolRegistryPhase } from "@/types";

type Phase = AgentRuntimePhase | PromptPackPhase | ToolRegistryPhase;

const STYLE_GREEN = "bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/20";

const phaseStyles: Record<Phase, string> = {
  // Agent phases
  Running: STYLE_GREEN,
  Pending: "bg-yellow-500/15 text-yellow-700 dark:text-yellow-400 border-yellow-500/20",
  Failed: "bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/20",
  // PromptPack phases
  Active: STYLE_GREEN,
  Canary: "bg-violet-500/15 text-violet-700 dark:text-violet-400 border-violet-500/20",
  Superseded: "bg-gray-500/15 text-gray-700 dark:text-gray-400 border-gray-500/20",
  // ToolRegistry phases
  Ready: STYLE_GREEN,
  Degraded: "bg-orange-500/15 text-orange-700 dark:text-orange-400 border-orange-500/20",
};

interface StatusBadgeProps {
  phase?: Phase;
  className?: string;
}

export function StatusBadge({ phase, className }: Readonly<StatusBadgeProps>) {
  if (!phase) {
    return (
      <Badge variant="outline" className={cn("text-xs", className)}>
        Unknown
      </Badge>
    );
  }

  return (
    <Badge
      variant="outline"
      className={cn("text-xs", phaseStyles[phase], className)}
    >
      {phase}
    </Badge>
  );
}
