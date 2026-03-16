/**
 * Per-agent quality tab showing eval scores scoped to a single agent.
 *
 * Reuses the shared quality components (breakdown, trends)
 * with an agent-scoped EvalFilter so only metrics for this agent are shown.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo } from "react";
import { Activity, TrendingUp, TrendingDown } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { EvalScoreBreakdown } from "@/components/quality/eval-score-breakdown";
import { EvalScoreTrendChart } from "@/components/quality/eval-score-trend-chart";
import { useEvalSummary, type EvalTrendRange } from "@/hooks/sessions";
import type { EvalFilter } from "@/lib/prometheus-queries";

const TIME_RANGES: { label: string; value: EvalTrendRange }[] = [
  { label: "1h", value: "1h" },
  { label: "6h", value: "6h" },
  { label: "24h", value: "24h" },
  { label: "7d", value: "7d" },
  { label: "30d", value: "30d" },
];

interface AgentQualityTabProps {
  agentName: string;
}

export function AgentQualityTab({ agentName }: Readonly<AgentQualityTabProps>) {
  const [activeMetric, setActiveMetric] = useState<string>();
  const [timeRange, setTimeRange] = useState<EvalTrendRange>("24h");

  const filter: EvalFilter = useMemo(() => ({ agent: agentName }), [agentName]);
  const { data: summaries, isLoading } = useEvalSummary(filter);

  const stats = useMemo(() => {
    if (!summaries || summaries.length === 0) {
      return { count: 0, avgScore: 0, lowestScore: 0, lowestName: "" };
    }
    const gauges = summaries.filter((s) => s.metricType === "gauge");
    const avgScore = gauges.length > 0
      ? gauges.reduce((sum, s) => sum + s.score, 0) / gauges.length
      : 0;
    const lowest = gauges.length > 0
      ? gauges.reduce((min, s) => (s.score < min.score ? s : min), gauges[0])
      : null;
    return {
      count: summaries.length,
      avgScore,
      lowestScore: lowest?.score ?? 0,
      lowestName: lowest?.evalId ?? "",
    };
  }, [summaries]);

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-4">
        <SummaryCard
          title="Evals"
          value={stats.count}
          icon={<Activity className="h-4 w-4 text-muted-foreground" />}
          isLoading={isLoading}
        />
        <SummaryCard
          title="Avg Score"
          value={stats.count > 0 ? `${(stats.avgScore * 100).toFixed(0)}%` : "-"}
          icon={<TrendingUp className="h-4 w-4 text-muted-foreground" />}
          isLoading={isLoading}
        />
        <SummaryCard
          title="Lowest Score"
          value={stats.count > 0 ? `${(stats.lowestScore * 100).toFixed(0)}%` : "-"}
          subtitle={stats.lowestName}
          icon={<TrendingDown className="h-4 w-4 text-muted-foreground" />}
          isLoading={isLoading}
        />
      </div>

      {/* Time range selector + trend chart */}
      <div className="flex items-center gap-2">
        {TIME_RANGES.map((r) => (
          <button
            key={r.value}
            onClick={() => setTimeRange(r.value)}
            className={`px-3 py-1 text-sm rounded-md transition-colors ${
              timeRange === r.value
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:bg-muted/80"
            }`}
          >
            {r.label}
          </button>
        ))}
      </div>

      <EvalScoreTrendChart timeRange={timeRange} filter={filter} height={300} />

      <EvalScoreBreakdown
        activeMetric={activeMetric}
        onSelectMetric={setActiveMetric}
        filter={filter}
      />
    </div>
  );
}

function SummaryCard({
  title,
  value,
  subtitle,
  icon,
  isLoading,
}: Readonly<{
  title: string;
  value: string | number;
  subtitle?: string;
  icon: React.ReactNode;
  isLoading: boolean;
}>) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-7 w-16" />
        ) : (
          <>
            <div className="text-2xl font-bold">{value}</div>
            {subtitle && (
              <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
