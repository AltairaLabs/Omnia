"use client";

import { use } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  ExternalLink,
  Bot,
  Clock,
  Wrench,
  Globe,
  Server,
  CheckCircle,
  XCircle,
  AlertCircle,
  Settings,
  FileJson,
  Terminal,
} from "lucide-react";
import { Header } from "@/components/layout";
import { StatusBadge } from "@/components/agents";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { YamlBlock } from "@/components/ui/yaml-block";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { useToolRegistry } from "@/hooks";
import { useAgents } from "@/hooks";
import type { DiscoveredTool, HandlerDefinition } from "@/types";

interface PageProps {
  params: Promise<{ name: string }>;
}

function formatRelativeTime(timestamp?: string): string {
  if (!timestamp) return "-";
  const date = new Date(timestamp);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

function getHandlerTypeIcon(type: string) {
  switch (type) {
    case "http":
      return Globe;
    case "grpc":
      return Server;
    case "mcp":
      return Terminal;
    case "openapi":
      return FileJson;
    default:
      return Wrench;
  }
}

function ToolStatusIcon({ status }: { status: DiscoveredTool["status"] }) {
  switch (status) {
    case "Available":
      return <CheckCircle className="h-4 w-4 text-green-500" />;
    case "Unavailable":
      return <XCircle className="h-4 w-4 text-red-500" />;
    default:
      return <AlertCircle className="h-4 w-4 text-yellow-500" />;
  }
}

function getEndpointFromHandler(handler: HandlerDefinition): string {
  if (handler.httpConfig?.endpoint) return handler.httpConfig.endpoint;
  if (handler.grpcConfig?.endpoint) return handler.grpcConfig.endpoint;
  if (handler.openAPIConfig?.specURL) return handler.openAPIConfig.specURL;
  if (handler.mcpConfig?.endpoint) return handler.mcpConfig.endpoint;
  if (handler.mcpConfig?.command) return `${handler.mcpConfig.command} ${handler.mcpConfig.args?.join(" ") || ""}`;
  return "N/A";
}

export default function ToolDetailPage({ params }: PageProps) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const namespace = searchParams.get("namespace") || "production";

  const { data: registry, isLoading } = useToolRegistry(name, namespace);
  const { data: agents } = useAgents();

  // Find agents that reference this ToolRegistry
  const usedByAgents = agents?.filter(
    (agent) =>
      agent.spec.toolRegistryRef?.name === name &&
      (agent.spec.toolRegistryRef?.namespace || agent.metadata.namespace) === namespace
  );

  if (isLoading) {
    return (
      <div className="flex flex-col h-full">
        <Header title={<Skeleton className="h-8 w-48" />} />
        <div className="flex-1 p-6 space-y-6">
          <Skeleton className="h-[200px]" />
          <Skeleton className="h-[300px]" />
        </div>
      </div>
    );
  }

  if (!registry) {
    return (
      <div className="flex flex-col h-full">
        <Header title="ToolRegistry Not Found" />
        <div className="flex-1 p-6 flex items-center justify-center">
          <div className="text-center">
            <p className="text-muted-foreground mb-4">
              ToolRegistry &quot;{name}&quot; was not found in namespace &quot;{namespace}&quot;
            </p>
            <Link href="/tools">
              <Button variant="outline">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Tools
              </Button>
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const { metadata, spec, status } = registry;
  const tools = status?.discoveredTools || [];
  const availableCount = tools.filter((t) => t.status === "Available").length;
  const totalCount = status?.discoveredToolsCount || 0;
  // Handle missing handlers gracefully (for data compatibility)
  const handlers = spec.handlers || [];

  return (
    <div className="flex flex-col h-full">
      <Header
        title={
          <div className="flex items-center gap-3">
            <Link href="/tools">
              <Button variant="ghost" size="icon" className="h-8 w-8">
                <ArrowLeft className="h-4 w-4" />
              </Button>
            </Link>
            <Wrench className="h-5 w-5 text-muted-foreground" />
            <span>{metadata.name}</span>
            <StatusBadge phase={status?.phase} />
          </div>
        }
        description={`${metadata.namespace} · ${availableCount}/${totalCount} tools available`}
      />

      <div className="flex-1 p-6">
        <Tabs defaultValue="overview" className="space-y-6">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="tools">Tools ({totalCount})</TabsTrigger>
            <TabsTrigger value="handlers">Handlers ({handlers.length})</TabsTrigger>
            <TabsTrigger value="usage">Usage ({usedByAgents?.length || 0})</TabsTrigger>
            <TabsTrigger value="config">Config</TabsTrigger>
          </TabsList>

          {/* Overview Tab */}
          <TabsContent value="overview" className="space-y-6">
            <div className="grid gap-6 md:grid-cols-2">
              {/* Status Card */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Status</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Phase</span>
                    <StatusBadge phase={status?.phase} />
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Discovered Tools</span>
                    <span className="text-sm font-medium">{totalCount}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Available</span>
                    <Badge variant="secondary" className="text-green-600 dark:text-green-400">
                      {availableCount}
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Unavailable</span>
                    <Badge variant="secondary" className="text-red-600 dark:text-red-400">
                      {totalCount - availableCount}
                    </Badge>
                  </div>
                  <Separator />
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Last Discovery</span>
                    <span className="text-sm">{formatRelativeTime(status?.lastDiscoveryTime)}</span>
                  </div>
                </CardContent>
              </Card>

              {/* Handler Types Card */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Handlers</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Total Handlers</span>
                    <span className="text-sm font-medium">{handlers.length}</span>
                  </div>
                  <Separator />
                  <div className="space-y-2">
                    <span className="text-sm text-muted-foreground">Handler Types</span>
                    <div className="flex flex-wrap gap-2">
                      {[...new Set(handlers.map((h) => h.type))].map((type) => {
                        const Icon = getHandlerTypeIcon(type);
                        const count = handlers.filter((h) => h.type === type).length;
                        return (
                          <Badge key={type} variant="outline" className="gap-1.5">
                            <Icon className="h-3 w-3" />
                            {type}
                            <span className="text-muted-foreground">({count})</span>
                          </Badge>
                        );
                      })}
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Conditions */}
            {status?.conditions && status.conditions.length > 0 && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Conditions</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {status.conditions.map((condition) => (
                      <div key={condition.type} className="flex items-start gap-3 p-3 rounded-lg bg-muted/50">
                        {condition.status === "True" ? (
                          <CheckCircle className="h-4 w-4 text-green-500 mt-0.5" />
                        ) : (
                          <XCircle className="h-4 w-4 text-red-500 mt-0.5" />
                        )}
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-sm">{condition.type}</span>
                            <span className="text-xs text-muted-foreground">
                              {formatRelativeTime(condition.lastTransitionTime)}
                            </span>
                          </div>
                          {condition.message && (
                            <p className="text-sm text-muted-foreground mt-1">{condition.message}</p>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            )}
          </TabsContent>

          {/* Tools Tab */}
          <TabsContent value="tools" className="space-y-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Discovered Tools</CardTitle>
                <CardDescription>
                  Tools discovered and available from this registry
                </CardDescription>
              </CardHeader>
              <CardContent>
                {tools.length === 0 ? (
                  <p className="text-sm text-muted-foreground text-center py-8">
                    No tools discovered yet
                  </p>
                ) : (
                  <div className="space-y-4">
                    {tools.map((tool) => (
                      <div
                        key={tool.name}
                        className="p-4 rounded-lg border bg-card"
                      >
                        <div className="flex items-start justify-between gap-4">
                          <div className="flex items-start gap-3 min-w-0">
                            <ToolStatusIcon status={tool.status} />
                            <div className="min-w-0">
                              <div className="flex items-center gap-2">
                                <code className="font-medium">{tool.name}</code>
                                <Badge variant="outline" className="text-xs">
                                  {tool.handlerName}
                                </Badge>
                              </div>
                              <p className="text-sm text-muted-foreground mt-1">
                                {tool.description}
                              </p>
                              <div className="flex items-center gap-2 mt-2 text-xs text-muted-foreground">
                                <code className="bg-muted px-1.5 py-0.5 rounded truncate max-w-[300px]">
                                  {tool.endpoint}
                                </code>
                              </div>
                              {tool.error && (
                                <p className="text-sm text-red-500 mt-2">
                                  Error: {tool.error}
                                </p>
                              )}
                            </div>
                          </div>
                          <div className="text-xs text-muted-foreground shrink-0">
                            <Clock className="h-3 w-3 inline mr-1" />
                            {formatRelativeTime(tool.lastChecked)}
                          </div>
                        </div>

                        {tool.inputSchema != null && (
                          <div className="mt-4 pt-4 border-t">
                            <div className="flex items-center gap-2 mb-2">
                              <FileJson className="h-3.5 w-3.5 text-muted-foreground" />
                              <span className="text-xs font-medium">Input Schema</span>
                            </div>
                            <pre className="text-xs bg-muted p-3 rounded overflow-auto max-h-32">
                              {JSON.stringify(tool.inputSchema, null, 2)}
                            </pre>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          {/* Handlers Tab */}
          <TabsContent value="handlers" className="space-y-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Handler Definitions</CardTitle>
                <CardDescription>
                  Configured handlers for discovering and executing tools
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Accordion type="multiple" className="w-full">
                  {handlers.map((handler) => {
                    const Icon = getHandlerTypeIcon(handler.type);
                    return (
                      <AccordionItem key={handler.name} value={`handler-${handler.name}`}>
                        <AccordionTrigger className="hover:no-underline">
                          <div className="flex items-center gap-3">
                            <Icon className="h-4 w-4 text-muted-foreground" />
                            <span className="font-medium">{handler.name}</span>
                            <Badge variant="outline" className="text-xs capitalize">
                              {handler.type}
                            </Badge>
                          </div>
                        </AccordionTrigger>
                        <AccordionContent className="space-y-4">
                          {/* Tool Definition */}
                          {handler.tool && (
                            <div className="space-y-2">
                              <h4 className="text-sm font-medium flex items-center gap-2">
                                <Wrench className="h-3.5 w-3.5" />
                                Tool Definition
                              </h4>
                              <div className="bg-muted rounded-lg p-3 space-y-2">
                                <div className="flex items-center gap-2">
                                  <span className="text-xs text-muted-foreground w-20">Name:</span>
                                  <code className="text-xs">{handler.tool.name}</code>
                                </div>
                                <div className="flex items-start gap-2">
                                  <span className="text-xs text-muted-foreground w-20">Description:</span>
                                  <span className="text-xs">{handler.tool.description}</span>
                                </div>
                              </div>
                            </div>
                          )}

                          {/* Endpoint/Config */}
                          <div className="space-y-2">
                            <h4 className="text-sm font-medium flex items-center gap-2">
                              <Settings className="h-3.5 w-3.5" />
                              Configuration
                            </h4>
                            <div className="bg-muted rounded-lg p-3 space-y-2">
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-muted-foreground w-20">Endpoint:</span>
                                <code className="text-xs break-all">{getEndpointFromHandler(handler)}</code>
                              </div>
                              {handler.httpConfig?.method && (
                                <div className="flex items-center gap-2">
                                  <span className="text-xs text-muted-foreground w-20">Method:</span>
                                  <Badge variant="secondary" className="text-xs">{handler.httpConfig.method}</Badge>
                                </div>
                              )}
                              {handler.httpConfig?.authType && handler.httpConfig.authType !== "none" && (
                                <div className="flex items-center gap-2">
                                  <span className="text-xs text-muted-foreground w-20">Auth:</span>
                                  <Badge variant="outline" className="text-xs capitalize">{handler.httpConfig.authType}</Badge>
                                </div>
                              )}
                              {handler.timeout && (
                                <div className="flex items-center gap-2">
                                  <span className="text-xs text-muted-foreground w-20">Timeout:</span>
                                  <span className="text-xs">{handler.timeout}</span>
                                </div>
                              )}
                              {handler.retries !== undefined && (
                                <div className="flex items-center gap-2">
                                  <span className="text-xs text-muted-foreground w-20">Retries:</span>
                                  <span className="text-xs">{handler.retries}</span>
                                </div>
                              )}
                            </div>
                          </div>

                          {/* Input/Output Schema if defined */}
                          {handler.tool?.inputSchema != null && (
                            <div className="space-y-2">
                              <h4 className="text-sm font-medium flex items-center gap-2">
                                <FileJson className="h-3.5 w-3.5" />
                                Input Schema
                              </h4>
                              <pre className="text-xs bg-muted p-3 rounded overflow-auto max-h-40">
                                {JSON.stringify(handler.tool.inputSchema, null, 2)}
                              </pre>
                            </div>
                          )}
                        </AccordionContent>
                      </AccordionItem>
                    );
                  })}
                </Accordion>
              </CardContent>
            </Card>
          </TabsContent>

          {/* Usage Tab */}
          <TabsContent value="usage" className="space-y-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Used By</CardTitle>
                <CardDescription>
                  Agents that reference this ToolRegistry
                </CardDescription>
              </CardHeader>
              <CardContent>
                {!usedByAgents || usedByAgents.length === 0 ? (
                  <p className="text-sm text-muted-foreground text-center py-8">
                    No agents are currently using this ToolRegistry
                  </p>
                ) : (
                  <div className="space-y-3">
                    {usedByAgents.map((agent) => (
                      <Link
                        key={agent.metadata.uid}
                        href={`/agents/${agent.metadata.name}?namespace=${agent.metadata.namespace}`}
                        className="flex items-center justify-between p-3 rounded-lg border hover:bg-muted/50 transition-colors"
                      >
                        <div className="flex items-center gap-3">
                          <Bot className="h-4 w-4 text-muted-foreground" />
                          <div>
                            <span className="font-medium text-sm">{agent.metadata.name}</span>
                            <p className="text-xs text-muted-foreground">
                              {agent.metadata.namespace}
                            </p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <StatusBadge phase={agent.status?.phase} />
                          <ExternalLink className="h-4 w-4 text-muted-foreground" />
                        </div>
                      </Link>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          {/* Config Tab */}
          <TabsContent value="config" className="space-y-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Full Configuration</CardTitle>
                <CardDescription>
                  Complete YAML specification for this ToolRegistry
                </CardDescription>
              </CardHeader>
              <CardContent>
                <YamlBlock data={registry} />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
