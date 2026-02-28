/**
 * Agent quality dashboard page.
 *
 * Shows aggregate eval pass rates, agent comparison, and recent failures.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo, useCallback } from "react";
import Link from "next/link";
import { Header } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
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
  CheckCircle2,
  XCircle,
  Activity,
} from "lucide-react";
import { useEvalSummary, useRecentEvalFailures, useEvalMetrics, type EvalTrendRange } from "@/hooks";
import { formatDistanceToNow } from "date-fns";
import type { EvalResultSummary } from "@/types/eval";
import { AssertionTypeBreakdown } from "@/components/quality/assertion-type-breakdown";
import { FailingSessionsTable } from "@/components/quality/failing-sessions-table";
import { PassRateTrendChart } from "@/components/quality/pass-rate-trend-chart";
import { AlertConfigPanel, buildAlertThresholdMap, loadAlerts, type EvalAlert } from "@/components/quality/alert-config-panel";

const PASS_THRESHOLD = 90;
const FAIL_THRESHOLD = 70;

/** Compute aggregate stats from Prometheus gauge summaries. */
function computeAggregateStats(summaries: EvalResultSummary[]) {
  const activeEvals = summaries.length;
  const overallPassRate =
    activeEvals > 0
      ? summaries.reduce((sum, s) => sum + s.passRate, 0) / activeEvals
      : 0;
  const passing = summaries.filter((s) => s.passRate >= PASS_THRESHOLD).length;
  const failing = summaries.filter((s) => s.passRate < FAIL_THRESHOLD).length;
  return { activeEvals, overallPassRate, passing, failing };
}

function getPassRateColor(rate: number): string {
  if (rate >= 90) return "text-green-600 dark:text-green-400";
  if (rate >= 70) return "text-yellow-600 dark:text-yellow-400";
  return "text-red-600 dark:text-red-400";
}

function getPassRateVariant(rate: number): "default" | "secondary" | "destructive" {
  if (rate >= 90) return "default";
  if (rate >= 70) return "secondary";
  return "destructive";
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
      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
      <TableCell><Skeleton className="h-4 w-32" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-12 ml-auto" /></TableCell>
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
      <div className="grid grid-cols-4 gap-4">
        <StatsCardSkeleton />
        <StatsCardSkeleton />
        <StatsCardSkeleton />
        <StatsCardSkeleton />
      </div>
    );
  }

  const stats = computeAggregateStats(summaries);

  return (
    <div className="grid grid-cols-4 gap-4">
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Active Evals</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.activeEvals}</p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Overall Pass Rate</span>
          </div>
          <p className={`text-2xl font-bold mt-1 ${getPassRateColor(stats.overallPassRate)}`}>
            {stats.overallPassRate.toFixed(1)}%
          </p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4 text-green-500" />
            <span className="text-sm text-muted-foreground">Passing</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.passing}</p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <XCircle className="h-4 w-4 text-red-500" />
            <span className="text-sm text-muted-foreground">Failing</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.failing}</p>
        </CardContent>
      </Card>
    </div>
  );
}

/** Per-eval pass rate table. */
function EvalPassRateTable({
  summaries,
  isLoading,
}: Readonly<{
  summaries: EvalResultSummary[];
  isLoading: boolean;
}>) {
  const sorted = useMemo(
    () => [...summaries].sort((a, b) => a.passRate - b.passRate),
    [summaries]
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Pass Rate by Eval</CardTitle>
      </CardHeader>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Eval ID</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Pass Rate</TableHead>
            <TableHead>Progress</TableHead>
            <TableHead className="text-right">Avg Score</TableHead>
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
              <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                No eval data available
              </TableCell>
            </TableRow>
          )}
          {!isLoading && sorted.map((summary) => (
            <TableRow key={summary.evalId}>
              <TableCell className="font-mono text-sm">{summary.evalId}</TableCell>
              <TableCell>
                <Badge variant="outline">{summary.evalType}</Badge>
              </TableCell>
              <TableCell>
                <Badge variant={getPassRateVariant(summary.passRate)}>
                  {summary.passRate.toFixed(1)}%
                </Badge>
              </TableCell>
              <TableCell className="w-[160px]">
                <Progress value={summary.passRate} className="h-2" />
              </TableCell>
              <TableCell className="text-right">
                {summary.avgScore === undefined ? "-" : summary.avgScore.toFixed(2)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

/** Recent eval failures table. */
function RecentFailures({
  isLoading,
  data,
  error,
}: Readonly<{
  isLoading: boolean;
  data: { evalResults: { id: string; sessionId: string; agentName: string; evalId: string; evalType: string; score?: number; createdAt: string }[]; total: number } | undefined;
  error: Error | null;
}>) {
  const failures = data?.evalResults || [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Recent Failures</CardTitle>
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
                <Link href={`/sessions/${result.sessionId}`} className="text-primary hover:underline">
                  <ExternalLink className="h-4 w-4" />
                </Link>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

export default function QualityPage() {
  const [activeMetric, setActiveMetric] = useState<string | undefined>();
  const [trendTimeRange, setTrendTimeRange] = useState<EvalTrendRange>("24h");
  const [alerts, setAlerts] = useState<EvalAlert[]>(() => loadAlerts());

  const summaryQuery = useEvalSummary();
  const failuresQuery = useRecentEvalFailures();
  const metricsQuery = useEvalMetrics();

  const summaries = summaryQuery.data || [];
  const alertThresholds = useMemo(() => buildAlertThresholdMap(alerts), [alerts]);

  const handleAlertsChange = useCallback((updated: EvalAlert[]) => {
    setAlerts(updated);
  }, []);

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Quality"
        description="Agent eval pass rates and quality metrics"
      />

      <div className="flex-1 p-6 space-y-6">
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
            <EvalPassRateTable summaries={summaries} isLoading={summaryQuery.isLoading} />
            <RecentFailures
              isLoading={failuresQuery.isLoading}
              data={failuresQuery.data}
              error={failuresQuery.error}
            />
          </TabsContent>

          <TabsContent value="assertions" className="space-y-6 mt-4">
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <AssertionTypeBreakdown
                activeMetric={activeMetric}
                onSelectMetric={setActiveMetric}
                alertThresholds={alertThresholds}
              />
              <FailingSessionsTable
                evalType={activeMetric}
              />
            </div>
            <PassRateTrendChart
              timeRange={trendTimeRange}
              onTimeRangeChange={setTrendTimeRange}
            />
            <AlertConfigPanel
              availableMetrics={metricsQuery.data}
              onAlertsChange={handleAlertsChange}
            />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
