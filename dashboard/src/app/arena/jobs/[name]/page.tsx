"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaJob, useArenaJobMutations } from "@/hooks/use-arena-jobs";
import { useProviderPreview, useToolRegistryPreview } from "@/hooks";
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
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  formatDate as formatDateBase,
  getConditionIcon,
} from "@/components/arena";
import { LogViewer } from "@/components/logs";
import { QuickRunDialog, type QuickRunInitialValues } from "@/components/arena/quick-run-dialog";
import type {
  ArenaJob,
  ArenaJobPhase,
  ArenaJobType,
  ProviderGroupSelector,
  ToolRegistrySelector,
} from "@/types/arena";
import type { LabelSelectorValue } from "@/components/ui/k8s-label-selector";
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

// Helper to display label selector details
function LabelSelectorDisplay({ selector }: { selector?: LabelSelectorValue }) {
  if (!selector) return <span className="text-muted-foreground">-</span>;

  const hasMatchLabels = selector.matchLabels && Object.keys(selector.matchLabels).length > 0;
  const hasMatchExpressions = selector.matchExpressions && selector.matchExpressions.length > 0;

  if (!hasMatchLabels && !hasMatchExpressions) {
    return <span className="text-muted-foreground italic">All (empty selector)</span>;
  }

  return (
    <div className="space-y-1">
      {hasMatchLabels && (
        <div className="flex flex-wrap gap-1">
          {Object.entries(selector.matchLabels!).map(([key, value]) => (
            <Badge key={key} variant="secondary" className="font-mono text-xs">
              {key}={value}
            </Badge>
          ))}
        </div>
      )}
      {hasMatchExpressions && (
        <div className="space-y-1">
          {selector.matchExpressions!.map((expr) => (
            <div key={`${expr.key}-${expr.operator}-${expr.values?.join(",") ?? ""}`} className="flex items-center gap-1 text-xs">
              <Badge variant="outline" className="font-mono">
                {expr.key}
              </Badge>
              <span className="text-muted-foreground">{expr.operator}</span>
              {expr.values && expr.values.length > 0 && (
                <span className="font-mono">
                  [{expr.values.join(", ")}]
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// Component to show provider group override with matching providers
function ProviderGroupOverrideDisplay({
  groupName,
  selector,
}: {
  groupName: string;
  selector: ProviderGroupSelector;
}) {
  const labelSelector: LabelSelectorValue = selector.selector || {};
  const { matchingProviders, matchCount, isLoading } = useProviderPreview(labelSelector);

  return (
    <div className="border rounded-md p-3 space-y-2">
      <div className="flex items-center justify-between">
        <Badge variant="outline" className="font-mono">
          {groupName}
        </Badge>
        <span className="text-xs text-muted-foreground">
          {isLoading ? "Loading..." : `${matchCount} provider(s) match`}
        </span>
      </div>
      <div className="text-sm">
        <p className="text-xs text-muted-foreground mb-1">Selector:</p>
        <LabelSelectorDisplay selector={labelSelector} />
      </div>
      {!isLoading && matchingProviders.length > 0 && (
        <div>
          <p className="text-xs text-muted-foreground mb-1">Matching Providers:</p>
          <div className="flex flex-wrap gap-1">
            {matchingProviders.slice(0, 10).map((provider) => (
              <Badge key={provider.metadata.name} variant="secondary" className="text-xs">
                {provider.metadata.name}
              </Badge>
            ))}
            {matchingProviders.length > 10 && (
              <Badge variant="outline" className="text-xs">
                +{matchingProviders.length - 10} more
              </Badge>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// Component to show tool registry override with matching registries
function ToolRegistryOverrideDisplay({
  selector,
}: {
  selector: ToolRegistrySelector;
}) {
  const labelSelector: LabelSelectorValue = selector.selector || {};
  const { matchingRegistries, matchCount, totalToolsCount, isLoading } =
    useToolRegistryPreview(labelSelector);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Selector</span>
        <span className="text-xs text-muted-foreground">
          {isLoading
            ? "Loading..."
            : `${matchCount} registry(s), ${totalToolsCount} tools`}
        </span>
      </div>
      <LabelSelectorDisplay selector={labelSelector} />
      {!isLoading && matchingRegistries.length > 0 && (
        <div>
          <p className="text-xs text-muted-foreground mb-1">Matching Registries:</p>
          <div className="flex flex-wrap gap-1">
            {matchingRegistries.map((registry) => (
              <Badge key={registry.metadata.name} variant="secondary" className="text-xs">
                {registry.metadata.name}
                {registry.status?.discoveredToolsCount != null && (
                  <span className="ml-1 text-muted-foreground">
                    ({registry.status.discoveredToolsCount})
                  </span>
                )}
              </Badge>
            ))}
          </div>
        </div>
      )}
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
            <div>
              <p className="text-sm text-muted-foreground">Timeout</p>
              <p className="mt-1 font-medium">{spec?.timeout || "30m"}</p>
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
                {status?.workers?.desired ?? spec?.workers?.replicas ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Active</p>
              <p className="text-2xl font-bold text-blue-600">
                {status?.workers?.active ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Succeeded</p>
              <p className="text-2xl font-bold text-green-600">
                {status?.workers?.succeeded ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Failed</p>
              <p className="text-2xl font-bold text-red-600">
                {status?.workers?.failed ?? 0}
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
                <p className="text-sm text-muted-foreground">Passing Threshold</p>
                <p className="mt-1 font-medium">
                  {spec.evaluation.passingThreshold == null
                    ? "-"
                    : `${(spec.evaluation.passingThreshold * 100).toFixed(0)}%`}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Continue on Failure</p>
                <p className="mt-1 font-medium">
                  {spec.evaluation.continueOnFailure ? "Yes" : "No"}
                </p>
              </div>
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

      {spec?.type === "loadtest" && spec.loadtest && (
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
                <p className="text-sm text-muted-foreground">Profile Type</p>
                <p className="mt-1 font-medium capitalize">
                  {spec.loadtest.profileType ?? "constant"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Duration</p>
                <p className="mt-1 font-medium">
                  {spec.loadtest.duration ?? "-"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Target RPS</p>
                <p className="mt-1 font-medium">
                  {spec.loadtest.targetRPS ?? "-"}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {spec?.type === "datagen" && spec.datagen && (
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
                <p className="text-sm text-muted-foreground">Sample Count</p>
                <p className="mt-1 font-medium">
                  {spec.datagen.sampleCount ?? "-"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Output Format</p>
                <p className="mt-1 font-medium">
                  {spec.datagen.outputFormat ?? "jsonl"}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Deduplicate</p>
                <p className="mt-1 font-medium">
                  {spec.datagen.deduplicate ? "Yes" : "No"}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Provider Overrides Card */}
      {spec?.providerOverrides && Object.keys(spec.providerOverrides).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Cpu className="h-4 w-4" />
              Provider Overrides
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {Object.entries(spec.providerOverrides).map(([groupName, selector]) => (
                <ProviderGroupOverrideDisplay
                  key={groupName}
                  groupName={groupName}
                  selector={selector}
                />
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Tool Registry Override Card */}
      {spec?.toolRegistryOverride && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Wrench className="h-4 w-4" />
              Tool Registry Override
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ToolRegistryOverrideDisplay selector={spec.toolRegistryOverride} />
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

  // Parse result summary values
  const totalItems = resultSummary ? Number.parseInt(resultSummary.totalItems || "0", 10) : 0;
  const passedItems = resultSummary ? Number.parseInt(resultSummary.passedItems || "0", 10) : 0;
  const failedItems = resultSummary ? Number.parseInt(resultSummary.failedItems || "0", 10) : 0;
  const passRate = resultSummary?.passRate;
  const avgDurationMs = resultSummary?.avgDurationMs;

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

  // Show results summary
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Job Results</CardTitle>
        <CardDescription>
          Results from {job.spec?.type} job
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Total Tests</p>
              <p className="text-2xl font-bold">{totalItems}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Passed</p>
              <p className="text-2xl font-bold text-green-600">
                {passedItems}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Failed</p>
              <p className="text-2xl font-bold text-red-600">
                {failedItems}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Pass Rate</p>
              <p className="text-2xl font-bold">
                {passRate ? `${passRate}%` : "-"}
              </p>
            </div>
          </div>

          {avgDurationMs && (
            <div className="pt-4 border-t">
              <p className="text-sm text-muted-foreground">Average Duration</p>
              <p className="text-lg font-medium">{avgDurationMs}ms</p>
            </div>
          )}

          {resultsUrl && (
            <div className="pt-4 border-t">
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
      </CardContent>
    </Card>
  );
}

function buildCloneInitialValues(job: ArenaJob): QuickRunInitialValues {
  return {
    executionMode: job.spec?.execution?.mode ?? "direct",
    targetAgent: job.spec?.execution?.target?.agentRuntimeRef?.name ?? "",
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
  const { cancelJob, deleteJob } = useArenaJobMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [cancelling, setCancelling] = useState(false);
  const [deleting, setDeleting] = useState(false);
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
            {isFinished && projectId && canEdit && (
              <Button
                variant="outline"
                onClick={() => setCloneDialogOpen(true)}
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
            {job.status?.workers?.active ?? 0} / {job.status?.workers?.desired ?? job.spec?.workers?.replicas ?? 0} workers
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
        {projectId && job.spec?.type && (
          <QuickRunDialog
            open={cloneDialogOpen}
            onOpenChange={setCloneDialogOpen}
            projectId={projectId}
            type={job.spec.type}
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
              workspace={currentWorkspace?.name || ""}
              resourceName={jobName}
              containers={["worker"]}
              showGrafanaLinks={false}
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
