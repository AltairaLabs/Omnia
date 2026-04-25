"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { Tier } from "@/lib/memory-analytics/types";
import { TIER_COLORS, TIER_LABELS } from "@/lib/memory-analytics/colors";

interface TierBadgeProps {
  tier: Tier | undefined;
  className?: string;
}

/**
 * Visual indicator of which memory tier an entry belongs to. Returns null if
 * `tier` is undefined so the caller can render `<TierBadge tier={memory.tier} />`
 * unconditionally without worrying about legacy responses.
 */
export function TierBadge({ tier, className }: Readonly<TierBadgeProps>) {
  if (!tier) return null;
  return (
    <Badge
      variant="outline"
      className={cn("font-normal", className)}
      style={{
        borderColor: TIER_COLORS[tier],
        color: TIER_COLORS[tier],
      }}
    >
      {TIER_LABELS[tier]}
    </Badge>
  );
}
