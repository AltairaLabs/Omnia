"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaConfig, useArenaConfigMutations } from "@/hooks/use-arena-configs";
import { useArenaSources } from "@/hooks";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertCircle,
  Play,
  Pencil,
  Trash2,
  ExternalLink,
  Info,
  AlertTriangle,
  FileText,
  Briefcase,
  Cpu,
  Wrench,
  CheckCircle,
  XCircle,
  Clock,
  Tag,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  ConfigDialog,
  formatDate as formatDateBase,
  getStatusBadge,
  getConditionIcon,
} from "@/components/arena";
import type { ArenaConfig, ArenaJob, Scenario, ArenaProviderStatus, ArenaToolRegistryStatus } from "@/types/arena";
import type { Condition } from "@/types/common";

// Use the shared utilities with detail page specific defaults
const formatDate = (dateString?: string) => formatDateBase(dateString, true);

function getJobPhaseIcon(phase?: string) {
  switch (phase) {
    case "Completed":
      return <CheckCircle className="h-4 w-4 text-green-500" />;
    case "Failed":
    case "Cancelled":
      return <XCircle className="h-4 w-4 text-red-500" />;
    case "Running":
      return <Clock className="h-4 w-4 text-blue-500 animate-pulse" />;
    default:
      return <Clock className="h-4 w-4 text-yellow-500" />;
  }
}

