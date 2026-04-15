"use client";

import { useState } from "react";
import Link from "next/link";
import { Header } from "@/components/layout";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { AlertCircle, BookOpen, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react";
import {
  useSkillSources,
  useSkillSourceMutations,
} from "@/hooks/use-skill-sources";
import { SkillSourceDialog } from "@/components/skills/skill-source-dialog";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  SkillSource,
  SkillSourcePhase,
  SkillSourceType,
} from "@/types/skill-source";

function phaseVariant(
  phase: SkillSourcePhase | undefined
): "default" | "secondary" | "destructive" | "outline" {
  switch (phase) {
    case "Ready":
      return "default";
    case "Error":
      return "destructive";
    case "Initializing":
    case "Fetching":
      return "secondary";
    default:
      return "outline";
  }
}

function typeVariant(type: SkillSourceType): "default" | "secondary" | "outline" {
  switch (type) {
    case "git":
      return "default";
    case "oci":
      return "secondary";
    default:
      return "outline";
  }
}

function formatDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString();
}

function EmptyState({
  canEdit,
  onCreate,
}: Readonly<{ canEdit: boolean; onCreate: () => void }>) {
  return (
    <div className="text-center py-12 text-muted-foreground">
      <BookOpen className="h-12 w-12 mx-auto mb-4 opacity-50" />
      <p className="text-lg font-medium mb-1">No SkillSources</p>
      <p className="text-sm">
        SkillSources pull SKILL.md content from Git, OCI, or a ConfigMap so
        PromptPacks can reference skills.
      </p>
      {canEdit && (
        <Button className="mt-4" onClick={onCreate}>
          <Plus className="h-4 w-4 mr-2" />
          Create SkillSource
        </Button>
      )}
    </div>
  );
}

function RowActions({
  onEdit,
  onDelete,
  canEdit,
}: Readonly<{
  onEdit: () => void;
  onDelete: () => void;
  canEdit: boolean;
}>) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" disabled={!canEdit}>
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
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

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header title="Skills" description="Skill sources visible to this workspace" />
      <div className="flex-1 p-6 space-y-4 overflow-auto">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    </div>
  );
}

export default function SkillsPage() {
  const { sources, loading, error, refetch } = useSkillSources();
  const { deleteSource } = useSkillSourceMutations();
  const { currentWorkspace } = useWorkspace();
  const canEdit = currentWorkspace?.permissions?.write ?? false;

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<SkillSource | null>(null);

  const openCreate = () => {
    setEditing(null);
    setDialogOpen(true);
  };

  const openEdit = (source: SkillSource) => {
    setEditing(source);
    setDialogOpen(true);
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete SkillSource "${name}"?`)) return;
    try {
      await deleteSource(name);
      refetch();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Delete failed");
    }
  };

  if (loading) {
    return <LoadingSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Skills" description="Skill sources visible to this workspace" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading skill sources</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Skills"
        description="Skill sources visible to this workspace"
      />

      <div className="flex-1 p-6 space-y-4 overflow-auto">
        {sources.length > 0 && canEdit && (
          <div className="flex justify-end">
            <Button onClick={openCreate}>
              <Plus className="h-4 w-4 mr-2" />
              Create SkillSource
            </Button>
          </div>
        )}

        <div className="rounded-lg border bg-card p-6">
          {sources.length === 0 ? (
            <EmptyState canEdit={canEdit} onCreate={openCreate} />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Phase</TableHead>
                  <TableHead>Skills</TableHead>
                  <TableHead>Interval</TableHead>
                  <TableHead>Last Fetch</TableHead>
                  <TableHead className="w-[50px]" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {sources.map((source) => {
                  const name = source.metadata.name ?? "";
                  return (
                    <TableRow key={name}>
                      <TableCell className="font-medium">
                        <Link
                          href={`/skills/${encodeURIComponent(name)}`}
                          className="hover:underline text-primary"
                        >
                          {name}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <Badge variant={typeVariant(source.spec.type)}>
                          {source.spec.type}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={phaseVariant(source.status?.phase)}>
                          {source.status?.phase ?? "Pending"}
                        </Badge>
                      </TableCell>
                      <TableCell>{source.status?.skillCount ?? 0}</TableCell>
                      <TableCell className="text-muted-foreground">
                        {source.spec.interval}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDate(source.status?.lastFetchTime)}
                      </TableCell>
                      <TableCell>
                        <RowActions
                          canEdit={canEdit}
                          onEdit={() => openEdit(source)}
                          onDelete={() => handleDelete(name)}
                        />
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </div>
      </div>

      <SkillSourceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        source={editing}
        onSuccess={() => {
          setDialogOpen(false);
          setEditing(null);
          refetch();
        }}
      />
    </div>
  );
}
