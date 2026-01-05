"use client";

import { use, useCallback } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, BarChart3, ExternalLink, FileText, MessageSquare } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Header } from "@/components/layout";
import { StatusBadge, ScaleControl } from "@/components/agents";
import { AgentConsole } from "@/components/console";
import { LogViewer } from "@/components/logs";
import { CostSummary, TokenUsageChart } from "@/components/cost";
import { getMockAgentUsage } from "@/lib/mock-data";
import { scaleAgent } from "@/lib/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { YamlBlock } from "@/components/ui/yaml-block";
import { useAgent } from "@/hooks";

interface AgentDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

export default function AgentDetailPage({ params }: AgentDetailPageProps) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const namespace = searchParams.get("namespace") || "production";
  const queryClient = useQueryClient();

  const { data: agent, isLoading } = useAgent(name, namespace);

  const handleScale = useCallback(async (replicas: number) => {
    await scaleAgent(namespace, name, replicas);
    // Invalidate queries to refresh data
    await queryClient.invalidateQueries({ queryKey: ["agent", namespace, name] });
    await queryClient.invalidateQueries({ queryKey: ["agents"] });
  }, [namespace, name, queryClient]);

  if (isLoading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Agent Details" />
        <div className="flex-1 p-6 space-y-6">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-[400px] rounded-lg" />
        </div>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Agent Not Found" />
        <div className="flex-1 p-6">
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">
              Agent &quot;{name}&quot; not found in namespace &quot;{namespace}&quot;
            </p>
            <Link href="/agents">
              <Button variant="outline" className="mt-4">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Agents
              </Button>
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const { metadata, spec, status } = agent;

  return (
    <div className="flex flex-col h-full">
      <Header
        title={metadata.name}
        description={`${metadata.namespace} namespace`}
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Back link and actions */}
        <div className="flex items-center justify-between">
          <Link href="/agents">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Agents
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            <StatusBadge phase={status?.phase} />
            <Button variant="outline" size="sm">
              <ExternalLink className="mr-2 h-4 w-4" />
              View in K8s
            </Button>
          </div>
        </div>

        {/* Tabs */}
        <Tabs defaultValue="overview">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="console" className="gap-1.5">
              <MessageSquare className="h-4 w-4" />
              Console
            </TabsTrigger>
            <TabsTrigger value="logs" className="gap-1.5">
              <FileText className="h-4 w-4" />
              Logs
            </TabsTrigger>
            <TabsTrigger value="usage" className="gap-1.5">
              <BarChart3 className="h-4 w-4" />
              Usage
            </TabsTrigger>
            <TabsTrigger value="config">Configuration</TabsTrigger>
            <TabsTrigger value="events">Events</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-4 mt-4">
            {/* Status Card */}
            <Card>
              <CardHeader>
                <CardTitle>Status</CardTitle>
                <CardDescription>Current state of the agent</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
                  <div>
                    <p className="text-sm text-muted-foreground">Phase</p>
                    <StatusBadge phase={status?.phase} className="mt-1" />
                  </div>
                  <div>
                    <ScaleControl
                      currentReplicas={status?.replicas?.ready ?? 0}
                      desiredReplicas={status?.replicas?.desired ?? spec.runtime?.replicas ?? 1}
                      minReplicas={spec.runtime?.autoscaling?.minReplicas ?? 0}
                      maxReplicas={spec.runtime?.autoscaling?.maxReplicas ?? 10}
                      autoscalingEnabled={spec.runtime?.autoscaling?.enabled ?? false}
                      autoscalingType={spec.runtime?.autoscaling?.type}
                      onScale={handleScale}
                    />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Active Version</p>
                    <p className="text-lg font-semibold">
                      {status?.activeVersion || "-"}
                    </p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Created</p>
                    <p className="text-sm font-medium">
                      {formatDate(metadata.creationTimestamp)}
                    </p>
                  </div>
                </div>

                {status?.conditions && status.conditions.length > 0 && (
                  <>
                    <Separator className="my-4" />
                    <div>
                      <p className="text-sm font-medium mb-2">Conditions</p>
                      <div className="space-y-2">
                        {status.conditions.map((condition, index) => (
                          <div
                            key={index}
                            className="flex items-start gap-4 text-sm"
                          >
                            <span
                              className={`px-2 py-0.5 rounded text-xs font-medium ${
                                condition.status === "True"
                                  ? "bg-green-500/15 text-green-700 dark:text-green-400"
                                  : "bg-red-500/15 text-red-700 dark:text-red-400"
                              }`}
                            >
                              {condition.type}
                            </span>
                            <div className="flex-1">
                              <p className="font-medium">{condition.reason}</p>
                              {condition.message && (
                                <p className="text-muted-foreground">
                                  {condition.message}
                                </p>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </>
                )}
              </CardContent>
            </Card>

            {/* Spec Summary Cards */}
            <div className="grid md:grid-cols-2 gap-4">
              <Card>
                <CardHeader>
                  <CardTitle>Framework</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Type</span>
                    <span className="font-medium capitalize">{spec.framework?.type || "promptkit"}</span>
                  </div>
                  {spec.framework?.version && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Version</span>
                      <span className="font-medium">{spec.framework.version}</span>
                    </div>
                  )}
                  {spec.framework?.image && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Image</span>
                      <span className="font-medium text-xs truncate max-w-[150px]" title={spec.framework.image}>
                        {spec.framework.image}
                      </span>
                    </div>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Provider</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Type</span>
                    <span className="font-medium capitalize">{spec.provider?.type || "claude"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Model</span>
                    <span className="font-medium">{spec.provider?.model || "claude-sonnet-4-20250514"}</span>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Facade</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Type</span>
                    <span className="font-medium capitalize">{spec.facade?.type || "websocket"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Port</span>
                    <span className="font-medium">{spec.facade?.port || 8080}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Handler</span>
                    <span className="font-medium">{spec.facade?.handler || "runtime"}</span>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>PromptPack</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Name</span>
                    <Link
                      href={`/promptpacks/${spec.promptPackRef?.name}`}
                      className="font-medium text-primary hover:underline"
                    >
                      {spec.promptPackRef?.name}
                    </Link>
                  </div>
                  {spec.promptPackRef?.version && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Version</span>
                      <span className="font-medium">{spec.promptPackRef.version}</span>
                    </div>
                  )}
                  {spec.promptPackRef?.track && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Track</span>
                      <span className="font-medium">{spec.promptPackRef.track}</span>
                    </div>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Session</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Type</span>
                    <span className="font-medium capitalize">{spec.session?.type || "memory"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">TTL</span>
                    <span className="font-medium">{spec.session?.ttl || "1h"}</span>
                  </div>
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          <TabsContent value="console" className="mt-4">
            <AgentConsole
              agentName={metadata.name}
              namespace={metadata.namespace || "default"}
            />
          </TabsContent>

          <TabsContent value="logs" className="mt-4">
            <LogViewer
              agentName={metadata.name}
              namespace={metadata.namespace || "default"}
            />
          </TabsContent>

          <TabsContent value="usage" className="space-y-6 mt-4">
            {(() => {
              const usage = getMockAgentUsage(
                metadata.namespace || "default",
                metadata.name || ""
              );

              if (!usage || usage.requestCount === 0) {
                return (
                  <Card>
                    <CardContent className="py-12">
                      <p className="text-muted-foreground text-center">
                        No usage data available for this agent yet.
                      </p>
                    </CardContent>
                  </Card>
                );
              }

              return (
                <>
                  <CostSummary
                    inputTokens={usage.inputTokens}
                    outputTokens={usage.outputTokens}
                    cacheHits={usage.cacheHits}
                    model={usage.model}
                    period={usage.period}
                  />

                  <div className="grid lg:grid-cols-2 gap-6">
                    <TokenUsageChart
                      data={usage.timeSeries}
                      title="Token Usage (24h)"
                      description="Input and output tokens over the last 24 hours"
                    />

                    <Card>
                      <CardHeader>
                        <CardTitle className="text-base">Performance</CardTitle>
                        <CardDescription>Request metrics for this agent</CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-4">
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Total Requests</span>
                          <span className="font-medium">{usage.requestCount.toLocaleString()}</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Error Count</span>
                          <span className="font-medium text-destructive">
                            {usage.errorCount.toLocaleString()}
                          </span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Error Rate</span>
                          <span className="font-medium">
                            {((usage.errorCount / usage.requestCount) * 100).toFixed(2)}%
                          </span>
                        </div>
                        <Separator />
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Avg Latency</span>
                          <span className="font-medium">{usage.avgLatencyMs.toLocaleString()}ms</span>
                        </div>
                        <div className="flex justify-between">
                          <span className="text-muted-foreground">Cache Hits</span>
                          <span className="font-medium">
                            {usage.cacheHits > 0
                              ? `${((usage.cacheHits / usage.inputTokens) * 100).toFixed(1)}%`
                              : "N/A"}
                          </span>
                        </div>
                      </CardContent>
                    </Card>
                  </div>
                </>
              );
            })()}
          </TabsContent>

          <TabsContent value="config" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Full Configuration</CardTitle>
                <CardDescription>Complete agent specification in YAML format</CardDescription>
              </CardHeader>
              <CardContent>
                <YamlBlock data={agent} className="max-h-[600px]" />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="events" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Recent Events</CardTitle>
                <CardDescription>Kubernetes events for this agent</CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-center py-8">
                  Event streaming will be available with K8s integration
                </p>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
