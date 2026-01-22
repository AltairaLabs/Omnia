"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/layout";
import { useArenaConfig, useArenaConfigMutations, useArenaConfigContent, useArenaConfigFile } from "@/hooks/use-arena-configs";
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
  MessageSquare,
  Package,
  Folder,
  FolderOpen,
  ChevronRight,
  ChevronDown,
  File,
  User,
} from "lucide-react";
import Link from "next/link";
import {
  ArenaBreadcrumb,
  ConfigDialog,
  formatDate as formatDateBase,
  getStatusBadge,
  getConditionIcon,
} from "@/components/arena";
import type {
  ArenaConfig,
  ArenaConfigContent,
  ArenaJob,
  Scenario,
  ArenaProviderStatus,
  ArenaToolRegistryStatus,
  ArenaPackageFile,
  ArenaPackageTreeNode,
} from "@/types/arena";
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

      {/* Resource References Card */}
      {((spec?.providers && spec.providers.length > 0) || (spec?.toolRegistries && spec.toolRegistries.length > 0)) && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Resource References</CardTitle>
            <CardDescription>Providers and tool registries used by this configuration</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {spec?.providers && spec.providers.length > 0 && (
                <div>
                  <p className="text-sm text-muted-foreground mb-2 flex items-center gap-1">
                    <Cpu className="h-3 w-3" />
                    Providers
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {spec.providers.map((ref) => (
                      <Link
                        key={ref.name}
                        href={`/providers/${ref.name}`}
                        className="inline-flex items-center gap-1 px-3 py-1.5 rounded-md border bg-muted/50 hover:bg-muted text-sm font-medium transition-colors"
                      >
                        <Cpu className="h-3 w-3 text-muted-foreground" />
                        {ref.name}
                        {ref.namespace && (
                          <span className="text-muted-foreground text-xs">({ref.namespace})</span>
                        )}
                        <ExternalLink className="h-3 w-3 text-muted-foreground" />
                      </Link>
                    ))}
                  </div>
                </div>
              )}
              {spec?.toolRegistries && spec.toolRegistries.length > 0 && (
                <div>
                  <p className="text-sm text-muted-foreground mb-2 flex items-center gap-1">
                    <Wrench className="h-3 w-3" />
                    Tool Registries
                  </p>
                  <div className="flex flex-wrap gap-2">
                    {spec.toolRegistries.map((ref) => (
                      <Link
                        key={ref.name}
                        href={`/tools/${ref.name}`}
                        className="inline-flex items-center gap-1 px-3 py-1.5 rounded-md border bg-muted/50 hover:bg-muted text-sm font-medium transition-colors"
                      >
                        <Wrench className="h-3 w-3 text-muted-foreground" />
                        {ref.name}
                        {ref.namespace && (
                          <span className="text-muted-foreground text-xs">({ref.namespace})</span>
                        )}
                        <ExternalLink className="h-3 w-3 text-muted-foreground" />
                      </Link>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

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
                  <Link
                    href={`/providers/${provider.name}`}
                    className="font-medium text-primary hover:underline flex items-center gap-1"
                  >
                    {provider.name}
                    <ExternalLink className="h-3 w-3" />
                  </Link>
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
                    <Link
                      href={`/tools/${registry.name}`}
                      className="font-medium text-primary hover:underline inline-flex items-center gap-1"
                    >
                      {registry.name}
                      <ExternalLink className="h-3 w-3" />
                    </Link>
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

/** Get icon for file type */
function getFileTypeIcon(type: ArenaPackageFile["type"]) {
  switch (type) {
    case "arena":
      return <Package className="h-4 w-4 text-purple-500" />;
    case "prompt":
      return <MessageSquare className="h-4 w-4 text-blue-500" />;
    case "provider":
      return <Cpu className="h-4 w-4 text-green-500" />;
    case "scenario":
      return <FileText className="h-4 w-4 text-orange-500" />;
    case "tool":
      return <Wrench className="h-4 w-4 text-yellow-500" />;
    case "persona":
      return <User className="h-4 w-4 text-pink-500" />;
    default:
      return <File className="h-4 w-4 text-muted-foreground" />;
  }
}

/** Get badge color for file type */
function getFileTypeBadge(type: ArenaPackageFile["type"]) {
  const colors: Record<ArenaPackageFile["type"], string> = {
    arena: "bg-purple-500/10 text-purple-700 border-purple-200",
    prompt: "bg-blue-500/10 text-blue-700 border-blue-200",
    provider: "bg-green-500/10 text-green-700 border-green-200",
    scenario: "bg-orange-500/10 text-orange-700 border-orange-200",
    tool: "bg-yellow-500/10 text-yellow-700 border-yellow-200",
    persona: "bg-pink-500/10 text-pink-700 border-pink-200",
    other: "bg-gray-500/10 text-gray-700 border-gray-200",
  };
  return colors[type];
}

/** File tree node component */
function FileTreeNode({
  node,
  selectedPath,
  expandedPaths,
  onSelect,
  onToggle,
  depth = 0,
}: Readonly<{
  node: ArenaPackageTreeNode;
  selectedPath: string | null;
  expandedPaths: Set<string>;
  onSelect: (path: string) => void;
  onToggle: (path: string) => void;
  depth?: number;
}>) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;

  return (
    <div>
      <button
        onClick={() => {
          if (node.isDirectory) {
            onToggle(node.path);
          } else {
            onSelect(node.path);
          }
        }}
        className={`w-full flex items-center gap-1.5 px-2 py-1 text-sm hover:bg-muted rounded transition-colors ${
          isSelected ? "bg-muted font-medium" : ""
        }`}
        style={{ paddingLeft: `${depth * 12 + 8}px` }}
      >
        {node.isDirectory ? (
          <>
            {isExpanded ? (
              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            )}
            {isExpanded ? (
              <FolderOpen className="h-4 w-4 text-amber-500 shrink-0" />
            ) : (
              <Folder className="h-4 w-4 text-amber-500 shrink-0" />
            )}
          </>
        ) : (
          <>
            <span className="w-3.5" />
            {getFileTypeIcon(node.type || "other")}
          </>
        )}
        <span className="truncate">{node.name}</span>
      </button>
      {node.isDirectory && isExpanded && node.children && (
        <div>
          {node.children.map((child) => (
            <FileTreeNode
              key={child.path}
              node={child}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onSelect={onSelect}
              onToggle={onToggle}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/** File content viewer component with Monaco editor */
function FileContentViewer({
  file,
  isEntryPoint,
  configName,
}: Readonly<{
  file: ArenaPackageFile | null;
  isEntryPoint: boolean;
  configName: string;
}>) {
  const { content, loading, error } = useArenaConfigFile(configName, file?.path ?? null);
  const [Editor, setEditor] = useState<typeof import("@monaco-editor/react").default | null>(null);

  // Dynamically import Monaco editor (it's a large bundle)
  useState(() => {
    import("@monaco-editor/react").then((mod) => {
      setEditor(() => mod.default);
    });
  });

  if (!file) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        <div className="text-center">
          <FileText className="h-12 w-12 mx-auto mb-2 opacity-50" />
          <p>Select a file to view its contents</p>
        </div>
      </div>
    );
  }

  // Determine Monaco language from file extension
  const getLanguage = (path: string): string => {
    if (path.endsWith(".yaml") || path.endsWith(".yml")) return "yaml";
    if (path.endsWith(".json")) return "json";
    return "plaintext";
  };

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center gap-2 p-3 border-b bg-muted/30">
        {getFileTypeIcon(file.type)}
        <span className="font-mono text-sm font-medium">{file.path}</span>
        {isEntryPoint && (
          <Badge variant="default" className="ml-2 text-xs">Entry Point</Badge>
        )}
        <Badge variant="outline" className={`ml-auto text-xs ${getFileTypeBadge(file.type)}`}>
          {file.type}
        </Badge>
        <span className="text-xs text-muted-foreground">{file.size} bytes</span>
      </div>
      <div className="flex-1 overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center h-full">
            <Skeleton className="h-4 w-32" />
          </div>
        ) : error ? (
          <div className="flex items-center justify-center h-full text-red-500">
            <AlertCircle className="h-4 w-4 mr-2" />
            {error.message}
          </div>
        ) : Editor && content ? (
          <Editor
            height="100%"
            language={getLanguage(file.path)}
            value={content}
            theme="vs-dark"
            options={{
              readOnly: true,
              minimap: { enabled: false },
              fontSize: 12,
              lineNumbers: "on",
              wordWrap: "on",
              scrollBeyondLastLine: false,
              automaticLayout: true,
            }}
          />
        ) : content ? (
          <div className="p-4 overflow-auto h-full bg-muted/20">
            <pre className="text-xs font-mono whitespace-pre-wrap">{content}</pre>
          </div>
        ) : null}
      </div>
    </div>
  );
}

function PackContentTab({
  content,
  loading,
  configName,
}: Readonly<{
  content: ArenaConfigContent | null;
  loading: boolean;
  configName: string;
}>) {
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  // Auto-expand all directories on initial load and select entry point
  useState(() => {
    if (content?.fileTree) {
      const allDirs = new Set<string>();
      const collectDirs = (nodes: ArenaPackageTreeNode[]) => {
        for (const node of nodes) {
          if (node.isDirectory) {
            allDirs.add(node.path);
            if (node.children) collectDirs(node.children);
          }
        }
      };
      collectDirs(content.fileTree);
      setExpandedPaths(allDirs);
      if (content.entryPoint) {
        setSelectedPath(content.entryPoint);
      }
    }
  });

  if (loading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-[400px]" />
      </div>
    );
  }

  const hasContent = content && content.files && content.files.length > 0;

  if (!hasContent) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Package className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No arena content found</p>
        <p className="text-sm">
          Content will appear here once the source is synced and files are loaded.
        </p>
      </div>
    );
  }

  const selectedFile = content.files.find((f) => f.path === selectedPath) || null;
  const isEntryPoint = selectedPath === content.entryPoint;

  const handleToggle = (path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  // Count files by type
  const typeCounts = content.files.reduce(
    (acc, file) => {
      acc[file.type] = (acc[file.type] || 0) + 1;
      return acc;
    },
    {} as Record<string, number>
  );

  return (
    <div className="space-y-4">
      {/* Summary badges */}
      <div className="flex flex-wrap gap-2">
        <Badge variant="outline" className="gap-1">
          <Package className="h-3 w-3" />
          {content.files.length} files
        </Badge>
        {Object.entries(typeCounts).map(([type, count]) => (
          <Badge key={type} variant="outline" className={`gap-1 ${getFileTypeBadge(type as ArenaPackageFile["type"])}`}>
            {getFileTypeIcon(type as ArenaPackageFile["type"])}
            {count} {type}
          </Badge>
        ))}
      </div>

      {/* File explorer */}
      <Card className="overflow-hidden">
        <div className="flex h-[500px]">
          {/* File tree sidebar */}
          <div className="w-64 border-r bg-muted/30 overflow-auto">
            <div className="p-2 border-b bg-muted/50">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Package Files
              </p>
            </div>
            <div className="py-1">
              {content.fileTree.map((node) => (
                <FileTreeNode
                  key={node.path}
                  node={node}
                  selectedPath={selectedPath}
                  expandedPaths={expandedPaths}
                  onSelect={setSelectedPath}
                  onToggle={handleToggle}
                />
              ))}
            </div>
          </div>

          {/* File content viewer */}
          <div className="flex-1 overflow-hidden">
            <FileContentViewer file={selectedFile} isEntryPoint={isEntryPoint} configName={configName} />
          </div>
        </div>
      </Card>

      {/* Quick reference cards for key resources */}
      {content.entryPoint && (
        <Card>
          <CardHeader className="py-3">
            <CardTitle className="text-sm flex items-center gap-2">
              <Info className="h-4 w-4" />
              Package Summary
            </CardTitle>
          </CardHeader>
          <CardContent className="py-3">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Entry Point</p>
                <p className="font-mono">{content.entryPoint}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Prompts</p>
                <p className="font-medium">{content.promptConfigs.length}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Providers</p>
                <p className="font-medium">{content.providers.length}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Scenarios</p>
                <p className="font-medium">{content.scenarios.length}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Tools</p>
                <p className="font-medium">{content.tools.length}</p>
              </div>
              {content.selfPlay?.enabled && (
                <div>
                  <p className="text-muted-foreground">Self-Play</p>
                  <Badge variant="default" className="bg-green-500">Enabled</Badge>
                </div>
              )}
              {content.judges && Object.keys(content.judges).length > 0 && (
                <div>
                  <p className="text-muted-foreground">Judges</p>
                  <p className="font-medium">{Object.keys(content.judges).length}</p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
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
  const { content: packContent, loading: contentLoading } = useArenaConfigContent(configName);
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
            <TabsTrigger value="pack">
              <Package className="h-4 w-4 mr-2" />
              Pack Content
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

          <TabsContent value="pack">
            <PackContentTab content={packContent} loading={contentLoading} configName={configName} />
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
