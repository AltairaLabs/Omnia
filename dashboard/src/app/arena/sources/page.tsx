"use client";

import { useState } from "react";
import { Header } from "@/components/layout";
import { useArenaSources, useArenaSourceMutations } from "@/hooks";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AlertCircle,
  Database,
  Plus,
  MoreHorizontal,
  RefreshCw,
  Trash2,
  Pencil,
  LayoutGrid,
  List,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  SourceDialog,
  formatDate,
  formatInterval,
  getSourceTypeBadge,
  getStatusBadge,
  getSourceUrl,
} from "@/components/arena";
import type { ArenaSource } from "@/types/arena";

interface SourceActionsProps {
  onSync: () => void;
  onEdit: () => void;
  onDelete: () => void;
  canEdit: boolean;
  syncing: boolean;
}

function SourceActions({
  onSync,
  onEdit,
  onDelete,
  canEdit,
  syncing,
}: Readonly<SourceActionsProps>) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon">
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onSync} disabled={syncing || !canEdit}>
          <RefreshCw className={`h-4 w-4 mr-2 ${syncing ? "animate-spin" : ""}`} />
          Sync Now
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onEdit} disabled={!canEdit}>
          <Pencil className="h-4 w-4 mr-2" />
          Edit
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={onDelete}
          disabled={!canEdit}
          className="text-destructive"
        >
          <Trash2 className="h-4 w-4 mr-2" />
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SourcesTable({
  sources,
  onSync,
  onEdit,
  onDelete,
  canEdit,
  syncing,
}: Readonly<{
  sources: ArenaSource[];
  onSync: (name: string) => void;
  onEdit: (source: ArenaSource) => void;
  onDelete: (name: string) => void;
  canEdit: boolean;
  syncing: string | null;
}>) {
  if (sources.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Database className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No sources found</p>
        <p className="text-sm">Create your first source to get started with Arena.</p>
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
          <TableHead>URL / Reference</TableHead>
          <TableHead>Interval</TableHead>
          <TableHead>Last Update</TableHead>
          <TableHead className="w-[50px]" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {sources.map((source) => (
          <TableRow key={source.metadata?.name}>
            <TableCell className="font-medium">
              <Link
                href={`/arena/sources/${source.metadata?.name}`}
                className="hover:underline text-primary"
              >
                {source.metadata?.name}
              </Link>
            </TableCell>
            <TableCell>{getSourceTypeBadge(source.spec?.type)}</TableCell>
            <TableCell>{getStatusBadge(source.status?.phase)}</TableCell>
            <TableCell className="max-w-[300px] truncate font-mono text-sm text-muted-foreground">
              {getSourceUrl(source)}
            </TableCell>
            <TableCell className="text-muted-foreground">
              {formatInterval(source.spec?.interval)}
            </TableCell>
            <TableCell className="text-muted-foreground">
              {formatDate(source.status?.artifact?.lastUpdateTime)}
            </TableCell>
            <TableCell>
              <SourceActions
                onSync={() => onSync(source.metadata?.name || "")}
                onEdit={() => onEdit(source)}
                onDelete={() => onDelete(source.metadata?.name || "")}
                canEdit={canEdit}
                syncing={syncing === source.metadata?.name}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function SourcesGrid({
  sources,
  onSync,
  onEdit,
  onDelete,
  canEdit,
  syncing,
}: Readonly<{
  sources: ArenaSource[];
  onSync: (name: string) => void;
  onEdit: (source: ArenaSource) => void;
  onDelete: (name: string) => void;
  canEdit: boolean;
  syncing: string | null;
}>) {
  if (sources.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Database className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No sources found</p>
        <p className="text-sm">Create your first source to get started with Arena.</p>
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {sources.map((source) => (
        <Card key={source.metadata?.name} className="hover:bg-muted/50 transition-colors">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">
                <Link
                  href={`/arena/sources/${source.metadata?.name}`}
                  className="hover:underline"
                >
                  {source.metadata?.name}
                </Link>
              </CardTitle>
              <CardDescription className="flex items-center gap-2">
                {getSourceTypeBadge(source.spec?.type)}
              </CardDescription>
            </div>
            <SourceActions
              onSync={() => onSync(source.metadata?.name || "")}
              onEdit={() => onEdit(source)}
              onDelete={() => onDelete(source.metadata?.name || "")}
              canEdit={canEdit}
              syncing={syncing === source.metadata?.name}
            />
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Status</span>
                {getStatusBadge(source.status?.phase)}
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Interval</span>
                <span>{formatInterval(source.spec?.interval)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Last Update</span>
                <span>{formatDate(source.status?.artifact?.lastUpdateTime)}</span>
              </div>
              <div className="pt-2 border-t">
                <span className="text-muted-foreground text-xs font-mono truncate block">
                  {getSourceUrl(source)}
                </span>
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
      <Header title="Sources" description="Manage PromptKit bundle sources" />
      <div className="flex-1 p-6 space-y-6 overflow-auto">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-10 w-32" />
        </div>
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-[180px]" />
          ))}
        </div>
      </div>
    </div>
  );
}

export default function ArenaSourcesPage() {
  const { sources, loading, error, refetch } = useArenaSources();
  const { syncSource, deleteSource } = useArenaSourceMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [viewMode, setViewMode] = useState<"table" | "grid">("grid");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingSource, setEditingSource] = useState<ArenaSource | null>(null);
  const [syncing, setSyncing] = useState<string | null>(null);

  const handleSync = async (name: string) => {
    try {
      setSyncing(name);
      await syncSource(name);
      refetch();
    } catch {
      // Error is handled by the hook
    } finally {
      setSyncing(null);
    }
  };

  const handleEdit = (source: ArenaSource) => {
    setEditingSource(source);
    setDialogOpen(true);
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Are you sure you want to delete source "${name}"?`)) {
      return;
    }
    try {
      await deleteSource(name);
      refetch();
    } catch {
      // Error is handled by the hook
    }
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
    setEditingSource(null);
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
        <Header title="Sources" description="Manage PromptKit bundle sources" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading sources</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Sources" description="Manage PromptKit bundle sources" />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Breadcrumb and Actions */}
        <div className="flex items-center justify-between">
          <ArenaBreadcrumb items={[{ label: "Sources" }]} />
          <div className="flex items-center gap-2">
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
                Create Source
              </Button>
            )}
          </div>
        </div>

        {/* Sources View */}
        <div className="rounded-lg border bg-card p-6">
          {viewMode === "table" ? (
            <SourcesTable
              sources={sources}
              onSync={handleSync}
              onEdit={handleEdit}
              onDelete={handleDelete}
              canEdit={canEdit}
              syncing={syncing}
            />
          ) : (
            <SourcesGrid
              sources={sources}
              onSync={handleSync}
              onEdit={handleEdit}
              onDelete={handleDelete}
              canEdit={canEdit}
              syncing={syncing}
            />
          )}
        </div>
      </div>

      {/* Create/Edit Dialog */}
      <SourceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        source={editingSource}
        onSuccess={handleSaveSuccess}
        onClose={handleDialogClose}
      />
    </div>
  );
}
