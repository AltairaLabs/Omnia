"use client";

import { useState } from "react";
import { Header } from "@/components/layout";
import { useArenaConfigs, useArenaConfigMutations } from "@/hooks/use-arena-configs";
import { useArenaSources } from "@/hooks";
import { useWorkspace } from "@/contexts/workspace-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
  Settings,
  Plus,
  MoreHorizontal,
  Trash2,
  Pencil,
  LayoutGrid,
  List,
  Play,
  FileText,
  Cpu,
  Wrench,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  ConfigDialog,
  getStatusBadge,
} from "@/components/arena";
import type { ArenaConfig } from "@/types/arena";

interface ConfigActionsProps {
  onRunJob: () => void;
  onEdit: () => void;
  onDelete: () => void;
  canEdit: boolean;
}

function ConfigActions({
  onRunJob,
  onEdit,
  onDelete,
  canEdit,
}: Readonly<ConfigActionsProps>) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon">
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onRunJob} disabled={!canEdit}>
          <Play className="h-4 w-4 mr-2" />
          Run Job
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

function ConfigsTable({
  configs,
  onRunJob,
  onEdit,
  onDelete,
  canEdit,
}: Readonly<{
  configs: ArenaConfig[];
  onRunJob: (config: ArenaConfig) => void;
  onEdit: (config: ArenaConfig) => void;
  onDelete: (name: string) => void;
  canEdit: boolean;
}>) {
  if (configs.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Settings className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No configs found</p>
        <p className="text-sm">Create your first config to get started with Arena.</p>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Source</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Scenarios</TableHead>
          <TableHead>Providers</TableHead>
          <TableHead>Tool Registries</TableHead>
          <TableHead className="w-[50px]" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {configs.map((config) => (
          <TableRow key={config.metadata?.name}>
            <TableCell className="font-medium">
              <Link
                href={`/arena/configs/${config.metadata?.name}`}
                className="hover:underline text-primary"
              >
                {config.metadata?.name}
              </Link>
            </TableCell>
            <TableCell>
              <Link
                href={`/arena/sources/${config.spec?.sourceRef?.name}`}
                className="hover:underline text-muted-foreground"
              >
                {config.spec?.sourceRef?.name || "-"}
              </Link>
            </TableCell>
            <TableCell>{getStatusBadge(config.status?.phase)}</TableCell>
            <TableCell>
              <Badge variant="secondary" className="gap-1">
                <FileText className="h-3 w-3" />
                {config.status?.scenarioCount ?? 0}
              </Badge>
            </TableCell>
            <TableCell>
              <Badge variant="outline" className="gap-1">
                <Cpu className="h-3 w-3" />
                {config.spec?.providers?.length ?? 0}
              </Badge>
            </TableCell>
            <TableCell>
              <Badge variant="outline" className="gap-1">
                <Wrench className="h-3 w-3" />
                {config.spec?.toolRegistries?.length ?? 0}
              </Badge>
            </TableCell>
            <TableCell>
              <ConfigActions
                onRunJob={() => onRunJob(config)}
                onEdit={() => onEdit(config)}
                onDelete={() => onDelete(config.metadata?.name || "")}
                canEdit={canEdit}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function ConfigsGrid({
  configs,
  onRunJob,
  onEdit,
  onDelete,
  canEdit,
}: Readonly<{
  configs: ArenaConfig[];
  onRunJob: (config: ArenaConfig) => void;
  onEdit: (config: ArenaConfig) => void;
  onDelete: (name: string) => void;
  canEdit: boolean;
}>) {
  if (configs.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Settings className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No configs found</p>
        <p className="text-sm">Create your first config to get started with Arena.</p>
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {configs.map((config) => (
        <Card key={config.metadata?.name} className="hover:bg-muted/50 transition-colors">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">
                <Link
                  href={`/arena/configs/${config.metadata?.name}`}
                  className="hover:underline"
                >
                  {config.metadata?.name}
                </Link>
              </CardTitle>
              <CardDescription className="flex items-center gap-2">
                {getStatusBadge(config.status?.phase)}
              </CardDescription>
            </div>
            <ConfigActions
              onRunJob={() => onRunJob(config)}
              onEdit={() => onEdit(config)}
              onDelete={() => onDelete(config.metadata?.name || "")}
              canEdit={canEdit}
            />
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Source</span>
                <Link
                  href={`/arena/sources/${config.spec?.sourceRef?.name}`}
                  className="hover:underline text-primary"
                >
                  {config.spec?.sourceRef?.name || "-"}
                </Link>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Scenarios</span>
                <Badge variant="secondary" className="gap-1">
                  <FileText className="h-3 w-3" />
                  {config.status?.scenarioCount ?? 0}
                </Badge>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Providers</span>
                <Badge variant="outline" className="gap-1">
                  <Cpu className="h-3 w-3" />
                  {config.spec?.providers?.length ?? 0}
                </Badge>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Tool Registries</span>
                <Badge variant="outline" className="gap-1">
                  <Wrench className="h-3 w-3" />
                  {config.spec?.toolRegistries?.length ?? 0}
                </Badge>
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
      <Header title="Configs" description="Manage Arena evaluation configurations" />
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

export default function ArenaConfigsPage() {
  const { configs, loading, error, refetch } = useArenaConfigs();
  const { sources } = useArenaSources();
  const { deleteConfig } = useArenaConfigMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [viewMode, setViewMode] = useState<"table" | "grid">("grid");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ArenaConfig | null>(null);

  const handleRunJob = (config: ArenaConfig) => {
    // Navigate to jobs page with config pre-selected
    window.location.href = `/arena/jobs?configRef=${config.metadata?.name}`;
  };

  const handleEdit = (config: ArenaConfig) => {
    setEditingConfig(config);
    setDialogOpen(true);
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Are you sure you want to delete config "${name}"?`)) {
      return;
    }
    try {
      await deleteConfig(name);
      refetch();
    } catch {
      // Error is handled by the hook
    }
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
    setEditingConfig(null);
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
        <Header title="Configs" description="Manage Arena evaluation configurations" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading configs</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Configs" description="Manage Arena evaluation configurations" />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Breadcrumb and Actions */}
        <div className="flex items-center justify-between">
          <ArenaBreadcrumb items={[{ label: "Configs" }]} />
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
                Create Config
              </Button>
            )}
          </div>
        </div>

        {/* Configs View */}
        <div className="rounded-lg border bg-card p-6">
          {viewMode === "table" ? (
            <ConfigsTable
              configs={configs}
              onRunJob={handleRunJob}
              onEdit={handleEdit}
              onDelete={handleDelete}
              canEdit={canEdit}
            />
          ) : (
            <ConfigsGrid
              configs={configs}
              onRunJob={handleRunJob}
              onEdit={handleEdit}
              onDelete={handleDelete}
              canEdit={canEdit}
            />
          )}
        </div>
      </div>

      {/* Create/Edit Dialog */}
      <ConfigDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        config={editingConfig}
        sources={sources}
        onSuccess={handleSaveSuccess}
        onClose={handleDialogClose}
      />
    </div>
  );
}
