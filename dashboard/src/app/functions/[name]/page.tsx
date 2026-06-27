/**
 * /functions/[name] — Function detail page.
 *
 * A function is a `mode: function` AgentRuntime, so it reuses the agent
 * detail controls (architecture diagram, scale control, conditions, logs,
 * metrics, quality, events, config) assembled into function-flavoured tabs.
 * Function-specific tabs: Schema (pretty-printed input/output) and
 * Invocations (sessions tagged "function"). The Console tab is dropped — a
 * function testing tool replaces it in a follow-up.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useCallback } from "react";
import { useParams, useSearchParams, useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, FileText, Activity, ShieldCheck, Workflow, Braces, History, FlaskConical } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Header } from "@/components/layout";
import {
  StatusBadge,
  ScaleControl,
  AgentTopology,
  AgentConditions,
  EvalConfigPanel,
  AgentMetricsPanel,
  AgentQualityTab,
  EventsPanel,
} from "@/components/agents";
import { SystemPackBadge } from "@/components/agents/system-pack-badge";
import { AgentWorkloadTab } from "@/components/workload-graph/agent-workload-tab";
import { LogViewer } from "@/components/logs";
import { FunctionSessionsPanel } from "@/components/functions/function-sessions-panel";
import { FunctionTestPanel } from "@/components/functions/function-test-panel";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { JsonBlock } from "@/components/ui/json-block";
import { YamlBlock } from "@/components/ui/yaml-block";
import { useAgent } from "@/hooks/agents";
import { useWorkspace } from "@/contexts/workspace-context";
import { useDataService } from "@/lib/data";
import { isFunctionMode } from "@/types/agent-runtime";

export default function FunctionDetailPage() {
  const params = useParams<{ name: string }>();
  const functionName = params?.name ?? "";
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const queryClient = useQueryClient();
  const dataService = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name || "demo";
  const currentTab = searchParams.get("tab") || "overview";

  const { data: agent, isLoading } = useAgent(functionName, workspace);

  const handleTabChange = useCallback((tab: string) => {
    const next = new URLSearchParams(searchParams.toString());
    next.set("tab", tab);
    router.replace(`${pathname}?${next.toString()}`, { scroll: false });
  }, [searchParams, router, pathname]);

  const handleScale = useCallback(async (replicas: number) => {
    await dataService.scaleAgent(workspace, functionName, replicas);
    await queryClient.invalidateQueries({ queryKey: ["agent", workspace, functionName] });
  }, [workspace, functionName, queryClient, dataService]);

  const refetchAgent = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["agent", workspace, functionName] });
  }, [workspace, functionName, queryClient]);

  if (isLoading) {
    return (
      <div className="flex flex-col h-full">
        <Header title={functionName} />
        <div className="p-6">
          <Skeleton className="h-[200px] w-full rounded-lg" />
        </div>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex flex-col h-full">
        <Header title={functionName} description="Function not found" />
        <div className="p-6">
          <p className="text-muted-foreground">
            No AgentRuntime named <code className="font-mono">{functionName}</code> exists in
            this workspace.
          </p>
        </div>
      </div>
    );
  }

  if (!isFunctionMode(agent.spec)) {
    return (
      <div className="flex flex-col h-full">
        <Header title={functionName} description="This AgentRuntime is not a Function" />
        <div className="p-6">
          <p className="text-muted-foreground">
            <code className="font-mono">{functionName}</code> is mode{" "}
            <Badge variant="outline">{agent.spec.mode ?? "agent"}</Badge>. The functions view
            only surfaces runtimes with <code className="font-mono">spec.mode: function</code>.
          </p>
        </div>
      </div>
    );
  }

  const { metadata, spec, status } = agent;

  return (
    <div className="flex flex-col h-full">
      <Header title={metadata.name} description={`Function in ${metadata.namespace ?? "default"}`} />

      <div className="flex-1 min-h-0 overflow-y-auto p-6 space-y-6">
        <div className="flex items-center justify-between">
          <Link href="/functions">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Functions
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            <SystemPackBadge labels={metadata.labels} annotations={metadata.annotations} />
            <Badge variant="default">Function</Badge>
            <StatusBadge phase={status?.phase} />
          </div>
        </div>

        <Tabs value={currentTab} onValueChange={handleTabChange}>
          {/* Horizontal scroll instead of wrap when all ten tabs don't fit. */}
          <div className="max-w-full overflow-x-auto">
            <TabsList className="w-max">
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="schema" className="gap-1.5">
              <Braces className="h-4 w-4" />
              Schema
            </TabsTrigger>
            <TabsTrigger value="test" className="gap-1.5">
              <FlaskConical className="h-4 w-4" />
              Test
            </TabsTrigger>
            <TabsTrigger value="invocations" className="gap-1.5">
              <History className="h-4 w-4" />
              Invocations
            </TabsTrigger>
            <TabsTrigger value="workload" className="gap-1.5">
              <Workflow className="h-4 w-4" />
              Workload
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
          </div>

          <TabsContent value="overview" className="space-y-4 mt-4">
            <AgentTopology
              agentName={metadata.name}
              facades={
                spec.facades?.length
                  ? spec.facades.map(f => ({ type: f.type, port: f.port }))
                  : [{ type: "websocket" }]
              }
              framework={spec.framework}
              promptPack={{ name: spec.promptPackRef?.name, version: spec.promptPackRef?.version }}
              context={spec.context}
              memoryEnabled={spec.memory?.enabled}
            />

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

            <AgentConditions conditions={status?.conditions} />
          </TabsContent>

          <TabsContent value="schema" className="mt-4">
            <div className="grid gap-4 md:grid-cols-2">
              <SchemaCard label="Input schema" schema={spec.inputSchema} />
              <SchemaCard label="Output schema" schema={spec.outputSchema} />
            </div>
          </TabsContent>

          {/* forceMount keeps the panel mounted across tab switches so a typed
              input and last result survive a detour to Schema/Invocations. But
              forceMount makes Radix render the content with hidden=false on
              EVERY tab, so we hide it ourselves with a wrapper that carries a
              plain `hidden` attribute unless the Test tab is active — no
              reliance on Radix's data-state or a Tailwind variant. */}
          <TabsContent value="test" className="mt-4" forceMount>
            <div hidden={currentTab !== "test"}>
              <FunctionTestPanel
                functionName={metadata.name}
                workspace={workspace}
                inputSchema={spec.inputSchema}
                outputSchema={spec.outputSchema}
                ready={status?.phase === "Running" && (status?.replicas?.ready ?? 0) > 0}
                unavailableReason={status?.phase ?? "Unknown"}
              />
            </div>
          </TabsContent>

          <TabsContent value="invocations" className="mt-4 space-y-3">
            <FunctionSessionsPanel functionName={functionName} />
            <p className="text-xs text-muted-foreground">
              Function invocations are recorded as sessions tagged{" "}
              <code className="font-mono">function</code>. Click a session id for the full
              transcript, tool calls, provider calls, and eval results.
            </p>
          </TabsContent>

          <TabsContent value="workload" className="mt-4">
            <AgentWorkloadTab agent={agent} workspace={workspace} />
          </TabsContent>

          <TabsContent value="logs" className="mt-4">
            <LogViewer agentName={metadata.name} workspace={workspace} resourceName={metadata.name} />
          </TabsContent>

          <TabsContent value="metrics" className="mt-4">
            <AgentMetricsPanel agentName={metadata.name} namespace={metadata.namespace || "default"} />
          </TabsContent>

          <TabsContent value="quality" className="mt-4">
            <AgentQualityTab agentName={metadata.name} />
          </TabsContent>

          <TabsContent value="config" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Full Configuration</CardTitle>
                <CardDescription>Complete function specification in YAML format</CardDescription>
              </CardHeader>
              <CardContent>
                <YamlBlock data={agent} className="max-h-[600px]" />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="events" className="mt-4">
            <EventsPanel agentName={metadata.name} workspace={workspace} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}

interface SchemaCardProps {
  label: string;
  schema?: Record<string, unknown>;
}

function SchemaCard({ label, schema }: Readonly<SchemaCardProps>) {
  return (
    <Card data-testid={`schema-card-${label.toLowerCase().replaceAll(/\s+/g, "-")}`}>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        {schema ? (
          <JsonBlock data={schema} className="max-h-[500px]" />
        ) : (
          <p className="text-sm text-muted-foreground">—</p>
        )}
      </CardContent>
    </Card>
  );
}