function OverviewTab({ config }: Readonly<{ config: ArenaConfig }>) {
  const { spec, status } = config;

  return (
    <div className="space-y-6">
      {/* Status Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Status</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Phase</p>
              <div className="mt-1">{getStatusBadge(status?.phase)}</div>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Source Revision</p>
              <p className="mt-1 font-mono text-sm truncate" title={status?.sourceRevision}>
                {status?.sourceRevision || "-"}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Scenario Count</p>
              <p className="mt-1 font-medium">{status?.scenarioCount ?? 0}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Suspended</p>
              <p className="mt-1 font-medium">{spec?.suspend ? "Yes" : "No"}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Source Reference Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Source Configuration</CardTitle>
          <CardDescription>PromptKit bundle source reference</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div>
              <p className="text-sm text-muted-foreground">Source Reference</p>
              <Link
                href={`/arena/sources/${spec?.sourceRef?.name}`}
                className="text-primary hover:underline font-medium"
              >
                {spec?.sourceRef?.name}
                {spec?.sourceRef?.namespace && (
                  <span className="text-muted-foreground ml-1">
                    ({spec.sourceRef.namespace})
                  </span>
                )}
              </Link>
            </div>
            {spec?.arenaFile && (
              <div>
                <p className="text-sm text-muted-foreground">Arena File</p>
                <p className="font-mono text-sm">{spec.arenaFile}</p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Scenario Filters Card */}
      {spec?.scenarios && (spec.scenarios.include || spec.scenarios.exclude) && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Scenario Filters</CardTitle>
            <CardDescription>Patterns to include or exclude scenarios</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {spec.scenarios.include && spec.scenarios.include.length > 0 && (
                <div>
                  <p className="text-sm text-muted-foreground mb-1">Include Patterns</p>
                  <div className="flex flex-wrap gap-2">
                    {spec.scenarios.include.map((pattern) => (
                      <Badge key={pattern} variant="secondary" className="font-mono text-xs">
                        {pattern}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
              {spec.scenarios.exclude && spec.scenarios.exclude.length > 0 && (
                <div>
                  <p className="text-sm text-muted-foreground mb-1">Exclude Patterns</p>
                  <div className="flex flex-wrap gap-2">
                    {spec.scenarios.exclude.map((pattern) => (
                      <Badge key={pattern} variant="outline" className="font-mono text-xs">
                        {pattern}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Default Values Card */}
      {spec?.defaults && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Default Values</CardTitle>
            <CardDescription>Default settings for job execution</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-4">
              {spec.defaults.temperature !== undefined && (
                <div>
                  <p className="text-sm text-muted-foreground">Temperature</p>
                  <p className="font-medium">{spec.defaults.temperature}</p>
                </div>
              )}
              {spec.defaults.concurrency !== undefined && (
                <div>
                  <p className="text-sm text-muted-foreground">Concurrency</p>
                  <p className="font-medium">{spec.defaults.concurrency}</p>
                </div>
              )}
              {spec.defaults.timeout && (
                <div>
                  <p className="text-sm text-muted-foreground">Timeout</p>
                  <p className="font-medium">{spec.defaults.timeout}</p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Providers Status Card */}
      {status?.providers && status.providers.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Cpu className="h-4 w-4" />
              Provider Status
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {status.providers.map((provider: ArenaProviderStatus) => (
                <div key={provider.name} className="flex items-center justify-between p-2 rounded border">
                  <span className="font-medium">{provider.name}</span>
                  <Badge
                    variant={provider.status === "Ready" ? "default" : "destructive"}
                    className={provider.status === "Ready" ? "bg-green-500" : ""}
                  >
                    {provider.status}
                  </Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Tool Registries Status Card */}
      {status?.toolRegistries && status.toolRegistries.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Wrench className="h-4 w-4" />
              Tool Registry Status
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {status.toolRegistries.map((registry: ArenaToolRegistryStatus) => (
                <div key={registry.name} className="flex items-center justify-between p-2 rounded border">
                  <div>
                    <span className="font-medium">{registry.name}</span>
                    {registry.toolCount !== undefined && (
                      <span className="text-muted-foreground ml-2">
                        ({registry.toolCount} tools)
                      </span>
                    )}
                  </div>
                  <Badge
                    variant={registry.status === "Ready" ? "default" : "destructive"}
                    className={registry.status === "Ready" ? "bg-green-500" : ""}
                  >
                    {registry.status}
                  </Badge>
                </div>
              ))}
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

function ScenariosTab({ scenarios }: Readonly<{ scenarios: Scenario[] }>) {
  if (scenarios.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <FileText className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No scenarios found</p>
        <p className="text-sm">
          Scenarios will appear here once the source is synced and scenarios are discovered.
        </p>
      </div>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Discovered Scenarios</CardTitle>
        <CardDescription>
          {scenarios.length} scenario{scenarios.length === 1 ? "" : "s"} found in this configuration
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Tags</TableHead>
              <TableHead>Path</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {scenarios.map((scenario) => (
              <TableRow key={scenario.name}>
                <TableCell className="font-medium">
                  {scenario.displayName || scenario.name}
                </TableCell>
                <TableCell className="text-muted-foreground max-w-[300px] truncate">
                  {scenario.description || "-"}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {scenario.tags?.slice(0, 3).map((tag) => (
                      <Badge key={tag} variant="outline" className="text-xs">
                        <Tag className="h-3 w-3 mr-1" />
                        {tag}
                      </Badge>
                    ))}
                    {scenario.tags && scenario.tags.length > 3 && (
                      <Badge variant="outline" className="text-xs">
                        +{scenario.tags.length - 3}
                      </Badge>
                    )}
                  </div>
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {scenario.path}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function JobsTab({
  jobs,
  configName,
}: Readonly<{
  jobs: ArenaJob[];
  configName: string;
}>) {
  if (jobs.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Briefcase className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No jobs found</p>
        <p className="text-sm">
          Create a job using this configuration to see it here.
        </p>
        <Link href={`/arena/jobs?configRef=${configName}`}>
          <Button variant="outline" className="mt-4">
            <ExternalLink className="h-4 w-4 mr-2" />
            Go to Jobs
          </Button>
        </Link>
      </div>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Jobs Using This Config</CardTitle>
        <CardDescription>
          {jobs.length} job{jobs.length === 1 ? "" : "s"} reference this configuration
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Progress</TableHead>
              <TableHead>Started</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {jobs.map((job) => (
              <TableRow key={job.metadata?.name}>
                <TableCell className="font-medium">
                  <Link
                    href={`/arena/jobs/${job.metadata?.name}`}
                    className="hover:underline text-primary flex items-center gap-2"
                  >
                    {getJobPhaseIcon(job.status?.phase)}
                    {job.metadata?.name}
                  </Link>
                </TableCell>
                <TableCell>
                  <Badge variant="secondary" className="capitalize">
                    {job.spec?.type}
                  </Badge>
                </TableCell>
                <TableCell>{getStatusBadge(job.status?.phase)}</TableCell>
                <TableCell>
                  {job.status?.completedTasks !== undefined && job.status?.totalTasks ? (
                    <span>
                      {job.status.completedTasks} / {job.status.totalTasks}
                    </span>
                  ) : (
                    "-"
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(job.status?.startTime)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header title="Config Details" description="Loading config information..." />
      <div className="flex-1 p-6 space-y-6 overflow-auto">
        <Skeleton className="h-8 w-64" />
        <div className="flex gap-2">
          <Skeleton className="h-10 w-24" />
          <Skeleton className="h-10 w-24" />
          <Skeleton className="h-10 w-24" />
        </div>
        <Skeleton className="h-[200px]" />
        <Skeleton className="h-[150px]" />
      </div>
    </div>
  );
}

export default function ArenaConfigDetailPage() {
  const params = useParams();
  const router = useRouter();
  const configName = params.name as string;

  const { config, scenarios, linkedJobs, loading, error, refetch } = useArenaConfig(configName);
  const { sources } = useArenaSources();
  const { deleteConfig } = useArenaConfigMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleRunJob = () => {
    window.location.href = `/arena/jobs?configRef=${configName}`;
  };

  const handleDelete = async () => {
    if (!confirm(`Are you sure you want to delete config "${configName}"?`)) {
      return;
    }
    try {
      setDeleting(true);
      await deleteConfig(configName);
      router.push("/arena/configs");
    } catch {
      setDeleting(false);
      // Error is handled by the hook
    }
  };

  const handleEditSuccess = () => {
    setDialogOpen(false);
    refetch();
  };

  if (loading) {
    return <LoadingSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Config Details" description="Error loading config" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading config</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  if (!config) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Config Details" description="Config not found" />
        <div className="flex-1 p-6">
          <Alert>
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>Config not found</AlertTitle>
            <AlertDescription>
              The config &quot;{configName}&quot; could not be found.
            </AlertDescription>
          </Alert>
          <Link href="/arena/configs">
            <Button variant="outline" className="mt-4">
              Back to Configs
            </Button>
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header
        title={configName}
        description="Arena evaluation configuration"
      />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Breadcrumb and Actions */}
        <div className="flex items-center justify-between">
          <ArenaBreadcrumb
            items={[
              { label: "Configs", href: "/arena/configs" },
              { label: configName },
            ]}
          />
          <div className="flex items-center gap-2">
            <Button
              variant="default"
              onClick={handleRunJob}
              disabled={!canEdit}
            >
              <Play className="h-4 w-4 mr-2" />
              Run Job
            </Button>
            {canEdit && (
              <>
                <Button variant="outline" onClick={() => setDialogOpen(true)}>
                  <Pencil className="h-4 w-4 mr-2" />
                  Edit
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDelete}
                  disabled={deleting}
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </Button>
              </>
            )}
          </div>
        </div>

        {/* Status Summary */}
        <div className="flex items-center gap-4">
          {getStatusBadge(config.status?.phase)}
          <Badge variant="secondary" className="gap-1">
            <FileText className="h-3 w-3" />
            {config.status?.scenarioCount ?? 0} scenarios
          </Badge>
          <Link
            href={`/arena/sources/${config.spec?.sourceRef?.name}`}
            className="text-sm text-muted-foreground hover:underline"
          >
            Source: {config.spec?.sourceRef?.name}
          </Link>
        </div>

        {/* Tabs */}
        <Tabs defaultValue="overview" className="space-y-4">
          <TabsList>
            <TabsTrigger value="overview">
              <Info className="h-4 w-4 mr-2" />
              Overview
            </TabsTrigger>
            <TabsTrigger value="scenarios">
              <FileText className="h-4 w-4 mr-2" />
              Scenarios ({scenarios.length})
            </TabsTrigger>
            <TabsTrigger value="jobs">
              <Briefcase className="h-4 w-4 mr-2" />
              Jobs ({linkedJobs.length})
            </TabsTrigger>
          </TabsList>

          <TabsContent value="overview">
            <OverviewTab config={config} />
          </TabsContent>

          <TabsContent value="scenarios">
            <ScenariosTab scenarios={scenarios} />
          </TabsContent>

          <TabsContent value="jobs">
            <JobsTab jobs={linkedJobs} configName={configName} />
          </TabsContent>
        </Tabs>
      </div>

      {/* Edit Dialog */}
      <ConfigDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        config={config}
        sources={sources}
        onSuccess={handleEditSuccess}
        onClose={() => setDialogOpen(false)}
      />
    </div>
  );
}
