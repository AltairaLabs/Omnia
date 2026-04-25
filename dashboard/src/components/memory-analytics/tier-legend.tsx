"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { TIERS } from "@/lib/memory-analytics/types";
import {
  TIER_COLORS,
  TIER_LABELS,
  TIER_DESCRIPTIONS,
} from "@/lib/memory-analytics/colors";

/**
 * Persistent header card explaining the three memory tiers. Doubles as the
 * canonical visual language reused by the demo video and blog.
 */
export function TierLegend() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">
          How memory is organized
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        {TIERS.map((tier) => (
          <div key={tier} className="flex items-start gap-2">
            <span
              className="mt-1 inline-block h-2.5 w-2.5 flex-shrink-0 rounded-full"
              style={{ backgroundColor: TIER_COLORS[tier] }}
              aria-hidden
            />
            <div>
              <span className="font-medium">{TIER_LABELS[tier]}</span>
              <span className="text-muted-foreground">
                {" — "}
                {TIER_DESCRIPTIONS[tier]}
              </span>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
