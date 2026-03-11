/**
 * Agent quality dashboard page.
 *
 * Shows aggregate eval pass rates, agent comparison, and recent failures.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { Header } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  TrendingUp,
  Bot,
  AlertCircle,
  ExternalLink,
  XCircle,
  Activity,
} from "lucide-react";
import { useEvalSummary, useRecentEvalFailures, useEvalFilter, type EvalTrendRange, useGrafana, buildDashboardUrl, GRAFANA_DASHBOARDS } from "@/hooks";
import { Button } from "@/components/ui/button";
import { formatDistanceToNow } from "date-fns";
import type { EvalResultSummary } from "@/types/eval";
import { AssertionTypeBreakdown } from "@/components/quality/assertion-type-breakdown";
import { FailingSessionsTable } from "@/components/quality/failing-sessions-table";
import { PassRateTrendChart } from "@/components/quality/pass-rate-trend-chart";

const TIME_RANGE_OPTIONS: { label: string; value: EvalTrendRange }[] = [
  { label: "Last 1h", value: "1h" },
  { label: "Last 6h", value: "6h" },
  { label: "Last 24h", value: "24h" },
  { label: "Last 7d", value: "7d" },
  { label: "Last 30d", value: "30d" },
];

/** Format a metric value for display based on its type. */
function formatValue(summary: EvalResultSummary): string {
  if (summary.metricType === "counter") return summary.total.toLocaleString();
  if (summary.metricType === "histogram") return `${(summary.avgScore ?? 0).toFixed(3)}s`;
  if (summary.metricType === "boolean") return (summary.avgScore ?? 0) >= 0.5 ? "true" : "false";
  return (summary.avgScore ?? 0).toFixed(3);
}

/** Get a display label for the metric type. */
function metricTypeLabel(summary: EvalResultSummary): string {
  return summary.metricType ?? "gauge";
}

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

function EvalTableSkeleton() {
  return (
    <TableRow>
      <TableCell><Skeleton className="h-4 w-24" /></TableCell>
      <TableCell><Skeleton className="h-4 w-20" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-16 ml-auto" /></TableCell>
    </TableRow>
  );
}

/** Summary cards showing overall stats. */
function SummaryCards({
  summaries,
  isLoading,
}: Readonly<{
  summaries: EvalResultSummary[];
  isLoading: boolean;
}>) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-3 gap-4">
        <StatsCardSkeleton />
        <StatsCardSkeleton />
        <StatsCardSkeleton />
      </div>
    );
  }

  const gaugeCount = summaries.filter((s) => !s.metricType || s.metricType === "gauge").length;
  const booleanCount = summaries.filter((s) => s.metricType === "boolean").length;
  const otherCount = summaries.length - gaugeCount - booleanCount;

  return (
    <div className="grid grid-cols-3 gap-4">
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Active Evals</span>
          </div>
          <p className="text-2xl font-bold mt-1">{summaries.length}</p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Score Metrics</span>
          </div>
          <p className="text-2xl font-bold mt-1">{gaugeCount + booleanCount}</p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Counter / Histogram</span>
          </div>
          <p className="text-2xl font-bold mt-1">{otherCount}</p>
        </CardContent>
      </Card>
    </div>
  );
}

