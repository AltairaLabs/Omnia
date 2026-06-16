"use client";

import { use, useCallback } from "react";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, FileText, MessageSquare, Activity, ShieldCheck, Workflow } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Header } from "@/components/layout";
import { StatusBadge, ScaleControl, AgentMetricsPanel, EventsPanel, EvalConfigPanel, AgentQualityTab, RolloutPanel, AgentTopology, AgentConditions } from "@/components/agents";
import { SystemPackBadge } from "@/components/agents/system-pack-badge";
import { AgentWorkloadTab } from "@/components/workload-graph/agent-workload-tab";
import { AgentConsole } from "@/components/console";
import { LogViewer } from "@/components/logs";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { YamlBlock } from "@/components/ui/yaml-block";
import { useAgent } from "@/hooks/agents";
import { usePromptPack } from "@/hooks/resources";
import { useAgentRolloutStream } from "@/hooks/use-agent-rollout-stream";

interface AgentDetailPageProps {
  params: Promise<{ name: string }>;
}

export default function AgentDetailPage({ params }: Readonly<AgentDetailPageProps>) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const { currentWorkspace } = useWorkspace();
  // Use workspace name for API calls (not K8s namespace)
  const workspace = currentWorkspace?.name || "demo";
  const currentTab = searchParams.get("tab") || "overview";
  const queryClient = useQueryClient();
  const dataService = useDataService();

  const { data: agent, isLoading } = useAgent(name, workspace);
  const { data: promptPack } = usePromptPack(agent?.spec?.promptPackRef?.name || "", workspace);
  // Live rollout via SSE: stays connected while the page is open, so it catches
  // a rollout starting (and lingers the terminal state) without a refresh.
  const liveRollout = useAgentRolloutStream(workspace, name);

  const handleTabChange = useCallback((tab: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", tab);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }, [searchParams, router, pathname]);

  const handleScale = useCallback(async (replicas: number) => {
    await dataService.scaleAgent(workspace, name, replicas);
    // Invalidate queries to refresh data
    await queryClient.invalidateQueries({ queryKey: ["agent", workspace, name] });
    await queryClient.invalidateQueries({ queryKey: ["agents"] });
  }, [workspace, name, queryClient, dataService]);

  const refetchAgent = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["agent", workspace, name] });
  }, [workspace, name, queryClient]);

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
              Agent &quot;{name}&quot; not found in workspace &quot;{workspace}&quot;
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

  // Once the SSE stream has any frame it is authoritative (catches start/finish
  // live); until it connects, fall back to the initial fetched agent.
  const rolloutSpec = liveRollout ? liveRollout.spec : spec.rollout ?? null;
  const rolloutStatus = liveRollout ? liveRollout.status : status?.rollout ?? null;

  return (
    <div className="flex flex-col h-full">
      <Header
        title={metadata.name}
        description={`${metadata.namespace} namespace`}
      />

      <div className="flex-1 min-h-0 overflow-y-auto p-6 space-y-6">
        {/* Back link and actions */}
        <div className="flex items-center justify-between">
          <Link href="/agents">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Agents
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            <SystemPackBadge
              labels={metadata.labels}
              annotations={metadata.annotations}
            />
            <StatusBadge phase={status?.phase} />
          </div>
        </div>

        {/* Tabs */}
        <Tabs value={currentTab} onValueChange={handleTabChange}>
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="workload" className="gap-1.5">
              <Workflow className="h-4 w-4" />
              Workload
            </TabsTrigger>
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
            <TabsTrigger value="quality" className="gap-1.5">
              <ShieldCheck className="h-4 w-4" />
              Quality
            </TabsTrigger>
            <TabsTrigger value="config">Configuration</TabsTrigger>
            <TabsTrigger value="events">Events</TabsTrigger>
          </TabsList>

          <TabsContent value="workload" className="mt-4">
            <AgentWorkloadTab agent={agent} workspace={workspace} />
          </TabsContent>

          <TabsContent value="overview" className="space-y-4 mt-4">
            {/* Rollout panel — only when a progressive-delivery rollout is
                configured or in progress (live values from the SSE stream). */}
            {(rolloutSpec || rolloutStatus) && (
              <RolloutPanel spec={rolloutSpec ?? undefined} status={rolloutStatus ?? undefined} />
            )}

            {/* Architecture diagram — facade(s) in front of the runtime, which
                nests PromptPack, Session and Memory. PromptPack → Workload tab. */}
            <AgentTopology
              agentName={metadata.name}
              facades={[{ type: spec.facade?.type ?? "websocket", port: spec.facade?.port }]}
              framework={spec.framework}
              promptPack={{
                name: spec.promptPackRef?.name,
                version: promptPack?.spec?.version || spec.promptPackRef?.version,
              }}
              session={spec.session}
              memoryEnabled={spec.memory?.enabled}
            />

            {/* Compact scale control (with autoscaling indicator) + thin metadata,
                under the diagram — replaces the bulky Status card. */}
            <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-muted-foreground">Replicas</span>
                <ScaleControl
                  compact
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
              <p className="text-xs text-muted-foreground">
                {status?.activeVersion ? `Active version ${status.activeVersion} · ` : ""}
                Created {metadata.creationTimestamp ? new Date(metadata.creationTimestamp).toLocaleString() : "-"}
              </p>
            </div>

            <EvalConfigPanel
              agentName={metadata.name}
              frameworkType={spec.framework?.type || "promptkit"}
              evalsEnabled={spec.evals?.enabled}
              sampling={spec.evals?.sampling}
              inlineGroups={spec.evals?.inline?.groups}
              workerGroups={spec.evals?.worker?.groups}
              promptPackName={spec.promptPackRef?.name}
            />

            {/* Conditions — full table, compact, at the bottom as reference. */}
            <AgentConditions conditions={status?.conditions} />
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
              workspace={workspace}
              resourceName={metadata.name}
            />
          </TabsContent>

          <TabsContent value="metrics" className="mt-4">
            <AgentMetricsPanel
              agentName={metadata.name}
              namespace={metadata.namespace || "default"}
            />
          </TabsContent>

          <TabsContent value="quality" className="mt-4">
            <AgentQualityTab agentName={metadata.name} />
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
              workspace={workspace}
            />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
