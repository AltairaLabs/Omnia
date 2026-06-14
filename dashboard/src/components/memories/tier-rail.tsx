"use client";

import type { Tier } from "@/lib/memory-analytics/types";
import { TIER_COLORS, TIER_LABELS, TIER_DESCRIPTIONS } from "@/lib/memory-analytics/colors";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";

const TIERS: Tier[] = ["institutional", "agent", "user", "user_for_agent"];

interface TierRailProps {
  counts: Record<Tier, number>;
  hidden: Set<string>;
  onToggle: (tier: Tier) => void;
}

export function TierRail({ counts, hidden, onToggle }: Readonly<TierRailProps>) {
  return (
    <TooltipProvider>
      <div className="flex flex-wrap items-center gap-2" data-testid="tier-rail">
        {TIERS.map((tier) => {
          const isHidden = hidden.has(tier);
          return (
            <Tooltip key={tier}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  data-testid={`tier-chip-${tier}`}
                  aria-pressed={!isHidden}
                  onClick={() => onToggle(tier)}
                  className={cn(
                    "flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium transition-opacity",
                    isHidden && "opacity-40",
                  )}
                >
                  <span
                    className="h-2.5 w-2.5 rounded-full"
                    style={{ backgroundColor: TIER_COLORS[tier] }}
                  />
                  <span>{TIER_LABELS[tier]}</span>
                  <span className="text-muted-foreground" data-testid={`tier-count-${tier}`}>
                    {counts[tier]}
                  </span>
                </button>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-xs">
                {TIER_DESCRIPTIONS[tier]}
              </TooltipContent>
            </Tooltip>
          );
        })}
      </div>
    </TooltipProvider>
  );
}
