"use client";

import { use, useCallback } from "react";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, FileText, MessageSquare, Activity } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Header } from "@/components/layout";
import { StatusBadge, ScaleControl, AgentMetricsPanel, EventsPanel } from "@/components/agents";
import { AgentConsole } from "@/components/console";
import { LogViewer } from "@/components/logs";
import { useDataService } from "@/lib/data";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { YamlBlock } from "@/components/ui/yaml-block";
import { useAgent, useProvider, usePromptPack } from "@/hooks";

interface AgentDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

export default function AgentDetailPage({ params }: Readonly<AgentDetailPageProps>) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const namespace = searchParams.get("namespace") || "production";
  const currentTab = searchParams.get("tab") || "overview";
  const queryClient = useQueryClient();
  const dataService = useDataService();

  const { data: agent, isLoading } = useAgent(name, namespace);
  const { data: provider } = useProvider(agent?.spec?.providerRef?.name, namespace);
  const { data: promptPack } = usePromptPack(agent?.spec?.promptPackRef?.name || "", namespace);

  const handleTabChange = useCallback((tab: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", tab);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }, [searchParams, router, pathname]);

  const handleScale = useCallback(async (replicas: number) => {
    await dataService.scaleAgent(namespace, name, replicas);
    // Invalidate queries to refresh data
    await queryClient.invalidateQueries({ queryKey: ["agent", namespace, name] });
    await queryClient.invalidateQueries({ queryKey: ["agents"] });
  }, [namespace, name, queryClient, dataService]);

  const refetchAgent = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["agent", namespace, name] });
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
          <StatusBadge phase={status?.phase} />
        </div>

        {/* Tabs */}
        <Tabs value={currentTab} onValueChange={handleTabChange}>
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
            <TabsTrigger value="metrics" className="gap-1.5">
              <Activity className="h-4 w-4" />
              Metrics
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
                      refetch={refetchAgent}
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
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="text-left text-muted-foreground border-b">
                            <th className="pb-2 font-medium">Type</th>
                            <th className="pb-2 font-medium">Reason</th>
                            <th className="pb-2 font-medium">Message</th>
                          </tr>
                        </thead>
                        <tbody>
                          {status.conditions.map((condition) => (
                            <tr key={condition.type} className="border-b last:border-0">
                              <td className="py-2 pr-4">
                                <span
                                  className={`px-2 py-0.5 rounded text-xs font-medium ${
                                    condition.status === "True"
                                      ? "bg-green-500/15 text-green-700 dark:text-green-400"
                                      : "bg-red-500/15 text-red-700 dark:text-red-400"
                                  }`}
                                >
                                  {condition.type}
                                </span>
                              </td>
                              <td className="py-2 pr-4 font-medium">
                                {condition.reason}
                              </td>
                              <td className="py-2 text-muted-foreground">
                                {condition.message || "—"}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
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

              {spec.providerRef?.name ? (
                <Card>
                  <CardHeader>
                    <CardTitle>Provider</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Name</span>
                      <Link
                        href={`/providers/${spec.providerRef.name}?namespace=${spec.providerRef.namespace || namespace}`}
                        className="font-medium text-primary hover:underline"
                      >
                        {spec.providerRef.name}
                      </Link>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Type</span>
                      <span className="font-medium capitalize">{provider?.spec?.type || spec.provider?.type || "-"}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Model</span>
                      <span className="font-medium">{provider?.spec?.model || spec.provider?.model || "-"}</span>
                    </div>
                  </CardContent>
                </Card>
              ) : (
                <Card>
                  <CardHeader>
                    <CardTitle>Provider</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Name</span>
                      <span className="font-medium">-</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Type</span>
                      <span className="font-medium capitalize">{spec.provider?.type || "-"}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Model</span>
                      <span className="font-medium">{spec.provider?.model || "-"}</span>
                    </div>
                  </CardContent>
                </Card>
              )}

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
                      href={`/promptpacks/${spec.promptPackRef?.name}?namespace=${namespace}`}
                      className="font-medium text-primary hover:underline"
                    >
                      {spec.promptPackRef?.name || "-"}
                    </Link>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Version</span>
                    <span className="font-medium">{promptPack?.spec?.version || spec.promptPackRef?.version || "-"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Phase</span>
                    <span className="font-medium">{promptPack?.status?.phase || "-"}</span>
                  </div>
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

          <TabsContent value="metrics" className="mt-4">
            <AgentMetricsPanel
              agentName={metadata.name}
              namespace={metadata.namespace || "default"}
            />
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
            <EventsPanel
              agentName={metadata.name}
              namespace={metadata.namespace || "default"}
            />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
