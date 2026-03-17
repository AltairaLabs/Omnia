"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaJob, useArenaJobMutations } from "@/hooks/arena";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AlertCircle,
  XCircle,
  Trash2,
  Info,
  AlertTriangle,
  CheckCircle,
  Clock,
  Users,
  Activity,
  Database,
  Gauge,
  Settings,
  BarChart3,
  Play,
  ExternalLink,
  Timer,
  RefreshCw,
  FileText,
  Cpu,
  Wrench,
  Copy,
  Zap,
  Network,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  formatDate as formatDateBase,
  getConditionIcon,
} from "@/components/arena";
import { LogViewer } from "@/components/logs";
import { QuickRunDialog, type QuickRunInitialValues } from "@/components/arena/quick-run-dialog";
import { generateName } from "@/lib/name-generator";
import type {
  ArenaJob,
  ArenaJobPhase,
  ArenaJobType,
  ArenaProviderEntry,
} from "@/types/arena";
import type { Condition } from "@/types/common";

const formatDate = (dateString?: string) => formatDateBase(dateString, true);

function getJobTypeBadge(type: ArenaJobType | undefined) {
  switch (type) {
    case "evaluation":
      return (
        <Badge variant="secondary" className="gap-1">
          <Activity className="h-3 w-3" />
          Evaluation
        </Badge>
      );
    case "loadtest":
      return (
        <Badge variant="secondary" className="gap-1 bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
          <Gauge className="h-3 w-3" />
          Load Test
        </Badge>
      );
    case "datagen":
      return (
        <Badge variant="secondary" className="gap-1 bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200">
          <Database className="h-3 w-3" />
          Data Gen
        </Badge>
      );
    default:
      return <Badge variant="outline">Unknown</Badge>;
  }
}

function getJobPhaseBadge(phase: ArenaJobPhase | undefined) {
  switch (phase) {
    case "Pending":
      return (
        <Badge variant="outline" className="gap-1">
          <Clock className="h-3 w-3" />
          Pending
        </Badge>
      );
    case "Running":
      return (
        <Badge variant="default" className="gap-1 bg-blue-500">
          <Play className="h-3 w-3" />
          Running
        </Badge>
      );
    case "Succeeded":
      return (
        <Badge variant="default" className="gap-1 bg-green-500">
          <CheckCircle className="h-3 w-3" />
          Succeeded
        </Badge>
      );
    case "Failed":
      return (
        <Badge variant="destructive" className="gap-1">
          <AlertTriangle className="h-3 w-3" />
          Failed
        </Badge>
      );
    case "Cancelled":
      return (
        <Badge variant="outline" className="gap-1 text-muted-foreground">
          <XCircle className="h-3 w-3" />
          Cancelled
        </Badge>
      );
    default:
      return <Badge variant="outline">Unknown</Badge>;
  }
}

function formatDuration(startTime?: string, completionTime?: string): string {
  if (!startTime) return "-";

  const start = new Date(startTime);
  const end = completionTime ? new Date(completionTime) : new Date();
  const durationMs = end.getTime() - start.getTime();

  const seconds = Math.floor(durationMs / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  }
  return `${seconds}s`;
}

// Component to show provider group with its entries
function ProviderGroupDisplay({
  groupName,
  entries,
}: {
  groupName: string;
  entries: ArenaProviderEntry[];
}) {
  return (
    <div className="border rounded-md p-3 space-y-2">
      <div className="flex items-center justify-between">
        <Badge variant="outline" className="font-mono">
          {groupName}
        </Badge>
        <span className="text-xs text-muted-foreground">
          {entries.length} entr{entries.length === 1 ? "y" : "ies"}
        </span>
      </div>
      <div className="flex flex-wrap gap-1">
        {entries.map((entry) => {
          const isAgent = !!entry.agentRef;
          const name = isAgent ? entry.agentRef!.name : entry.providerRef?.name ?? "unknown";
          const entryKey = isAgent ? `agent-${name}` : `provider-${name}-${entry.providerRef?.namespace ?? ""}`;
          return (
            <Badge key={entryKey} variant="secondary" className="text-xs flex items-center gap-0.5">
              {isAgent ? (
                <Network className="h-2.5 w-2.5 text-blue-500" />
              ) : (
                <Zap className="h-2.5 w-2.5 text-amber-500" />
              )}
              {name}
            </Badge>
          );
        })}
      </div>
    </div>
  );
}

