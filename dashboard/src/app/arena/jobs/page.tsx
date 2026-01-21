"use client";

import { useState, useMemo } from "react";
import { useSearchParams } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaJobs, useArenaJobMutations } from "@/hooks/use-arena-jobs";
import { useArenaConfigs } from "@/hooks/use-arena-configs";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AlertCircle,
  Briefcase,
  Plus,
  MoreHorizontal,
  Trash2,
  LayoutGrid,
  List,
  XCircle,
  Play,
  CheckCircle,
  Clock,
  AlertTriangle,
  Users,
  Activity,
  Database,
  Gauge,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  JobDialog,
} from "@/components/arena";
import type { ArenaJob, ArenaJobPhase, ArenaJobType } from "@/types/arena";

interface JobActionsProps {
  job: ArenaJob;
  onCancel: () => void;
  onDelete: () => void;
  canEdit: boolean;
}

function JobActions({
  job,
  onCancel,
  onDelete,
  canEdit,
}: Readonly<JobActionsProps>) {
  const isRunning = job.status?.phase === "Running" || job.status?.phase === "Pending";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon">
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {isRunning && (
          <DropdownMenuItem onClick={onCancel} disabled={!canEdit}>
            <XCircle className="h-4 w-4 mr-2" />
            Cancel
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          onClick={onDelete}
          disabled={!canEdit || isRunning}
          className="text-destructive"
        >
          <Trash2 className="h-4 w-4 mr-2" />
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

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
    case "Completed":
      return (
        <Badge variant="default" className="gap-1 bg-green-500">
          <CheckCircle className="h-3 w-3" />
          Completed
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

function JobProgress({ job }: Readonly<{ job: ArenaJob }>) {
  const total = job.status?.totalTasks ?? 0;
  const completed = job.status?.completedTasks ?? 0;
  const failed = job.status?.failedTasks ?? 0;

  if (total === 0) {
    return <span className="text-muted-foreground">-</span>;
  }

  const progress = Math.round(((completed + failed) / total) * 100);

  return (
    <div className="flex items-center gap-2 min-w-[120px]">
      <Progress value={progress} className="h-2 flex-1" />
      <span className="text-xs text-muted-foreground whitespace-nowrap">
        {completed}/{total}
      </span>
    </div>
  );
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
    return `${hours}h ${minutes % 60}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  }
  return `${seconds}s`;
}

function JobsTable({
  jobs,
  onCancel,
  onDelete,
  canEdit,
}: Readonly<{
  jobs: ArenaJob[];
  onCancel: (name: string) => void;
  onDelete: (name: string) => void;
  canEdit: boolean;
}>) {
  if (jobs.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Briefcase className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No jobs found</p>
        <p className="text-sm">Create your first job to run evaluations, load tests, or generate data.</p>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Config</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Progress</TableHead>
          <TableHead>Workers</TableHead>
          <TableHead>Duration</TableHead>
          <TableHead className="w-[50px]" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {jobs.map((job) => (
          <TableRow key={job.metadata?.name}>
            <TableCell className="font-medium">
              <Link
                href={`/arena/jobs/${job.metadata?.name}`}
                className="hover:underline text-primary"
              >
                {job.metadata?.name}
              </Link>
            </TableCell>
            <TableCell>
              <Link
                href={`/arena/configs/${job.spec?.configRef?.name}`}
                className="hover:underline text-muted-foreground"
              >
                {job.spec?.configRef?.name || "-"}
              </Link>
            </TableCell>
            <TableCell>{getJobTypeBadge(job.spec?.type)}</TableCell>
            <TableCell>{getJobPhaseBadge(job.status?.phase)}</TableCell>
            <TableCell>
              <JobProgress job={job} />
            </TableCell>
            <TableCell>
              <Badge variant="outline" className="gap-1">
                <Users className="h-3 w-3" />
                {job.status?.workers?.active ?? 0}/{job.status?.workers?.desired ?? job.spec?.workers?.replicas ?? 0}
              </Badge>
            </TableCell>
            <TableCell className="text-muted-foreground">
              {formatDuration(job.status?.startTime, job.status?.completionTime)}
            </TableCell>
            <TableCell>
              <JobActions
                job={job}
                onCancel={() => onCancel(job.metadata?.name || "")}
                onDelete={() => onDelete(job.metadata?.name || "")}
                canEdit={canEdit}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function JobsGrid({
  jobs,
  onCancel,
  onDelete,
  canEdit,
}: Readonly<{
  jobs: ArenaJob[];
  onCancel: (name: string) => void;
  onDelete: (name: string) => void;
  canEdit: boolean;
}>) {
  if (jobs.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Briefcase className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No jobs found</p>
        <p className="text-sm">Create your first job to run evaluations, load tests, or generate data.</p>
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {jobs.map((job) => (
        <Card key={job.metadata?.name} className="hover:bg-muted/50 transition-colors">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">
                <Link
                  href={`/arena/jobs/${job.metadata?.name}`}
                  className="hover:underline"
                >
                  {job.metadata?.name}
                </Link>
              </CardTitle>
              <CardDescription className="flex items-center gap-2">
                {getJobPhaseBadge(job.status?.phase)}
              </CardDescription>
            </div>
            <JobActions
              job={job}
              onCancel={() => onCancel(job.metadata?.name || "")}
              onDelete={() => onDelete(job.metadata?.name || "")}
              canEdit={canEdit}
            />
          </CardHeader>
          <CardContent>
            <div className="space-y-3 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Type</span>
                {getJobTypeBadge(job.spec?.type)}
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Config</span>
                <Link
                  href={`/arena/configs/${job.spec?.configRef?.name}`}
                  className="hover:underline text-primary"
                >
                  {job.spec?.configRef?.name || "-"}
                </Link>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Workers</span>
                <Badge variant="outline" className="gap-1">
                  <Users className="h-3 w-3" />
                  {job.status?.workers?.active ?? 0}/{job.status?.workers?.desired ?? job.spec?.workers?.replicas ?? 0}
                </Badge>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Duration</span>
                <span>{formatDuration(job.status?.startTime, job.status?.completionTime)}</span>
              </div>
              <div className="pt-2">
                <JobProgress job={job} />
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header title="Jobs" description="Manage Arena evaluation jobs" />
      <div className="flex-1 p-6 space-y-6 overflow-auto">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-10 w-32" />
        </div>
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-[200px]" />
          ))}
        </div>
      </div>
    </div>
  );
}

export default function ArenaJobsPage() {
  const searchParams = useSearchParams();
  const initialConfigRef = searchParams.get("configRef") || undefined;

  const [typeFilter, setTypeFilter] = useState<ArenaJobType | "all">("all");
  const [phaseFilter, setPhaseFilter] = useState<ArenaJobPhase | "all">("all");

  const { jobs, loading, error, refetch } = useArenaJobs({
    configRef: initialConfigRef,
  });
  const { configs } = useArenaConfigs();
  const { cancelJob, deleteJob } = useArenaJobMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [viewMode, setViewMode] = useState<"table" | "grid">("grid");
  const [dialogOpen, setDialogOpen] = useState(!!initialConfigRef);
  const [preselectedConfig] = useState<string | undefined>(initialConfigRef);

  const filteredJobs = useMemo(() => {
    return jobs.filter((job) => {
      if (typeFilter !== "all" && job.spec?.type !== typeFilter) {
        return false;
      }
      if (phaseFilter !== "all" && job.status?.phase !== phaseFilter) {
        return false;
      }
      return true;
    });
  }, [jobs, typeFilter, phaseFilter]);

  const handleCancel = async (name: string) => {
    if (!confirm(`Are you sure you want to cancel job "${name}"?`)) {
      return;
    }
    try {
      await cancelJob(name);
      refetch();
    } catch {
      // Error is handled by the hook
    }
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Are you sure you want to delete job "${name}"?`)) {
      return;
    }
    try {
      await deleteJob(name);
      refetch();
    } catch {
      // Error is handled by the hook
    }
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
  };

  const handleSaveSuccess = () => {
    handleDialogClose();
    refetch();
  };

  if (loading) {
    return <LoadingSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Jobs" description="Manage Arena evaluation jobs" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading jobs</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Jobs" description="Manage Arena evaluation jobs" />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Breadcrumb and Actions */}
        <div className="flex items-center justify-between">
          <ArenaBreadcrumb items={[{ label: "Jobs" }]} />
          <div className="flex items-center gap-2">
            <Select
              value={typeFilter}
              onValueChange={(v) => setTypeFilter(v as ArenaJobType | "all")}
            >
              <SelectTrigger className="w-[140px]">
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="evaluation">Evaluation</SelectItem>
                <SelectItem value="loadtest">Load Test</SelectItem>
                <SelectItem value="datagen">Data Gen</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={phaseFilter}
              onValueChange={(v) => setPhaseFilter(v as ArenaJobPhase | "all")}
            >
              <SelectTrigger className="w-[140px]">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="Pending">Pending</SelectItem>
                <SelectItem value="Running">Running</SelectItem>
                <SelectItem value="Completed">Completed</SelectItem>
                <SelectItem value="Failed">Failed</SelectItem>
                <SelectItem value="Cancelled">Cancelled</SelectItem>
              </SelectContent>
            </Select>
            <Tabs
              value={viewMode}
              onValueChange={(v) => setViewMode(v as "table" | "grid")}
            >
              <TabsList>
                <TabsTrigger value="grid">
                  <LayoutGrid className="h-4 w-4" />
                </TabsTrigger>
                <TabsTrigger value="table">
                  <List className="h-4 w-4" />
                </TabsTrigger>
              </TabsList>
            </Tabs>
            {canEdit && (
              <Button onClick={() => setDialogOpen(true)}>
                <Plus className="h-4 w-4 mr-2" />
                Create Job
              </Button>
            )}
          </div>
        </div>

        {/* Jobs View */}
        <div className="rounded-lg border bg-card p-6">
          {viewMode === "table" ? (
            <JobsTable
              jobs={filteredJobs}
              onCancel={handleCancel}
              onDelete={handleDelete}
              canEdit={canEdit}
            />
          ) : (
            <JobsGrid
              jobs={filteredJobs}
              onCancel={handleCancel}
              onDelete={handleDelete}
              canEdit={canEdit}
            />
          )}
        </div>
      </div>

      {/* Create Dialog */}
      <JobDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        configs={configs}
        preselectedConfig={preselectedConfig}
        onSuccess={handleSaveSuccess}
        onClose={handleDialogClose}
      />
    </div>
  );
}
