"use client";

import Link from "next/link";
import { Header } from "@/components/layout";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AlertCircle, BookOpen } from "lucide-react";
import { useSkillSources } from "@/hooks/use-skill-sources";
import type { SkillSourcePhase, SkillSourceType } from "@/types/skill-source";

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

function EmptyState() {
  return (
    <div className="text-center py-12 text-muted-foreground">
      <BookOpen className="h-12 w-12 mx-auto mb-4 opacity-50" />
      <p className="text-lg font-medium mb-1">No SkillSources</p>
      <p className="text-sm">
        Create a SkillSource with <code className="font-mono">kubectl apply</code>{" "}
        to fetch skill content from Git, OCI, or a ConfigMap.
      </p>
    </div>
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
  const { sources, loading, error } = useSkillSources();

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
        <div className="rounded-lg border bg-card p-6">
          {sources.length === 0 ? (
            <EmptyState />
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
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </div>
      </div>
    </div>
  );
}
