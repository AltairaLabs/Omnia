/**
 * Per-agent quality tab showing eval metrics scoped to a single agent.
 *
 * Reuses the shared quality components (breakdown, trends, failing sessions)
 * with an agent-scoped EvalFilter so only metrics for this agent are shown.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo } from "react";
import { CheckCircle, XCircle, Activity, TrendingUp } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { AssertionTypeBreakdown } from "@/components/quality/assertion-type-breakdown";
import { PassRateTrendChart } from "@/components/quality/pass-rate-trend-chart";
import { FailingSessionsTable } from "@/components/quality/failing-sessions-table";
import { useEvalSummary, type EvalTrendRange } from "@/hooks";
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
      return { total: 0, passing: 0, failing: 0, avgPassRate: 0 };
    }
    const gauges = summaries.filter((s) => s.metricType === "gauge");
    const passing = gauges.filter((s) => s.passRate >= 90).length;
    const failing = gauges.filter((s) => s.passRate < 70).length;
    const avgPassRate =
      gauges.length > 0
        ? gauges.reduce((sum, s) => sum + s.passRate, 0) / gauges.length
        : 0;
    return { total: summaries.length, passing, failing, avgPassRate };
  }, [summaries]);

  return (
    <div className="space-y-4">
      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <SummaryCard
          title="Active Evals"
          value={stats.total}
          icon={<Activity className="h-4 w-4 text-muted-foreground" />}
          isLoading={isLoading}
        />
        <SummaryCard
          title="Avg Pass Rate"
          value={`${stats.avgPassRate.toFixed(1)}%`}
          icon={<TrendingUp className="h-4 w-4 text-muted-foreground" />}
          isLoading={isLoading}
        />
        <SummaryCard
          title="Passing"
          value={stats.passing}
          icon={<CheckCircle className="h-4 w-4 text-green-500" />}
          isLoading={isLoading}
        />
        <SummaryCard
          title="Failing"
          value={stats.failing}
          icon={<XCircle className="h-4 w-4 text-red-500" />}
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

      <PassRateTrendChart timeRange={timeRange} filter={filter} height={300} />

      {/* Metrics breakdown + failing sessions */}
      <div className="grid md:grid-cols-2 gap-4">
        <AssertionTypeBreakdown
          activeMetric={activeMetric}
          onSelectMetric={setActiveMetric}
          filter={filter}
        />
        <FailingSessionsTable
          evalType={activeMetric}
          agentName={agentName}
        />
      </div>
    </div>
  );
}

function SummaryCard({
  title,
  value,
  icon,
  isLoading,
}: Readonly<{
  title: string;
  value: string | number;
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
          <div className="text-2xl font-bold">{value}</div>
        )}
      </CardContent>
    </Card>
  );
}
