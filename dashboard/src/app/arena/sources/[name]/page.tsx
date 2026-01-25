"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaSource, useArenaSourceMutations } from "@/hooks";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
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
  RefreshCw,
  Pencil,
  Trash2,
  History,
  Info,
  AlertTriangle,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  SourceDialog,
  formatDate as formatDateBase,
  formatInterval,
  formatBytes,
  getSourceTypeIcon as getSourceTypeIconBase,
  getStatusBadge,
  getConditionIcon,
} from "@/components/arena";
import type { ArenaSource } from "@/types/arena";
import type { Condition } from "@/types/common";

// Use the shared utilities with detail page specific defaults
const formatDate = (dateString?: string) => formatDateBase(dateString, true);
const getSourceTypeIcon = (type?: ArenaSource["spec"]["type"]) => getSourceTypeIconBase(type, "md");

function OverviewTab({ source }: Readonly<{ source: ArenaSource }>) {
  const { spec, status } = source;

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
              <p className="text-sm text-muted-foreground">Type</p>
              <div className="mt-1 flex items-center gap-2 capitalize">
                {getSourceTypeIcon(spec?.type)}
                {spec?.type}
              </div>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Sync Interval</p>
              <p className="mt-1 font-medium">{formatInterval(spec?.interval)}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Suspended</p>
              <p className="mt-1 font-medium">{spec?.suspend ? "Yes" : "No"}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Configuration Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Configuration</CardTitle>
          <CardDescription>Source-specific settings</CardDescription>
        </CardHeader>
        <CardContent>
          {spec?.type === "git" && spec.git && (
            <div className="space-y-3">
              <div>
                <p className="text-sm text-muted-foreground">Repository URL</p>
                <p className="font-mono text-sm">{spec.git.url}</p>
              </div>
              {spec.git.ref?.branch && (
                <div>
                  <p className="text-sm text-muted-foreground">Branch</p>
                  <p className="font-medium">{spec.git.ref.branch}</p>
                </div>
              )}
              {spec.git.ref?.tag && (
                <div>
                  <p className="text-sm text-muted-foreground">Tag</p>
                  <p className="font-medium">{spec.git.ref.tag}</p>
                </div>
              )}
              {spec.git.path && (
                <div>
                  <p className="text-sm text-muted-foreground">Path</p>
                  <p className="font-mono text-sm">{spec.git.path}</p>
                </div>
              )}
            </div>
          )}
          {spec?.type === "oci" && spec.oci && (
            <div className="space-y-3">
              <div>
                <p className="text-sm text-muted-foreground">OCI Repository</p>
                <p className="font-mono text-sm">{spec.oci.url}</p>
              </div>
              {spec.oci.ref?.tag && (
                <div>
                  <p className="text-sm text-muted-foreground">Tag</p>
                  <p className="font-medium">{spec.oci.ref.tag}</p>
                </div>
              )}
              {spec.oci.ref?.semver && (
                <div>
                  <p className="text-sm text-muted-foreground">SemVer Constraint</p>
                  <p className="font-mono text-sm">{spec.oci.ref.semver}</p>
                </div>
              )}
            </div>
          )}
          {spec?.type === "s3" && spec.s3 && (
            <div className="space-y-3">
              <div>
                <p className="text-sm text-muted-foreground">Bucket</p>
                <p className="font-medium">{spec.s3.bucket}</p>
              </div>
              {spec.s3.prefix && (
                <div>
                  <p className="text-sm text-muted-foreground">Prefix</p>
                  <p className="font-mono text-sm">{spec.s3.prefix}</p>
                </div>
              )}
              {spec.s3.region && (
                <div>
                  <p className="text-sm text-muted-foreground">Region</p>
                  <p className="font-medium">{spec.s3.region}</p>
                </div>
              )}
              {spec.s3.endpoint && (
                <div>
                  <p className="text-sm text-muted-foreground">Endpoint</p>
                  <p className="font-mono text-sm">{spec.s3.endpoint}</p>
                </div>
              )}
            </div>
          )}
          {spec?.type === "configmap" && spec.configMap && (
            <div>
              <p className="text-sm text-muted-foreground">ConfigMap Name</p>
              <p className="font-medium">{spec.configMap.name}</p>
            </div>
          )}
          {spec?.secretRef && (
            <div className="mt-4 pt-4 border-t">
              <p className="text-sm text-muted-foreground">Credentials Secret</p>
              <p className="font-medium">{spec.secretRef.name}</p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Current Artifact Card */}
      {status?.artifact && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Current Artifact</CardTitle>
            <CardDescription>Latest synced artifact information</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-sm text-muted-foreground">Revision</p>
                <p className="font-mono text-sm truncate" title={status.artifact.revision}>
                  {status.artifact.revision}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Last Updated</p>
                <p className="font-medium">{formatDate(status.artifact.lastUpdateTime)}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Size</p>
                <p className="font-medium">{formatBytes(status.artifact.size)}</p>
              </div>
              {status.artifact.checksum && (
                <div>
                  <p className="text-sm text-muted-foreground">Checksum</p>
                  <p className="font-mono text-sm truncate" title={status.artifact.checksum}>
                    {status.artifact.checksum.substring(0, 16)}...
                  </p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function SyncHistoryTab({ source }: Readonly<{ source: ArenaSource }>) {
  const conditions = source.status?.conditions || [];

  if (conditions.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <History className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No sync history</p>
        <p className="text-sm">Sync events will appear here after the source reconciles.</p>
      </div>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Conditions</CardTitle>
        <CardDescription>Current state and recent events</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {conditions.map((condition: Condition) => (
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
  );
}

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header title="Source Details" description="Loading source information..." />
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

export default function ArenaSourceDetailPage() {
  const params = useParams();
  const router = useRouter();
  const sourceName = params.name as string;

  const { source, loading, error, refetch } = useArenaSource(sourceName);
  const { syncSource, deleteSource } = useArenaSourceMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [dialogOpen, setDialogOpen] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleSync = async () => {
    try {
      setSyncing(true);
      await syncSource(sourceName);
      refetch();
    } catch {
      // Error is handled by the hook
    } finally {
      setSyncing(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm(`Are you sure you want to delete source "${sourceName}"?`)) {
      return;
    }
    try {
      setDeleting(true);
      await deleteSource(sourceName);
      router.push("/arena/sources");
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
        <Header title="Source Details" description="Error loading source" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading source</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  if (!source) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Source Details" description="Source not found" />
        <div className="flex-1 p-6">
          <Alert>
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>Source not found</AlertTitle>
            <AlertDescription>
              The source &quot;{sourceName}&quot; could not be found.
            </AlertDescription>
          </Alert>
          <Link href="/arena/sources">
            <Button variant="outline" className="mt-4">
              Back to Sources
            </Button>
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header
        title={sourceName}
        description={`${source.spec?.type?.toUpperCase()} source for PromptKit bundles`}
      />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Breadcrumb and Actions */}
        <div className="flex items-center justify-between">
          <ArenaBreadcrumb
            items={[
              { label: "Sources", href: "/arena/sources" },
              { label: sourceName },
            ]}
          />
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={handleSync}
              disabled={syncing || !canEdit}
            >
              <RefreshCw className={`h-4 w-4 mr-2 ${syncing ? "animate-spin" : ""}`} />
              Sync Now
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
          <div className="flex items-center gap-2">
            {getSourceTypeIcon(source.spec?.type)}
            <span className="capitalize font-medium">{source.spec?.type}</span>
          </div>
          {getStatusBadge(source.status?.phase)}
          {source.status?.artifact && (
            <span className="text-sm text-muted-foreground">
              Last synced: {formatDate(source.status.artifact.lastUpdateTime)}
            </span>
          )}
        </div>

        {/* Tabs */}
        <Tabs defaultValue="overview" className="space-y-4">
          <TabsList>
            <TabsTrigger value="overview">
              <Info className="h-4 w-4 mr-2" />
              Overview
            </TabsTrigger>
            <TabsTrigger value="history">
              <History className="h-4 w-4 mr-2" />
              Sync History
            </TabsTrigger>
          </TabsList>

          <TabsContent value="overview">
            <OverviewTab source={source} />
          </TabsContent>

          <TabsContent value="history">
            <SyncHistoryTab source={source} />
          </TabsContent>
        </Tabs>
      </div>

      {/* Edit Dialog */}
      <SourceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        source={source}
        onSuccess={handleEditSuccess}
        onClose={() => setDialogOpen(false)}
      />
    </div>
  );
}
