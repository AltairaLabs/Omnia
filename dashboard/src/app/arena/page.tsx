"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { StatCard } from "@/components/dashboard";
import { JobDialog } from "@/components/arena";
import { useArenaStats } from "@/hooks";
import { useArenaSources } from "@/hooks/use-arena-sources";
import { useArenaJobMutations } from "@/hooks/use-arena-jobs";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Badge } from "@/components/ui/badge";
import { EnterpriseGate } from "@/components/license/license-gate";
import { Button } from "@/components/ui/button";
import {
  Database,
  Settings,
  Play,
  Plus,
  CheckCircle,
  AlertCircle,
  Loader2,
  Clock,
  Target,
  FileCode,
  LayoutTemplate,
  MoreHorizontal,
  XCircle,
  Trash2,
  Copy,
} from "lucide-react";
import Link from "next/link";
import { generateName } from "@/lib/name-generator";
import type { ArenaJob } from "@/types/arena";

function formatDate(dateString?: string): string {
  if (!dateString) return "-";
  const date = new Date(dateString);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function getJobStatusBadge(phase?: string) {
  switch (phase) {
    case "Running":
      return <Badge variant="default" className="bg-blue-500"><Loader2 className="h-3 w-3 mr-1 animate-spin" /> Running</Badge>;
    case "Succeeded":
      return <Badge variant="default" className="bg-green-500"><CheckCircle className="h-3 w-3 mr-1" /> Succeeded</Badge>;
    case "Failed":
      return <Badge variant="destructive"><AlertCircle className="h-3 w-3 mr-1" /> Failed</Badge>;
    case "Cancelled":
      return <Badge variant="secondary">Cancelled</Badge>;
    case "Pending":
      return <Badge variant="outline"><Clock className="h-3 w-3 mr-1" /> Pending</Badge>;
    default:
      return <Badge variant="outline">{phase || "Unknown"}</Badge>;
  }
}

function getJobTypeBadge(type?: string) {
  switch (type) {
    case "evaluation":
      return <Badge variant="outline" className="border-purple-500 text-purple-600">Evaluation</Badge>;
    case "loadtest":
      return <Badge variant="outline" className="border-orange-500 text-orange-600">Load Test</Badge>;
    case "datagen":
      return <Badge variant="outline" className="border-cyan-500 text-cyan-600">Data Gen</Badge>;
    default:
      return <Badge variant="outline">{type || "Unknown"}</Badge>;
  }
}

interface RecentJobsTableProps {
  jobs: ArenaJob[];
  onCancel: (name: string) => void;
  onDelete: (name: string) => void;
  onClone: (job: ArenaJob) => void;
  canEdit: boolean;
}

function RecentJobsTable({ jobs, onCancel, onDelete, onClone, canEdit }: Readonly<RecentJobsTableProps>) {
  if (jobs.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        No jobs found. Create your first job to get started.
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Progress</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="w-10" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {jobs.map((job) => {
          const completed = job.status?.progress?.completed || 0;
          const total = job.status?.progress?.total || 0;
          const progressPct = total > 0 ? Math.round((completed / total) * 100) : 0;
          const isRunning = job.status?.phase === "Running" || job.status?.phase === "Pending";
          const isFinished = job.status?.phase === "Succeeded" || job.status?.phase === "Failed" || job.status?.phase === "Cancelled";

          return (
            <TableRow key={job.metadata?.name}>
              <TableCell className="font-medium">
                <Link
                  href={`/arena/jobs/${job.metadata?.name}`}
                  className="hover:underline text-primary"
                >
                  {job.metadata?.name}
                </Link>
              </TableCell>
              <TableCell>{getJobTypeBadge(job.spec?.type)}</TableCell>
              <TableCell>{getJobStatusBadge(job.status?.phase)}</TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <div className="w-16 h-2 bg-muted rounded-full overflow-hidden">
                    <div
                      className="h-full bg-primary transition-all"
                      style={{ width: `${progressPct}%` }}
                    />
                  </div>
                  <span className="text-sm text-muted-foreground">{progressPct}%</span>
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDate(job.metadata?.creationTimestamp)}
              </TableCell>
              <TableCell>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="h-8 w-8">
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    {isFinished && (
                      <DropdownMenuItem onClick={() => onClone(job)} disabled={!canEdit}>
                        <Copy className="h-4 w-4 mr-2" />
                        Clone
                      </DropdownMenuItem>
                    )}
                    {isRunning && (
                      <DropdownMenuItem onClick={() => onCancel(job.metadata?.name || "")} disabled={!canEdit}>
                        <XCircle className="h-4 w-4 mr-2" />
                        Cancel
                      </DropdownMenuItem>
                    )}
                    <DropdownMenuItem
                      onClick={() => onDelete(job.metadata?.name || "")}
                      disabled={!canEdit || isRunning}
                      className="text-destructive"
                    >
                      <Trash2 className="h-4 w-4 mr-2" />
                      Delete
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header
        title="Arena"
        description="Evaluate, load test, and generate data for your AI agents"
      />
      <div className="flex-1 p-6 space-y-6 overflow-auto">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-[120px]" />
          ))}
        </div>
        <Skeleton className="h-[300px]" />
      </div>
    </div>
  );
}

function ArenaContent() {
  const router = useRouter();
  const { stats, recentJobs, loading, error, refetch } = useArenaStats();
  const { sources } = useArenaSources();
  const { cancelJob, deleteJob, createJob } = useArenaJobMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;
  const [jobDialogOpen, setJobDialogOpen] = useState(false);

  const handleCancel = async (name: string) => {
    if (!confirm(`Are you sure you want to cancel job "${name}"?`)) return;
    try {
      await cancelJob(name);
      refetch();
    } catch {
      // Error handled by hook
    }
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Are you sure you want to delete job "${name}"?`)) return;
    try {
      await deleteJob(name);
      refetch();
    } catch {
      // Error handled by hook
    }
  };

  const handleClone = async (job: ArenaJob) => {
    if (!job.spec) return;
    try {
      const cloneName = generateName();
      const cloned = await createJob(cloneName, job.spec);
      router.push(`/arena/jobs/${cloned.metadata.name}`);
    } catch {
      // Error handled by hook
    }
  };

  if (loading) {
    return <LoadingSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header
          title="Arena"
          description="Evaluate, load test, and generate data for your AI agents"
        />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading Arena stats</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  const successRatePercent = stats?.jobs.successRate
    ? `${Math.round(stats.jobs.successRate * 100)}%`
    : "N/A";

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Arena"
        description="Evaluate, load test, and generate data for your AI agents"
      />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Stats Cards */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Link href="/arena/sources" className="h-full">
            <StatCard
              title="Active Sources"
              value={stats?.sources.active || 0}
              icon={Database}
              description={stats?.sources.failed ? `${stats.sources.failed} failed` : undefined}
            />
          </Link>

          <Link href="/arena/jobs" className="h-full">
            <StatCard
              title="Running Jobs"
              value={stats?.jobs.running || 0}
              icon={Play}
              description={stats?.jobs.queued ? `${stats.jobs.queued} queued` : undefined}
            />
          </Link>

          <StatCard
            title="Success Rate"
            value={successRatePercent}
            icon={Target}
            description={stats?.jobs.completed ? `${stats.jobs.completed} completed` : undefined}
          />

          <StatCard
            title="Total Jobs"
            value={stats?.jobs.total || 0}
            icon={Settings}
            description={stats?.jobs.failed ? `${stats.jobs.failed} failed` : undefined}
          />
        </div>

        {/* Recent Jobs */}
        <div className="rounded-lg border bg-card p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Recent Jobs</h2>
            <div className="flex items-center gap-2">
              <Button size="sm" variant="outline" onClick={() => setJobDialogOpen(true)}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                New Job
              </Button>
              <Link
                href="/arena/jobs"
                className="text-sm text-muted-foreground hover:text-foreground"
              >
                View all
              </Link>
            </div>
          </div>
          <RecentJobsTable
            jobs={recentJobs}
            onCancel={handleCancel}
            onDelete={handleDelete}
            onClone={handleClone}
            canEdit={canEdit}
          />
        </div>

        {/* Quick Links */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Link
            href="/arena/sources"
            className="rounded-lg border bg-card p-6 hover:bg-muted/50 transition-colors"
          >
            <Database className="h-8 w-8 mb-3 text-muted-foreground" />
            <h3 className="font-semibold mb-1">Manage Sources</h3>
            <p className="text-sm text-muted-foreground">
              Configure Git, OCI, or S3 sources containing arena configurations and scenarios
            </p>
          </Link>

          <Link
            href="/arena/templates"
            className="rounded-lg border bg-card p-6 hover:bg-muted/50 transition-colors"
          >
            <LayoutTemplate className="h-8 w-8 mb-3 text-muted-foreground" />
            <h3 className="font-semibold mb-1">Templates</h3>
            <p className="text-sm text-muted-foreground">
              Browse and use templates to create new arena projects with pre-configured settings
            </p>
          </Link>

          <Link
            href="/arena/projects"
            className="rounded-lg border bg-card p-6 hover:bg-muted/50 transition-colors"
          >
            <FileCode className="h-8 w-8 mb-3 text-muted-foreground" />
            <h3 className="font-semibold mb-1">Project Editor</h3>
            <p className="text-sm text-muted-foreground">
              Create and edit arena project configurations with the built-in YAML editor
            </p>
          </Link>

          <Link
            href="/arena/jobs"
            className="rounded-lg border bg-card p-6 hover:bg-muted/50 transition-colors"
          >
            <Play className="h-8 w-8 mb-3 text-muted-foreground" />
            <h3 className="font-semibold mb-1">Run Jobs</h3>
            <p className="text-sm text-muted-foreground">
              Execute evaluations, load tests, or data generation jobs
            </p>
          </Link>
        </div>
      </div>

      <JobDialog
        open={jobDialogOpen}
        onOpenChange={setJobDialogOpen}
        sources={sources}
        onClose={() => setJobDialogOpen(false)}
      />
    </div>
  );
}

export default function ArenaPage() {
  return (
    <EnterpriseGate featureName="Arena Fleet">
      <ArenaContent />
    </EnterpriseGate>
  );
}