/** Per-eval metrics table showing current values by type. */
function EvalMetricsTable({
  summaries,
  isLoading,
}: Readonly<{
  summaries: EvalResultSummary[];
  isLoading: boolean;
}>) {
  const sorted = useMemo(
    () => [...summaries].sort((a, b) => a.evalId.localeCompare(b.evalId)),
    [summaries]
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Eval Metrics</CardTitle>
      </CardHeader>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Eval</TableHead>
            <TableHead>Metric Type</TableHead>
            <TableHead className="text-right">Current Value</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading && (
            <>
              <EvalTableSkeleton />
              <EvalTableSkeleton />
              <EvalTableSkeleton />
            </>
          )}
          {!isLoading && sorted.length === 0 && (
            <TableRow>
              <TableCell colSpan={3} className="text-center py-8 text-muted-foreground">
                No eval metrics found. Eval metrics are emitted by the runtime when evals execute.
              </TableCell>
            </TableRow>
          )}
          {!isLoading && sorted.map((summary) => (
            <TableRow key={summary.evalId}>
              <TableCell className="font-mono text-sm">{summary.evalId}</TableCell>
              <TableCell>
                <Badge variant="outline">{metricTypeLabel(summary)}</Badge>
              </TableCell>
              <TableCell className="text-right font-mono">
                {formatValue(summary)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

const FAILURES_PAGE_SIZE = 10;

/** Recent eval failures table with pagination. */
function RecentFailures() {
  const [page, setPage] = useState(0);
  const offset = page * FAILURES_PAGE_SIZE;
  const { data, isLoading, error } = useRecentEvalFailures({
    limit: FAILURES_PAGE_SIZE,
    offset,
  });

  const failures = data?.results || [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / FAILURES_PAGE_SIZE));
  const showFrom = total > 0 ? offset + 1 : 0;
  const showTo = Math.min(offset + FAILURES_PAGE_SIZE, total);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-base">Recent Failures</CardTitle>
        {total > 0 && (
          <span className="text-sm text-muted-foreground">
            {showFrom}&ndash;{showTo} of {total}
          </span>
        )}
      </CardHeader>
      {error && (
        <CardContent>
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>Failed to load recent failures</AlertDescription>
          </Alert>
        </CardContent>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Eval ID</TableHead>
            <TableHead>Agent</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Score</TableHead>
            <TableHead>Time</TableHead>
            <TableHead></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading && (
            <>
              <EvalTableSkeleton />
              <EvalTableSkeleton />
              <EvalTableSkeleton />
            </>
          )}
          {!isLoading && failures.length === 0 && (
            <TableRow>
              <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                No recent failures
              </TableCell>
            </TableRow>
          )}
          {!isLoading && failures.map((result) => (
            <TableRow key={result.id}>
              <TableCell className="font-mono text-sm">
                <div className="flex items-center gap-2">
                  <XCircle className="h-4 w-4 text-red-500" />
                  {result.evalId}
                </div>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Bot className="h-4 w-4 text-muted-foreground" />
                  {result.agentName}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline">{result.evalType}</Badge>
              </TableCell>
              <TableCell>{result.score === undefined ? "-" : result.score.toFixed(2)}</TableCell>
              <TableCell className="text-muted-foreground">
                {formatDistanceToNow(new Date(result.createdAt), { addSuffix: true })}
              </TableCell>
              <TableCell>
                <Link href={`/sessions/${result.sessionId}?tab=evals`} className="text-primary hover:underline">
                  <ExternalLink className="h-4 w-4" />
                </Link>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {totalPages > 1 && (
        <div className="flex items-center justify-end gap-2 p-4 pt-0">
          <Button
            variant="outline"
            size="sm"
            disabled={page === 0}
            onClick={() => setPage((p) => p - 1)}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {page + 1} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!data?.hasMore}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </Card>
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

        <Tabs defaultValue="overview">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="assertions">Assertions</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-6 mt-4">
            <SummaryCards summaries={summaries} isLoading={summaryQuery.isLoading} />
            <EvalMetricsTable summaries={summaries} isLoading={summaryQuery.isLoading} />
            <RecentFailures />
          </TabsContent>

          <TabsContent value="assertions" className="space-y-6 mt-4">
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <AssertionTypeBreakdown
                activeMetric={activeMetric}
                onSelectMetric={setActiveMetric}
                filter={filter}
              />
              <FailingSessionsTable
                evalType={activeMetric}
              />
            </div>
            <PassRateTrendChart
              timeRange={trendTimeRange}
              filter={filter}
            />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
