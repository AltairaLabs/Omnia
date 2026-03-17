/**
 * Agent quality dashboard page.
 *
 * Shows aggregate eval scores — no pass/fail concepts.
 * Single view with summary cards, score trend chart, and breakdown.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo } from "react";
import { Header } from "@/components/layout";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Activity, TrendingUp, AlertCircle, ExternalLink, TrendingDown } from "lucide-react";
import { useEvalSummary, useEvalFilter, type EvalTrendRange, type EvalScoreSummary } from "@/hooks/sessions";
import { useGrafana, buildDashboardUrl, GRAFANA_DASHBOARDS } from "@/hooks/logs";
import { Button } from "@/components/ui/button";
import { EvalScoreBreakdown } from "@/components/quality/eval-score-breakdown";
import { EvalScoreTrendChart } from "@/components/quality/eval-score-trend-chart";

const TIME_RANGE_OPTIONS: { label: string; value: EvalTrendRange }[] = [
  { label: "Last 1h", value: "1h" },
  { label: "Last 6h", value: "6h" },
  { label: "Last 24h", value: "24h" },
  { label: "Last 7d", value: "7d" },
  { label: "Last 30d", value: "30d" },
];

function StatsCardSkeleton() {
  return (
    <Card>
      <CardContent className="pt-6">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-4" />
          <Skeleton className="h-4 w-24" />
        </div>
        <Skeleton className="h-8 w-16 mt-1" />
      </CardContent>
    </Card>
  );
}

/** Summary cards: Evals count, Avg Score, Lowest Score. */
function SummaryCards({
  summaries,
  isLoading,
}: Readonly<{
  summaries: EvalScoreSummary[];
  isLoading: boolean;
}>) {
  const stats = useMemo(() => {
    if (summaries.length === 0) {
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

  if (isLoading) {
    return (
      <div className="grid grid-cols-3 gap-4">
        <StatsCardSkeleton />
        <StatsCardSkeleton />
        <StatsCardSkeleton />
      </div>
    );
  }

  return (
    <div className="grid grid-cols-3 gap-4">
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Evals</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.count}</p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Avg Score</span>
          </div>
          <p className="text-2xl font-bold mt-1">
            {stats.count > 0 ? `${(stats.avgScore * 100).toFixed(0)}%` : "-"}
          </p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <TrendingDown className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Lowest Score</span>
          </div>
          <p className="text-2xl font-bold mt-1">
            {stats.count > 0 ? `${(stats.lowestScore * 100).toFixed(0)}%` : "-"}
          </p>
          {stats.lowestName && (
            <p className="text-xs text-muted-foreground mt-0.5">{stats.lowestName}</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

/** Filter bar with time range, agent, and promptpack selectors. */
function FilterBar({
  timeRange,
  onTimeRangeChange,
  agents,
  promptpacks,
  selectedAgent,
  selectedPromptPack,
  onAgentChange,
  onPromptPackChange,
}: Readonly<{
  timeRange: EvalTrendRange;
  onTimeRangeChange: (range: EvalTrendRange) => void;
  agents: string[];
  promptpacks: string[];
  selectedAgent: string | undefined;
  selectedPromptPack: string | undefined;
  onAgentChange: (agent: string | undefined) => void;
  onPromptPackChange: (pp: string | undefined) => void;
}>) {
  return (
    <div className="flex items-center gap-3 flex-wrap">
      <Select value={timeRange} onValueChange={(v) => onTimeRangeChange(v as EvalTrendRange)}>
        <SelectTrigger className="w-[130px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {TIME_RANGE_OPTIONS.map((opt) => (
            <SelectItem key={opt.value} value={opt.value}>
              {opt.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {agents.length > 0 && (
        <Select
          value={selectedAgent ?? "__all__"}
          onValueChange={(v) => onAgentChange(v === "__all__" ? undefined : v)}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="All agents" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All agents</SelectItem>
            {agents.map((a) => (
              <SelectItem key={a} value={a}>{a}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      {promptpacks.length > 0 && (
        <Select
          value={selectedPromptPack ?? "__all__"}
          onValueChange={(v) => onPromptPackChange(v === "__all__" ? undefined : v)}
        >
          <SelectTrigger className="w-[200px]">
            <SelectValue placeholder="All prompt packs" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All prompt packs</SelectItem>
            {promptpacks.map((p) => (
              <SelectItem key={p} value={p}>{p}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}

export default function QualityPage() {
  const [activeMetric, setActiveMetric] = useState<string | undefined>();
  const [trendTimeRange, setTrendTimeRange] = useState<EvalTrendRange>("24h");

  const grafanaConfig = useGrafana();
  const grafanaUrl = buildDashboardUrl(grafanaConfig, GRAFANA_DASHBOARDS.QUALITY);

  const evalFilter = useEvalFilter();
  const { filter } = evalFilter;

  const summaryQuery = useEvalSummary(filter);
  const summaries = summaryQuery.data || [];

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Quality"
        description="Agent eval metrics and quality scores"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Top-level filter bar */}
        <div className="flex items-center justify-between gap-4">
          <FilterBar
            timeRange={trendTimeRange}
            onTimeRangeChange={setTrendTimeRange}
            agents={evalFilter.agents}
            promptpacks={evalFilter.promptpacks}
            selectedAgent={evalFilter.selectedAgent}
            selectedPromptPack={evalFilter.selectedPromptPack}
            onAgentChange={evalFilter.setAgent}
            onPromptPackChange={evalFilter.setPromptPack}
          />
          {grafanaUrl && (
            <Button variant="ghost" size="sm" asChild>
              <a href={grafanaUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4 mr-2" />
                View in Grafana
              </a>
            </Button>
          )}
        </div>

        {/* Error state */}
        {summaryQuery.error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading quality data</AlertTitle>
            <AlertDescription>
              {summaryQuery.error instanceof Error ? summaryQuery.error.message : "An unexpected error occurred"}
            </AlertDescription>
          </Alert>
        )}

        <SummaryCards summaries={summaries} isLoading={summaryQuery.isLoading} />

        <EvalScoreTrendChart
          timeRange={trendTimeRange}
          filter={filter}
        />

        <EvalScoreBreakdown
          activeMetric={activeMetric}
          onSelectMetric={setActiveMetric}
          filter={filter}
        />
      </div>
    </div>
  );
}
