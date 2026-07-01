"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { getStatusClasses, type StatusKind } from "@/lib/colors/status";
import type { AgentRuntimePhase, PromptPackPhase, ToolRegistryPhase } from "@/types";

type Phase = AgentRuntimePhase | PromptPackPhase | ToolRegistryPhase;

// Map each resource phase onto a semantic status kind so badge coloring is
// token-driven and re-themes with a white-label brand.
const phaseKind: Record<Phase, StatusKind> = {
  // Agent phases
  Running: "success",
  Pending: "warning",
  Failed: "error",
  // PromptPack phases
  Active: "success",
  Superseded: "neutral",
  // ToolRegistry phases
  Ready: "success",
  Degraded: "warning",
};

function phaseClasses(phase: Phase): string {
  const s = getStatusClasses(phaseKind[phase]);
  return cn(s.bg, s.text, s.border);
}

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
      className={cn("text-xs", phaseClasses(phase), className)}
      data-testid="status-badge"
    >
      {phase}
    </Badge>
  );
}
