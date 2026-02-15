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
import { Progress } from "@/components/ui/progress";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  ShieldCheck,
  TrendingUp,
  Bot,
  AlertCircle,
  ExternalLink,
  CheckCircle2,
  XCircle,
  Activity,
} from "lucide-react";
import { useEvalSummary, useRecentEvalFailures, useAgents } from "@/hooks";
import { formatDistanceToNow } from "date-fns";
import type { EvalResultSummary } from "@/types/eval";

/** Time range presets for filtering. */
const TIME_RANGES: { label: string; value: string }[] = [
  { label: "All Time", value: "all" },
  { label: "Last 1h", value: "1h" },
  { label: "Last 24h", value: "24h" },
  { label: "Last 7d", value: "7d" },
  { label: "Last 30d", value: "30d" },
];

const TIME_MS: Record<string, number> = {
  "1h": 60 * 60 * 1000,
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
  "30d": 30 * 24 * 60 * 60 * 1000,
};

function getTimeRangeFrom(value: string): string | undefined {
  if (value === "all") return undefined;
  return new Date(Date.now() - (TIME_MS[value] || 0)).toISOString();
}

/** Compute aggregate stats from summaries. */
function computeAggregateStats(summaries: EvalResultSummary[]) {
  const totalEvals = summaries.reduce((sum, s) => sum + s.total, 0);
  const totalPassed = summaries.reduce((sum, s) => sum + s.passed, 0);
  const overallPassRate = totalEvals > 0 ? (totalPassed / totalEvals) * 100 : 0;
  return { totalEvals, totalPassed, overallPassRate, evalCount: summaries.length };
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
            <span className="text-sm text-muted-foreground">Total Evals Run</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.totalEvals.toLocaleString()}</p>
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
            <ShieldCheck className="h-4 w-4 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Eval Types</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.evalCount}</p>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4 text-green-500" />
            <span className="text-sm text-muted-foreground">Total Passed</span>
          </div>
          <p className="text-2xl font-bold mt-1">{stats.totalPassed.toLocaleString()}</p>
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
            <TableHead className="text-right">Total</TableHead>
            <TableHead className="text-right">Passed</TableHead>
            <TableHead className="text-right">Failed</TableHead>
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
              <TableCell colSpan={8} className="text-center py-8 text-muted-foreground">
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
              <TableCell className="text-right">{summary.total}</TableCell>
              <TableCell className="text-right text-green-600 dark:text-green-400">{summary.passed}</TableCell>
              <TableCell className="text-right text-red-600 dark:text-red-400">{summary.failed}</TableCell>
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
  const [agentFilter, setAgentFilter] = useState<string>("all");
  const [timeRange, setTimeRange] = useState<string>("7d");

  const createdAfter = getTimeRangeFrom(timeRange);
  const agentName = agentFilter === "all" ? undefined : agentFilter;

  const summaryQuery = useEvalSummary({ agentName, createdAfter });
  const failuresQuery = useRecentEvalFailures({ agentName, limit: 10 });
  const agentsQuery = useAgents();

  const agentNames = useMemo(() => {
    if (!agentsQuery.data) return [];
    return [...new Set(agentsQuery.data.map((a) => a.metadata?.name).filter(Boolean))] as string[];
  }, [agentsQuery.data]);

  const summaries = summaryQuery.data || [];

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Quality"
        description="Agent eval pass rates and quality metrics"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Filters */}
        <div className="flex items-center gap-4">
          <Select value={agentFilter} onValueChange={setAgentFilter}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Agent" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Agents</SelectItem>
              {agentNames.map((agent) => (
                <SelectItem key={agent} value={agent}>
                  {agent}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={timeRange} onValueChange={setTimeRange}>
            <SelectTrigger className="w-[150px]">
              <SelectValue placeholder="Time Range" />
            </SelectTrigger>
            <SelectContent>
              {TIME_RANGES.map((range) => (
                <SelectItem key={range.value} value={range.value}>
                  {range.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
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

        {/* Summary cards */}
        <SummaryCards summaries={summaries} isLoading={summaryQuery.isLoading} />

        {/* Eval pass rate table */}
        <EvalPassRateTable summaries={summaries} isLoading={summaryQuery.isLoading} />

        {/* Recent failures */}
        <RecentFailures
          isLoading={failuresQuery.isLoading}
          data={failuresQuery.data}
          error={failuresQuery.error}
        />
      </div>
    </div>
  );
}
