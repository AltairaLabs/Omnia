"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { ConsolidationStats } from "@/hooks/use-consolidation-stats";

const ACTION_LABELS: Record<string, string> = {
  create_summary: "Create summary",
  supersede: "Supersede",
  rescope: "Rescope",
  invalidate: "Invalidate",
  merge_entities: "Merge entities",
  discard: "Discard",
  rescore: "Rescore",
};

interface ConsolidationSectionProps {
  stats: ConsolidationStats | undefined;
  loading?: boolean;
}

export function ConsolidationSection({
  stats,
  loading,
}: Readonly<ConsolidationSectionProps>) {
  if (loading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Consolidation</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-8 w-24" data-testid="consolidation-skeleton" />
        </CardContent>
      </Card>
    );
  }
  if (!stats || stats.passesTotal === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Consolidation</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          No consolidation runs in the selected window.
        </CardContent>
      </Card>
    );
  }
  const entries = Object.entries(stats.actionsByType).sort(
    (a, b) => b[1] - a[1],
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle>Consolidation</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div>
            <div className="text-sm text-muted-foreground">Passes</div>
            <div className="text-2xl font-bold">{stats.passesTotal.toLocaleString()}</div>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">Actions</div>
            <div className="text-2xl font-bold">{stats.actionsTotal.toLocaleString()}</div>
          </div>
        </div>
        {entries.length > 0 && (
          <div className="mt-4">
            <div className="text-sm font-medium mb-2">By action type</div>
            <ul className="text-sm space-y-1">
              {entries.map(([action, count]) => (
                <li key={action} className="flex justify-between">
                  <span>{ACTION_LABELS[action] ?? action}</span>
                  <span className="font-mono">{count.toLocaleString()}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