function OverviewTab({ job }: Readonly<{ job: ArenaJob }>) {
  const { spec, status } = job;

  // Read from status.progress for work item tracking
  const total = status?.progress?.total ?? 0;
  const completed = status?.progress?.completed ?? 0;
  const failed = status?.progress?.failed ?? 0;
  const progressPct = total > 0 ? Math.round(((completed + failed) / total) * 100) : 0;

  // Read from status.result.summary for test results
  const resultSummary = status?.result?.summary;
  const passedItems = resultSummary ? Number.parseInt(resultSummary.passedItems || "0", 10) : 0;
  const failedItems = resultSummary ? Number.parseInt(resultSummary.failedItems || "0", 10) : 0;
  const totalItems = resultSummary ? Number.parseInt(resultSummary.totalItems || "0", 10) : 0;
  const passRate = resultSummary?.passRate;

  return (
    <div className="space-y-6">
      {/* Progress Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Progress</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="flex items-center gap-4">
              <Progress value={progressPct} className="flex-1" />
              <span className="text-sm font-medium">{progressPct}%</span>
            </div>
            {/* Work item progress */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">Work Items</p>
                <p className="text-2xl font-bold">{total}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Completed</p>
                <p className="text-2xl font-bold text-green-600">{completed}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Failed</p>
                <p className="text-2xl font-bold text-red-600">{failed}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Remaining</p>
                <p className="text-2xl font-bold text-muted-foreground">
                  {Math.max(0, total - completed - failed)}
                </p>
              </div>
            </div>
            {/* Test results summary (when available) */}
            {resultSummary && (
              <>
                <div className="border-t pt-4">
                  <p className="text-sm font-medium mb-3">Test Results</p>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div>
                      <p className="text-sm text-muted-foreground">Total Tests</p>
                      <p className="text-2xl font-bold">{totalItems}</p>
                    </div>
                    <div>
                      <p className="text-sm text-muted-foreground">Passed</p>
                      <p className="text-2xl font-bold text-green-600">{passedItems}</p>
                    </div>
                    <div>
                      <p className="text-sm text-muted-foreground">Failed</p>
                      <p className="text-2xl font-bold text-red-600">{failedItems}</p>
                    </div>
                    <div>
                      <p className="text-sm text-muted-foreground">Pass Rate</p>
                      <p className="text-2xl font-bold">{passRate ? `${passRate}%` : "-"}</p>
                    </div>
                  </div>
                </div>
              </>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Timing Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Timing</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Started</p>
              <p className="mt-1 font-medium">{formatDate(status?.startTime) || "-"}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Completed</p>
              <p className="mt-1 font-medium">{formatDate(status?.completionTime) || "-"}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Duration</p>
              <p className="mt-1 font-medium">
                {formatDuration(status?.startTime, status?.completionTime)}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Workers Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Users className="h-4 w-4" />
            Workers
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Desired</p>
              <p className="text-2xl font-bold">
                {spec?.workers?.replicas ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Active</p>
              <p className="text-2xl font-bold text-blue-600">
                {status?.activeWorkers ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Succeeded</p>
              <p className="text-2xl font-bold text-green-600">
                {status?.progress?.completed ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Failed</p>
              <p className="text-2xl font-bold text-red-600">
                {status?.progress?.failed ?? 0}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Source Reference Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Settings className="h-4 w-4" />
            Configuration
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div>
              <p className="text-sm text-muted-foreground">Source</p>
              <Link
                href={`/arena/sources/${spec?.sourceRef?.name}`}
                className="text-primary hover:underline font-medium"
              >
                {spec?.sourceRef?.name}
              </Link>
            </div>
            {spec?.arenaFile && (
              <div>
                <p className="text-sm text-muted-foreground">Arena File</p>
                <p className="mt-1 font-mono text-sm">{spec.arenaFile}</p>
              </div>
            )}
            <div>
              <p className="text-sm text-muted-foreground">Job Type</p>
              <div className="mt-1">{getJobTypeBadge(spec?.type)}</div>
            </div>
            {spec?.suspend !== undefined && (
              <div>
                <p className="text-sm text-muted-foreground">Suspended</p>
                <p className="mt-1 font-medium">{spec.suspend ? "Yes" : "No"}</p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Type-specific Config Card */}
      {spec?.type === "evaluation" && spec.evaluation && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Activity className="h-4 w-4" />
              Evaluation Settings
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">Output Formats</p>
                <div className="flex gap-1 mt-1">
                  {spec.evaluation.outputFormats?.map((fmt) => (
                    <Badge key={fmt} variant="outline" className="text-xs">
                      {fmt}
                    </Badge>
                  )) ?? "-"}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {spec?.type === "loadtest" && spec.loadTest && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Gauge className="h-4 w-4" />
              Load Test Settings
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">Ramp Up</p>
                <p className="mt-1 font-medium">
                  {spec.loadTest.rampUp ?? "30s"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Duration</p>
                <p className="mt-1 font-medium">
                  {spec.loadTest.duration ?? "-"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Target RPS</p>
                <p className="mt-1 font-medium">
                  {spec.loadTest.targetRPS ?? "-"}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {spec?.type === "datagen" && spec.dataGen && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Database className="h-4 w-4" />
              Data Generation Settings
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">Count</p>
                <p className="mt-1 font-medium">
                  {spec.dataGen.count ?? "-"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Output Format</p>
                <p className="mt-1 font-medium">
                  {spec.dataGen.format ?? "jsonl"}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Provider Groups Card */}
      {spec?.providers && Object.keys(spec.providers).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Cpu className="h-4 w-4" />
              Provider Groups
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {Object.entries(spec.providers).map(([groupName, entries]) => (
                <ProviderGroupDisplay
                  key={groupName}
                  groupName={groupName}
                  entries={entries}
                />
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Tool Registries Card */}
      {spec?.toolRegistries && spec.toolRegistries.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Wrench className="h-4 w-4" />
              Tool Registries
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-1">
              {spec.toolRegistries.map((ref) => (
                <Badge key={ref.name} variant="secondary" className="text-xs">
                  {ref.name}
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Results URL Card */}
      {status?.result?.url && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <BarChart3 className="h-4 w-4" />
              Results
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Results URL:</span>
              <a
                href={status.result.url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline flex items-center gap-1"
              >
                {status.result.url}
                <ExternalLink className="h-3 w-3" />
              </a>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Conditions Card */}
      {status?.conditions && status.conditions.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Conditions</CardTitle>
            <CardDescription>Current state and events</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {status.conditions.map((condition: Condition) => (
                <div
                  key={condition.type}
                  className="flex items-start gap-3 p-3 rounded-lg border"
                >
                  {getConditionIcon(condition.status)}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between">
                      <p className="font-medium">{condition.type}</p>
                      <span className="text-xs text-muted-foreground">
                        {formatDate(condition.lastTransitionTime)}
                      </span>
                    </div>
                    {condition.reason && (
                      <p className="text-sm text-muted-foreground">{condition.reason}</p>
                    )}
                    {condition.message && (
                      <p className="text-sm mt-1">{condition.message}</p>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

interface ResultDetailsScenario {
  name: string;
  total: number;
  passed: number;
  failed: number;
  passRate: number;
  avgDurationMs: number;
  totalTokens?: number;
  totalCost?: number;
}
interface ResultDetailsProvider {
  name: string;
  total: number;
  passed: number;
  failed: number;
  passRate: number;
  avgDurationMs: number;
  totalTokens?: number;
  totalCost?: number;
}
interface ResultDetailsAssertion {
  name: string;
  total: number;
  passed: number;
  failed: number;
  passRate: number;
  failures?: string[];
}
interface ResultDetailsError {
  message: string;
  count: number;
  workItemIds?: string[];
}
interface ResultDetails {
  scenarios?: ResultDetailsScenario[];
  providers?: ResultDetailsProvider[];
  assertions?: ResultDetailsAssertion[];
  errors?: ResultDetailsError[];
}

function parseDetails(raw: string | undefined): ResultDetails | null {
  if (!raw) return null;
  try {
    return JSON.parse(raw) as ResultDetails;
  } catch {
    return null;
  }
}

function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatCost(cost: number): string {
  return `$${cost.toFixed(4)}`;
}

function ResultsTab({ job }: Readonly<{ job: ArenaJob }>) {
  const phase = job.status?.phase;
  const resultsUrl = job.status?.result?.url;
  const resultSummary = job.status?.result?.summary;

  if (phase === "Pending" || phase === "Running") {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <RefreshCw className="h-12 w-12 mx-auto mb-4 opacity-50 animate-spin" />
        <p className="text-lg font-medium mb-1">Job is still running</p>
        <p className="text-sm">Results will be available once the job completes.</p>
      </div>
    );
  }

  if (phase === "Cancelled") {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <XCircle className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">Job was cancelled</p>
        <p className="text-sm">No results are available for cancelled jobs.</p>
      </div>
    );
  }

  const totalItems = resultSummary ? Number.parseInt(resultSummary.totalItems || "0", 10) : 0;
  const passedItems = resultSummary ? Number.parseInt(resultSummary.passedItems || "0", 10) : 0;
  const failedItems = resultSummary ? Number.parseInt(resultSummary.failedItems || "0", 10) : 0;
  const passRate = resultSummary?.passRate;
  const avgDurationMs = resultSummary?.avgDurationMs;
  const totalTokens = resultSummary?.totalTokens;
  const totalCost = resultSummary?.totalCost;
  const details = parseDetails(resultSummary?.details);

  if (!resultsUrl && !resultSummary) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <BarChart3 className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No results available</p>
        <p className="text-sm">
          Results may still be processing or the job did not produce output.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Summary stats */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Summary</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Total Tests</p>
              <p className="text-2xl font-bold">{totalItems}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Passed</p>
              <p className="text-2xl font-bold text-green-600">{passedItems}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Failed</p>
              <p className="text-2xl font-bold text-red-600">{failedItems}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Pass Rate</p>
              <p className="text-2xl font-bold">{passRate ? `${passRate}%` : "-"}</p>
            </div>
          </div>
          {(avgDurationMs || totalTokens || totalCost) && (
            <div className="flex gap-6 pt-4 mt-4 border-t text-sm">
              {avgDurationMs && (
                <div>
                  <span className="text-muted-foreground">Avg Duration: </span>
                  <span className="font-medium">{avgDurationMs}ms</span>
                </div>
              )}
              {totalTokens && (
                <div>
                  <span className="text-muted-foreground">Total Tokens: </span>
                  <span className="font-medium">{Number(totalTokens).toLocaleString()}</span>
                </div>
              )}
              {totalCost && (
                <div>
                  <span className="text-muted-foreground">Total Cost: </span>
                  <span className="font-medium">${totalCost}</span>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Scenario breakdown */}
      {details?.scenarios && details.scenarios.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Scenarios</CardTitle>
            <CardDescription>Per-scenario breakdown</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 font-medium">Scenario</th>
                    <th className="pb-2 font-medium text-right">Total</th>
                    <th className="pb-2 font-medium text-right">Passed</th>
                    <th className="pb-2 font-medium text-right">Failed</th>
                    <th className="pb-2 font-medium text-right">Pass Rate</th>
                    <th className="pb-2 font-medium text-right">Avg Duration</th>
                    <th className="pb-2 font-medium text-right">Tokens</th>
                    <th className="pb-2 font-medium text-right">Cost</th>
                  </tr>
                </thead>
                <tbody>
                  {details.scenarios.map((s) => (
                    <tr key={s.name} className="border-b last:border-0">
                      <td className="py-2 font-medium">{s.name}</td>
                      <td className="py-2 text-right">{s.total}</td>
                      <td className="py-2 text-right text-green-600">{s.passed}</td>
                      <td className="py-2 text-right text-red-600">{s.failed > 0 ? s.failed : "-"}</td>
                      <td className="py-2 text-right">{s.passRate.toFixed(1)}%</td>
                      <td className="py-2 text-right">{formatDurationMs(s.avgDurationMs)}</td>
                      <td className="py-2 text-right">{s.totalTokens ? s.totalTokens.toLocaleString() : "-"}</td>
                      <td className="py-2 text-right">{s.totalCost ? formatCost(s.totalCost) : "-"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Provider breakdown (only show if more than one provider) */}
      {details?.providers && details.providers.length > 1 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Providers</CardTitle>
            <CardDescription>Per-provider breakdown</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 font-medium">Provider</th>
                    <th className="pb-2 font-medium text-right">Total</th>
                    <th className="pb-2 font-medium text-right">Passed</th>
                    <th className="pb-2 font-medium text-right">Failed</th>
                    <th className="pb-2 font-medium text-right">Pass Rate</th>
                    <th className="pb-2 font-medium text-right">Avg Duration</th>
                  </tr>
                </thead>
                <tbody>
                  {details.providers.map((p) => (
                    <tr key={p.name} className="border-b last:border-0">
                      <td className="py-2 font-medium">{p.name}</td>
                      <td className="py-2 text-right">{p.total}</td>
                      <td className="py-2 text-right text-green-600">{p.passed}</td>
                      <td className="py-2 text-right text-red-600">{p.failed > 0 ? p.failed : "-"}</td>
                      <td className="py-2 text-right">{p.passRate.toFixed(1)}%</td>
                      <td className="py-2 text-right">{formatDurationMs(p.avgDurationMs)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Assertions */}
      {details?.assertions && details.assertions.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Assertions</CardTitle>
            <CardDescription>
              {details.assertions.filter((a) => a.failed === 0).length} of {details.assertions.length} fully passing
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {details.assertions.map((a) => (
                <div key={a.name} className="flex items-start gap-2 text-sm">
                  {a.failed === 0 ? (
                    <CheckCircle className="h-4 w-4 text-green-500 mt-0.5 shrink-0" />
                  ) : (
                    <XCircle className="h-4 w-4 text-red-500 mt-0.5 shrink-0" />
                  )}
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{a.name}</span>
                      <span className="text-muted-foreground">
                        {a.passed}/{a.total} passed ({a.passRate.toFixed(0)}%)
                      </span>
                    </div>
                    {a.failures && a.failures.length > 0 && (
                      <ul className="mt-1 space-y-0.5 text-muted-foreground">
                        {a.failures.map((msg) => (
                          <li key={msg} className="text-red-600">
                            {msg}
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Errors */}
      {details?.errors && details.errors.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base text-red-600">Errors</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {details.errors.map((e) => (
                <div key={e.message} className="text-sm">
                  <div className="flex items-center gap-2">
                    <AlertCircle className="h-4 w-4 text-red-500 shrink-0" />
                    <span className="font-medium">{e.message}</span>
                    {e.count > 1 && (
                      <Badge variant="secondary" className="text-xs">{e.count}x</Badge>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* External results link */}
      {resultsUrl && (
        <div>
          <a
            href={resultsUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2"
          >
            <Button>
              <ExternalLink className="h-4 w-4 mr-2" />
              View Full Results
            </Button>
          </a>
        </div>
      )}
    </div>
  );
}

function buildCloneInitialValues(job: ArenaJob): QuickRunInitialValues {
  return {
    includePatterns: job.spec?.scenarios?.include?.join(", ") ?? "",
    excludePatterns: job.spec?.scenarios?.exclude?.join(", ") ?? "",
    verbose: job.spec?.verbose ?? false,
  };
}

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header title="Job Details" description="Loading job information..." />
      <div className="flex-1 p-6 space-y-6 overflow-auto">
        <Skeleton className="h-8 w-64" />
        <div className="flex gap-2">
          <Skeleton className="h-10 w-24" />
          <Skeleton className="h-10 w-24" />
        </div>
        <Skeleton className="h-[200px]" />
        <Skeleton className="h-[150px]" />
      </div>
    </div>
  );
}

export default function ArenaJobDetailPage() {
  const params = useParams();
  const router = useRouter();
  const jobName = params.name as string;

  const { job, loading, error, refetch } = useArenaJob(jobName);
  const { cancelJob, deleteJob, createJob } = useArenaJobMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [cancelling, setCancelling] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [cloning, setCloning] = useState(false);
  const [cloneDialogOpen, setCloneDialogOpen] = useState(false);

  const isRunning = job?.status?.phase === "Running" || job?.status?.phase === "Pending";
  const isFinished = job?.status?.phase === "Succeeded" || job?.status?.phase === "Failed" || job?.status?.phase === "Cancelled";
  const projectId = job?.metadata?.labels?.["arena.omnia.altairalabs.ai/project-id"];

  const handleCancel = async () => {
    if (!confirm(`Are you sure you want to cancel job "${jobName}"?`)) {
      return;
    }
    try {
      setCancelling(true);
      await cancelJob(jobName);
      refetch();
    } catch {
      // Error is handled by the hook
    } finally {
      setCancelling(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm(`Are you sure you want to delete job "${jobName}"?`)) {
      return;
    }
    try {
      setDeleting(true);
      await deleteJob(jobName);
      router.push("/arena/jobs");
    } catch {
      setDeleting(false);
      // Error is handled by the hook
    }
  };

  const handleClone = async () => {
    if (!job?.spec) return;
    try {
      setCloning(true);
      const cloneName = generateName();
      const cloned = await createJob(cloneName, job.spec);
      router.push(`/arena/jobs/${cloned.metadata.name}`);
    } catch {
      setCloning(false);
    }
  };

  if (loading) {
    return <LoadingSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Job Details" description="Error loading job" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading job</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  if (!job) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Job Details" description="Job not found" />
        <div className="flex-1 p-6">
          <Alert>
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>Job not found</AlertTitle>
            <AlertDescription>
              The job &quot;{jobName}&quot; could not be found.
            </AlertDescription>
          </Alert>
          <Link href="/arena/jobs">
            <Button variant="outline" className="mt-4">
              Back to Jobs
            </Button>
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header
        title={jobName}
        description="Arena job execution"
      />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Breadcrumb and Actions */}
        <div className="flex items-center justify-between">
          <ArenaBreadcrumb
            items={[
              { label: "Jobs", href: "/arena/jobs" },
              { label: jobName },
            ]}
          />
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => refetch()}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Refresh
            </Button>
            {isRunning && canEdit && (
              <Button
                variant="outline"
                onClick={handleCancel}
                disabled={cancelling}
              >
                <XCircle className="h-4 w-4 mr-2" />
                Cancel
              </Button>
            )}
            {isFinished && canEdit && (
              <Button
                variant="outline"
                onClick={projectId ? () => setCloneDialogOpen(true) : handleClone}
                disabled={cloning}
              >
                <Copy className="h-4 w-4 mr-2" />
                Clone
              </Button>
            )}
            {isFinished && canEdit && (
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleting}
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </Button>
            )}
          </div>
        </div>

        {/* Status Summary */}
        <div className="flex items-center gap-4 flex-wrap">
          {getJobPhaseBadge(job.status?.phase)}
          {getJobTypeBadge(job.spec?.type)}
          <Badge variant="outline" className="gap-1">
            <Users className="h-3 w-3" />
            {job.status?.activeWorkers ?? 0} / {job.spec?.workers?.replicas ?? 0} workers
          </Badge>
          <Badge variant="outline" className="gap-1">
            <Timer className="h-3 w-3" />
            {formatDuration(job.status?.startTime, job.status?.completionTime)}
          </Badge>
          <Link
            href={`/arena/sources/${job.spec?.sourceRef?.name}`}
            className="text-sm text-muted-foreground hover:underline"
          >
            Source: {job.spec?.sourceRef?.name}
          </Link>
        </div>

        {/* Clone Dialog */}
        {projectId && (
          <QuickRunDialog
            open={cloneDialogOpen}
            onOpenChange={setCloneDialogOpen}
            projectId={projectId}
            initialValues={buildCloneInitialValues(job)}
            onJobCreated={(newJobName) => router.push(`/arena/jobs/${newJobName}`)}
          />
        )}

        {/* Tabs */}
        <Tabs defaultValue="overview" className="space-y-4">
          <TabsList>
            <TabsTrigger value="overview">
              <Info className="h-4 w-4 mr-2" />
              Overview
            </TabsTrigger>
            <TabsTrigger value="logs">
              <FileText className="h-4 w-4 mr-2" />
              Logs
            </TabsTrigger>
            <TabsTrigger value="results">
              <BarChart3 className="h-4 w-4 mr-2" />
              Results
            </TabsTrigger>
          </TabsList>

          <TabsContent value="overview">
            <OverviewTab job={job} />
          </TabsContent>

          <TabsContent value="logs">
            <LogViewer
              jobName={jobName}
              jobPhase={job?.status?.phase}
              workspace={currentWorkspace?.name || ""}
              resourceName={jobName}
              containers={["worker"]}
              showGrafanaLinks={true}
            />
          </TabsContent>

          <TabsContent value="results">
            <ResultsTab job={job} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
