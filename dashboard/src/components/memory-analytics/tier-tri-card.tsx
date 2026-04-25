"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { TIERS, isTier, type AggregateRow } from "@/lib/memory-analytics/types";
import {
  TIER_COLORS,
  TIER_LABELS,
  TIER_DESCRIPTIONS,
} from "@/lib/memory-analytics/colors";

interface TierTriCardProps {
  rows: AggregateRow[];
  loading?: boolean;
}

interface TierStat {
  count: number;
  share: number;
}

function rowsToStats(rows: AggregateRow[]): Record<string, TierStat> {
  const counts: Record<string, number> = {
    institutional: 0,
    agent: 0,
    user: 0,
  };
  for (const r of rows) {
    if (isTier(r.key)) counts[r.key] = r.value;
  }
  const total = counts.institutional + counts.agent + counts.user;
  const out: Record<string, TierStat> = {};
  for (const tier of TIERS) {
    const count = counts[tier];
    const share = total === 0 ? 0 : (count / total) * 100;
    out[tier] = { count, share };
  }
  return out;
}

export function TierTriCard({ rows, loading }: Readonly<TierTriCardProps>) {
  if (loading) {
    return (
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {TIERS.map((tier) => (
          <Card key={tier}>
            <CardHeader>
              <Skeleton className="h-4 w-24" data-testid="tier-skeleton" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-8 w-16 mb-1" data-testid="tier-skeleton" />
              <Skeleton className="h-3 w-12" data-testid="tier-skeleton" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  const stats = rowsToStats(rows);

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
      {TIERS.map((tier) => {
        const stat = stats[tier];
        return (
          <Card key={tier} className="h-full">
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">
                {TIER_LABELS[tier]}
              </CardTitle>
              <span
                className="inline-block h-3 w-3 rounded-full"
                style={{ backgroundColor: TIER_COLORS[tier] }}
                aria-hidden
              />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {stat.count.toLocaleString()}
              </div>
              <p className="text-xs text-muted-foreground">
                {stat.share.toFixed(1)}%
              </p>
              <p className="text-xs text-muted-foreground mt-2 leading-snug">
                {TIER_DESCRIPTIONS[tier]}
              </p>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
