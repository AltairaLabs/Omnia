"use client";

import { use } from "react";
import Link from "next/link";
import dynamic from "next/dynamic";
import { Header } from "@/components/layout";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AlertCircle, ChevronLeft, FolderTree, Info } from "lucide-react";
import { useSkillSource } from "@/hooks/use-skill-sources";
import type {
  SkillSource,
  SkillSourcePhase,
  SkillSourceType,
} from "@/types/skill-source";

const SkillSourceExplorer = dynamic(
  () =>
    import("@/components/skills/skill-source-explorer").then(
      (m) => m.SkillSourceExplorer
    ),
  { ssr: false }
);

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

function sourceUrl(source: SkillSource | null): string {
  if (!source) return "";
  if (source.spec.type === "git" && source.spec.git) return source.spec.git.url;
  if (source.spec.type === "oci" && source.spec.oci) return source.spec.oci.url;
  if (source.spec.type === "configmap" && source.spec.configMap) {
    return `configmap: ${source.spec.configMap.name}`;
  }
  return "";
}

function OverviewContent({ source }: { source: SkillSource }) {
  const filter = source.spec.filter;
  const conditions = source.status?.conditions ?? [];

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Source</CardTitle>
              <CardDescription className="mt-1 flex items-center gap-2">
                <Badge variant={typeVariant(source.spec.type)}>
                  {source.spec.type}
                </Badge>
                <Badge variant={phaseVariant(source.status?.phase)}>
                  {source.status?.phase ?? "Pending"}
                </Badge>
              </CardDescription>
            </div>
            <div className="text-right text-sm text-muted-foreground">
              <div>{source.status?.skillCount ?? 0} skills resolved</div>
              <div className="mt-1">
                Last fetch: {formatDate(source.status?.lastFetchTime)}
              </div>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div>
            <div className="text-muted-foreground text-xs uppercase">
              Reference
            </div>
            <div className="font-mono break-all">{sourceUrl(source)}</div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <div className="text-muted-foreground text-xs uppercase">
                Interval
              </div>
              <div>{source.spec.interval}</div>
            </div>
            {source.spec.timeout && (
              <div>
                <div className="text-muted-foreground text-xs uppercase">
                  Timeout
                </div>
                <div>{source.spec.timeout}</div>
              </div>
            )}
            {source.spec.targetPath && (
              <div>
                <div className="text-muted-foreground text-xs uppercase">
                  Target path
                </div>
                <div className="font-mono">{source.spec.targetPath}</div>
              </div>
            )}
            <div>
              <div className="text-muted-foreground text-xs uppercase">
                Suspended
              </div>
              <div>{source.spec.suspend ? "yes" : "no"}</div>
            </div>
          </div>
          {filter &&
          (filter.include?.length ||
            filter.exclude?.length ||
            filter.names?.length) ? (
            <div>
              <div className="text-muted-foreground text-xs uppercase">
                Filter
              </div>
              <div className="text-xs font-mono whitespace-pre-wrap mt-1">
                {JSON.stringify(filter, null, 2)}
              </div>
            </div>
          ) : null}
          {source.status?.artifact && (
            <div>
              <div className="text-muted-foreground text-xs uppercase">
                Artifact
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="text-muted-foreground">Revision</div>
                <div className="font-mono break-all">
                  {source.status.artifact.revision}
                </div>
                {source.status.artifact.version && (
                  <>
                    <div className="text-muted-foreground">Version</div>
                    <div className="font-mono break-all">
                      {source.status.artifact.version}
                    </div>
                  </>
                )}
                {source.status.artifact.contentPath && (
                  <>
                    <div className="text-muted-foreground">Content path</div>
                    <div className="font-mono break-all">
                      {source.status.artifact.contentPath}
                    </div>
                  </>
                )}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {conditions.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Conditions</CardTitle>
            <CardDescription>Reconciler-reported state</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Reason</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Last Transition</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {conditions.map((c) => (
                  <TableRow key={c.type}>
                    <TableCell className="font-medium">{c.type}</TableCell>
                    <TableCell>
                      <Badge
                        variant={c.status === "True" ? "default" : "destructive"}
                      >
                        {c.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {c.reason}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {c.message}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatDate(c.lastTransitionTime)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default function SkillSourceDetailPage({
  params,
}: {
  params: Promise<{ name: string }>;
}) {
  const { name } = use(params);
  const { source, loading, error } = useSkillSource(name);

  if (loading) {
    return (
      <div className="flex flex-col h-full">
        <Header title={name} description="SkillSource details" />
        <div className="flex-1 p-6 space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header title={name} description="SkillSource details" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error loading skill source</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  if (!source) {
    return (
      <div className="flex flex-col h-full">
        <Header title={name} description="SkillSource details" />
        <div className="flex-1 p-6">
          <Alert>
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Not found</AlertTitle>
            <AlertDescription>
              No SkillSource named <code>{name}</code> in this workspace.
            </AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header
        title={source.metadata.name ?? name}
        description="SkillSource details"
      />

      <div className="flex-1 flex flex-col min-h-0">
        <div className="px-6 pt-6">
          <Link
            href="/skills"
            className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            Back to skills
          </Link>
        </div>

        <Tabs defaultValue="overview" className="flex-1 flex flex-col min-h-0 px-6 pt-4">
          <TabsList>
            <TabsTrigger value="overview">
              <Info className="h-4 w-4 mr-2" />
              Overview
            </TabsTrigger>
            <TabsTrigger value="explorer">
              <FolderTree className="h-4 w-4 mr-2" />
              Explorer
            </TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="overflow-auto pb-6">
            <OverviewContent source={source} />
          </TabsContent>

          <TabsContent value="explorer" className="flex-1 min-h-0 pb-6">
            <SkillSourceExplorer sourceName={name} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
