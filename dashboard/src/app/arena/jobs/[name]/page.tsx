"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaJob, useArenaJobMutations } from "@/hooks/use-arena-jobs";
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
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  formatDate as formatDateBase,
  getConditionIcon,
} from "@/components/arena";
import { LogViewer } from "@/components/logs";
import type { ArenaJob, ArenaJobPhase, ArenaJobType } from "@/types/arena";
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

function OverviewTab({ job }: Readonly<{ job: ArenaJob }>) {
  const { spec, status } = job;

  const total = status?.totalTasks ?? 0;
  const completed = status?.completedTasks ?? 0;
  const failed = status?.failedTasks ?? 0;
  const progress = total > 0 ? Math.round(((completed + failed) / total) * 100) : 0;

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
              <Progress value={progress} className="flex-1" />
              <span className="text-sm font-medium">{progress}%</span>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">Total Tasks</p>
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

      {/* Config Reference Card */}
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
              <p className="text-sm text-muted-foreground">Config Reference</p>
              <Link
                href={`/arena/configs/${spec?.configRef?.name}`}
                className="text-primary hover:underline font-medium"
              >
                {spec?.configRef?.name}
              </Link>
            </div>
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

      {/* Results URL Card */}
      {status?.resultsUrl && (
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
                href={status.resultsUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline flex items-center gap-1"
              >
                {status.resultsUrl}
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
  const resultsUrl = job.status?.resultsUrl;

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

  if (!resultsUrl) {
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

  // For now, show a summary card with link to results
  // In a full implementation, you'd fetch and display the actual results
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
              <p className="text-sm text-muted-foreground">Total Tasks</p>
              <p className="text-2xl font-bold">{job.status?.totalTasks ?? 0}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Completed</p>
              <p className="text-2xl font-bold text-green-600">
                {job.status?.completedTasks ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Failed</p>
              <p className="text-2xl font-bold text-red-600">
                {job.status?.failedTasks ?? 0}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Success Rate</p>
              <p className="text-2xl font-bold">
                {job.status?.totalTasks && job.status.totalTasks > 0
                  ? `${Math.round(((job.status.completedTasks ?? 0) / job.status.totalTasks) * 100)}%`
                  : "-"}
              </p>
            </div>
          </div>

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
        </div>
      </CardContent>
    </Card>
  );
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

  const isRunning = job?.status?.phase === "Running" || job?.status?.phase === "Pending";
  const isFinished = job?.status?.phase === "Succeeded" || job?.status?.phase === "Failed" || job?.status?.phase === "Cancelled";

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
            href={`/arena/configs/${job.spec?.configRef?.name}`}
            className="text-sm text-muted-foreground hover:underline"
          >
            Config: {job.spec?.configRef?.name}
          </Link>
        </div>

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
