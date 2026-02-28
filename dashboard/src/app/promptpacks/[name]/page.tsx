"use client";

import { use } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft, Bot, GitBranch, Clock, FileCode, Wrench,
  MessageSquare, FileText, Shield, Variable, Activity,
  Workflow, Sparkles, ArrowRight,
} from "lucide-react";
import { Header } from "@/components/layout";
import { StatusBadge } from "@/components/agents";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import { YamlBlock } from "@/components/ui/yaml-block";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { usePromptPack, usePromptPackContent, useAgents } from "@/hooks";
import { useWorkspaces } from "@/hooks/use-workspaces";
import type {
  PromptDefinition,
  ToolDefinition,
  ValidatorDefinition,
  EvalDefinition,
  ToolPolicy,
} from "@/lib/data/types";

interface PromptPackDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

// Helper to convert prompts Record to array for iteration
function promptsToArray(
  prompts?: Record<string, PromptDefinition>
): Array<PromptDefinition & { id: string }> {
  if (!prompts) return [];
  return Object.entries(prompts).map(([id, prompt]) => ({ ...prompt, id }));
}

// Helper to normalize tools (Record or array) to array
function toolsToArray(
  tools?: Record<string, ToolDefinition> | ToolDefinition[]
): ToolDefinition[] {
  if (!tools) return [];
  if (Array.isArray(tools)) return tools;
  return Object.values(tools);
}

// --- Sub-components for Content tab sections ---

function EvalsList({ evals, title }: { evals: EvalDefinition[]; title: string }) {
  return (
    <div>
      <p className="text-sm font-medium mb-2 flex items-center gap-1.5">
        <Activity className="h-4 w-4" />
        {title} ({evals.length})
      </p>
      <div className="space-y-2">
        {evals.map((ev) => (
          <div key={ev.id} className="border rounded-lg p-3">
            <div className="flex items-center gap-2 mb-1">
              <span className="font-medium text-sm">{ev.id}</span>
              <Badge variant="secondary" className="text-xs">{ev.type}</Badge>
              <Badge variant="outline" className="text-xs">{ev.trigger}</Badge>
              {ev.sample_percentage != null && (
                <span className="text-xs text-muted-foreground">
                  {ev.sample_percentage}% sample
                </span>
              )}
            </div>
            {ev.description && (
              <p className="text-xs text-muted-foreground mb-1">{ev.description}</p>
            )}
            {ev.metric && (
              <div className="text-xs text-muted-foreground">
                Metric: <code className="bg-muted px-1 rounded">{ev.metric.name}</code>
                {" "}({ev.metric.type})
                {ev.metric.range && ` [${ev.metric.range.min}-${ev.metric.range.max}]`}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

function ToolPolicySection({ policy }: { policy: ToolPolicy }) {
  return (
    <div>
      <p className="text-sm font-medium mb-2 flex items-center gap-1.5">
        <Shield className="h-4 w-4" />
        Tool Policy
      </p>
      <div className="text-xs space-y-1 bg-muted/50 rounded px-3 py-2">
        {policy.tool_choice && (
          <div className="flex justify-between">
            <span className="text-muted-foreground">Tool Choice</span>
            <span className="font-mono">{policy.tool_choice}</span>
          </div>
        )}
        {policy.max_rounds != null && (
          <div className="flex justify-between">
            <span className="text-muted-foreground">Max Rounds</span>
            <span className="font-mono">{policy.max_rounds}</span>
          </div>
        )}
        {policy.max_tool_calls_per_turn != null && (
          <div className="flex justify-between">
            <span className="text-muted-foreground">Max Calls/Turn</span>
            <span className="font-mono">{policy.max_tool_calls_per_turn}</span>
          </div>
        )}
      </div>
    </div>
  );
}

function ValidatorsList({ validators }: { validators: ValidatorDefinition[] }) {
  return (
    <div>
      <p className="text-sm font-medium mb-2 flex items-center gap-1.5">
        <Shield className="h-4 w-4" />
        Validators ({validators.length})
      </p>
      <div className="space-y-2">
        {validators.map((v, idx) => (
          <div key={v.id || v.type || idx} className="border rounded-lg p-2.5">
            <div className="flex items-center gap-2 mb-1">
              <Badge variant="secondary" className="text-xs">{v.type || v.id}</Badge>
              {v.enabled !== undefined && (
                <Badge
                  variant="outline"
                  className={`text-xs ${v.enabled ? "border-green-500/30 text-green-600" : "border-red-500/30 text-red-600"}`}
                >
                  {v.enabled ? "enabled" : "disabled"}
                </Badge>
              )}
              {v.fail_on_violation && (
                <Badge variant="outline" className="text-xs border-orange-500/30 text-orange-600">
                  fail on violation
                </Badge>
              )}
            </div>
            {v.description && (
              <p className="text-xs text-muted-foreground">{v.description}</p>
            )}
            {v.params && Object.keys(v.params).length > 0 && (
              <div className="text-xs mt-1 space-y-0.5">
                {Object.entries(v.params).map(([key, val]) => (
                  <div key={key} className="flex justify-between">
                    <span className="text-muted-foreground">{key}</span>
                    <span className="font-mono">{JSON.stringify(val)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

// --- Main page component ---

export default function PromptPackDetailPage({ params }: Readonly<PromptPackDetailPageProps>) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const namespaceParam = searchParams.get("namespace") || "production";

  const { data: workspaces } = useWorkspaces();
  const workspace = workspaces?.find(w => w.namespace === namespaceParam);
  const workspaceName = workspace?.name || namespaceParam;
  const namespace = workspace?.namespace || namespaceParam;

  const { data: promptPack, isLoading } = usePromptPack(name, workspaceName);
  const { data: packContent, isLoading: isContentLoading } = usePromptPackContent(name, workspaceName);
  const { data: allAgents } = useAgents();

  const promptsArray = promptsToArray(packContent?.prompts);
  const toolsArray = toolsToArray(packContent?.tools);
  const fragmentsEntries = Object.entries(packContent?.fragments || {});
  const validatorsArray = packContent?.validators || [];
  const packEvals = packContent?.evals || [];
  const workflowConfig = packContent?.workflow;
  const skillsArray = packContent?.skills || [];
  const workflowStatesCount = workflowConfig ? Object.keys(workflowConfig.states).length : 0;

  const usingAgents = allAgents?.filter(
    (agent) =>
      agent.spec.promptPackRef?.name === name &&
      agent.metadata.namespace === namespace
  );

  if (isLoading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="PromptPack Details" />
        <div className="flex-1 p-6 space-y-6">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-[400px] rounded-lg" />
        </div>
      </div>
    );
  }

  if (!promptPack) {
    return (
      <div className="flex flex-col h-full">
        <Header title="PromptPack Not Found" />
        <div className="flex-1 p-6">
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">
              PromptPack &quot;{name}&quot; not found in namespace &quot;{namespace}&quot;
            </p>
            <Link href="/promptpacks">
              <Button variant="outline" className="mt-4">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to PromptPacks
              </Button>
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const { metadata, spec, status } = promptPack;
  const isCanary = status?.phase === "Canary";

  return (
    <div className="flex flex-col h-full">
      <Header
        title={metadata.name}
        description={`${metadata.namespace} namespace`}
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Back link and actions */}
        <div className="flex items-center justify-between">
          <Link href="/promptpacks">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to PromptPacks
            </Button>
          </Link>
          <StatusBadge phase={status?.phase} />
        </div>

        {/* Tabs */}
        <Tabs defaultValue="overview">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="content" className="gap-1.5">
              <FileCode className="h-4 w-4" />
              Content
            </TabsTrigger>
            <TabsTrigger value="usage" className="gap-1.5">
              <Bot className="h-4 w-4" />
              Usage ({usingAgents?.length ?? 0})
            </TabsTrigger>
            <TabsTrigger value="config">Configuration</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-4 mt-4">
            {/* Status Card */}
            <Card>
              <CardHeader>
                <CardTitle>Status</CardTitle>
                <CardDescription>Current state of the PromptPack</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
                  <div>
                    <p className="text-sm text-muted-foreground">Phase</p>
                    <StatusBadge phase={status?.phase} className="mt-1" />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Active Version</p>
                    <div className="flex items-center gap-1.5 mt-1">
                      <GitBranch className="h-4 w-4 text-muted-foreground" />
                      <span className="text-lg font-semibold">
                        v{status?.activeVersion || spec.version}
                      </span>
                    </div>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Rollout Strategy</p>
                    <Badge variant="outline" className="mt-1 capitalize">
                      {spec.rollout.type}
                    </Badge>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Last Updated</p>
                    <div className="flex items-center gap-1.5 mt-1">
                      <Clock className="h-4 w-4 text-muted-foreground" />
                      <span className="text-sm font-medium">
                        {formatDate(status?.lastUpdated)}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Canary progress */}
                {isCanary && status?.canaryWeight !== undefined && (
                  <>
                    <Separator className="my-4" />
                    <div className="space-y-3">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="text-sm font-medium">Canary Rollout</p>
                          <p className="text-xs text-muted-foreground">
                            Rolling out v{status.canaryVersion} to replace v{status.activeVersion}
                          </p>
                        </div>
                        <div className="text-right">
                          <p className="text-2xl font-bold">{status.canaryWeight}%</p>
                          <p className="text-xs text-muted-foreground">traffic to canary</p>
                        </div>
                      </div>
                      <Progress value={status.canaryWeight} className="h-2" />
                      {spec.rollout.canary && (
                        <div className="flex gap-4 text-xs text-muted-foreground">
                          <span>Step: +{spec.rollout.canary.stepWeight}%</span>
                          <span>Interval: {spec.rollout.canary.interval}</span>
                        </div>
                      )}
                    </div>
                  </>
                )}
              </CardContent>
            </Card>

            {/* Source Card */}
            <Card>
              <CardHeader>
                <CardTitle>Source</CardTitle>
                <CardDescription>Configuration source reference</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Type</span>
                  <Badge variant="outline" className="capitalize">
                    {spec.source.type}
                  </Badge>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">ConfigMap</span>
                  <code className="bg-muted px-2 py-0.5 rounded text-sm">
                    {spec.source.configMapRef?.name}
                  </code>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Created</span>
                  <span className="text-sm">{formatDate(metadata.creationTimestamp)}</span>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="content" className="space-y-4 mt-4">
            {isContentLoading && (
              <div className="space-y-4">
                <Skeleton className="h-32 rounded-lg" />
                <Skeleton className="h-48 rounded-lg" />
              </div>
            )}
            {!isContentLoading && !packContent && (
              <Card>
                <CardContent className="py-8 text-center">
                  <p className="text-muted-foreground">
                    Content not available. The PromptPack may not have a ConfigMap source or the content could not be loaded.
                  </p>
                </CardContent>
              </Card>
            )}
            {!isContentLoading && packContent && (
              <>
                {/* Metadata Card */}
                <Card>
                  <CardHeader className="pb-3">
                    <div className="flex items-center justify-between">
                      <CardTitle className="text-base">Pack Metadata</CardTitle>
                      {packContent.version && <Badge variant="outline">v{packContent.version}</Badge>}
                    </div>
                    {packContent.description && <CardDescription>{packContent.description}</CardDescription>}
                  </CardHeader>
                  <CardContent>
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                      <div>
                        <p className="text-muted-foreground">ID</p>
                        <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{packContent.id || "-"}</code>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Template Engine</p>
                        <span className="font-medium">
                          {packContent.template_engine?.syntax || "-"}
                          {packContent.template_engine?.version && ` v${packContent.template_engine.version}`}
                        </span>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Prompts</p>
                        <span className="font-medium">{promptsArray.length}</span>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Tools</p>
                        <span className="font-medium">{toolsArray.length}</span>
                      </div>
                      {packEvals.length > 0 && (
                        <div>
                          <p className="text-muted-foreground">Pack Evals</p>
                          <span className="font-medium">{packEvals.length}</span>
                        </div>
                      )}
                      {skillsArray.length > 0 && (
                        <div>
                          <p className="text-muted-foreground">Skills</p>
                          <span className="font-medium">{skillsArray.length}</span>
                        </div>
                      )}
                      {workflowStatesCount > 0 && (
                        <div>
                          <p className="text-muted-foreground">Workflow States</p>
                          <span className="font-medium">{workflowStatesCount}</span>
                        </div>
                      )}
                    </div>
                  </CardContent>
                </Card>

                {/* Pack-level Evals */}
                {packEvals.length > 0 && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <Activity className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Pack Evals ({packEvals.length})</CardTitle>
                      </div>
                      <CardDescription>Pack-level evaluation definitions applied across all prompts</CardDescription>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <EvalsList evals={packEvals} title="Evaluations" />
                    </CardContent>
                  </Card>
                )}

                {/* Workflow Card */}
                {workflowConfig && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <Workflow className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Workflow</CardTitle>
                        {workflowConfig.version && (
                          <Badge variant="outline" className="text-xs">v{workflowConfig.version}</Badge>
                        )}
                      </div>
                      <CardDescription>
                        State machine with {workflowStatesCount} states, entry point:{" "}
                        <code className="bg-muted px-1 rounded">{workflowConfig.entry}</code>
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <div className="space-y-3">
                        {Object.entries(workflowConfig.states).map(([stateId, state]) => {
                          const isEntry = stateId === workflowConfig.entry;
                          return (
                            <div
                              key={stateId}
                              className={`border rounded-lg p-3 ${isEntry ? "border-primary/50 bg-primary/5" : ""}`}
                            >
                              <div className="flex items-center gap-2 mb-1">
                                <span className="font-medium text-sm">{stateId}</span>
                                {isEntry && (
                                  <Badge className="text-xs bg-primary/15 text-primary border-primary/20">
                                    entry
                                  </Badge>
                                )}
                                <Badge variant="outline" className="text-xs">
                                  {state.prompt_task}
                                </Badge>
                                {state.persistence && (
                                  <Badge variant="secondary" className="text-xs">
                                    {state.persistence}
                                  </Badge>
                                )}
                              </div>
                              {state.description && (
                                <p className="text-xs text-muted-foreground mb-2">{state.description}</p>
                              )}
                              {state.on_event && Object.keys(state.on_event).length > 0 && (
                                <div className="flex flex-wrap gap-2 mt-1">
                                  {Object.entries(state.on_event).map(([event, target]) => (
                                    <div key={event} className="flex items-center gap-1 text-xs">
                                      <Badge variant="outline" className="text-xs">{event}</Badge>
                                      <ArrowRight className="h-3 w-3 text-muted-foreground" />
                                      <code className="bg-muted px-1 rounded">{target}</code>
                                    </div>
                                  ))}
                                </div>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    </CardContent>
                  </Card>
                )}

                {/* Skills Card */}
                {skillsArray.length > 0 && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <Sparkles className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Skills ({skillsArray.length})</CardTitle>
                      </div>
                      <CardDescription>Inline skill definitions available to agents</CardDescription>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <Accordion type="multiple" className="w-full">
                        {skillsArray.map((skill) => (
                          <AccordionItem key={skill.name} value={skill.name}>
                            <AccordionTrigger className="hover:no-underline">
                              <div className="flex items-center gap-2">
                                <span className="font-medium">{skill.name}</span>
                              </div>
                            </AccordionTrigger>
                            <AccordionContent className="space-y-2">
                              {skill.description && (
                                <p className="text-sm text-muted-foreground">{skill.description}</p>
                              )}
                              {skill.instructions && (
                                <div className="bg-muted rounded-lg p-3 font-mono text-xs whitespace-pre-wrap max-h-[200px] overflow-auto">
                                  {skill.instructions}
                                </div>
                              )}
                            </AccordionContent>
                          </AccordionItem>
                        ))}
                      </Accordion>
                    </CardContent>
                  </Card>
                )}

                {/* Prompts Section */}
                {promptsArray.length > 0 && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <MessageSquare className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Prompts ({promptsArray.length})</CardTitle>
                      </div>
                      <CardDescription>Specialized prompts for different scenarios</CardDescription>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <Accordion type="multiple" className="w-full">
                        {promptsArray.map((prompt) => (
                          <AccordionItem key={prompt.id} value={prompt.id}>
                            <AccordionTrigger className="hover:no-underline">
                              <div className="flex items-center gap-3">
                                <span className="font-medium">{prompt.name || prompt.id}</span>
                                {prompt.version && <Badge variant="secondary" className="text-xs">v{prompt.version}</Badge>}
                                {prompt.description && (
                                  <span className="text-xs text-muted-foreground hidden md:inline">{prompt.description}</span>
                                )}
                              </div>
                            </AccordionTrigger>
                            <AccordionContent className="space-y-4">
                              {/* System Template */}
                              {prompt.system_template && (
                                <div>
                                  <p className="text-sm font-medium mb-2">System Template</p>
                                  <div className="bg-muted rounded-lg p-3 font-mono text-xs whitespace-pre-wrap max-h-[200px] overflow-auto">
                                    {prompt.system_template}
                                  </div>
                                </div>
                              )}

                              {/* Variables */}
                              {prompt.variables && prompt.variables.length > 0 && (
                                <div>
                                  <p className="text-sm font-medium mb-2 flex items-center gap-1.5">
                                    <Variable className="h-4 w-4" />
                                    Variables ({prompt.variables.length})
                                  </p>
                                  <div className="grid gap-2">
                                    {prompt.variables.map((v) => (
                                      <div key={v.name} className="flex items-center gap-2 text-xs bg-muted/50 rounded px-2 py-1.5">
                                        <code className="text-primary font-medium">{`{{${v.name}}}`}</code>
                                        <Badge variant="outline" className="text-xs">{v.type}</Badge>
                                        {v.required && <Badge className="text-xs bg-red-500/15 text-red-600 border-red-500/20">required</Badge>}
                                        {v.values && (
                                          <span className="text-muted-foreground">
                                            [{v.values.join(", ")}]
                                          </span>
                                        )}
                                        {v.description && (
                                          <span className="text-muted-foreground">{v.description}</span>
                                        )}
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              )}

                              {/* Tools & Parameters */}
                              <div className="grid md:grid-cols-2 gap-4">
                                {prompt.tools && prompt.tools.length > 0 && (
                                  <div>
                                    <p className="text-sm font-medium mb-2 flex items-center gap-1.5">
                                      <Wrench className="h-4 w-4" />
                                      Tools ({prompt.tools.length})
                                    </p>
                                    <div className="flex flex-wrap gap-1">
                                      {prompt.tools.map((tool) => (
                                        <Badge key={tool} variant="outline" className="text-xs">{tool}</Badge>
                                      ))}
                                    </div>
                                  </div>
                                )}
                                {prompt.parameters && Object.keys(prompt.parameters).length > 0 && (
                                  <div>
                                    <p className="text-sm font-medium mb-2">Parameters</p>
                                    <div className="text-xs space-y-1">
                                      {Object.entries(prompt.parameters).map(([key, value]) => (
                                        <div key={key} className="flex justify-between">
                                          <span className="text-muted-foreground capitalize">{key.replaceAll("_", " ")}</span>
                                          <span className="font-mono">{String(value)}</span>
                                        </div>
                                      ))}
                                    </div>
                                  </div>
                                )}
                              </div>

                              {/* Tool Policy */}
                              {prompt.tool_policy && (
                                <ToolPolicySection policy={prompt.tool_policy} />
                              )}

                              {/* Prompt-level Validators */}
                              {prompt.validators && prompt.validators.length > 0 && (
                                <ValidatorsList validators={prompt.validators} />
                              )}

                              {/* Prompt-level Evals */}
                              {prompt.evals && prompt.evals.length > 0 && (
                                <EvalsList evals={prompt.evals} title="Evals" />
                              )}
                            </AccordionContent>
                          </AccordionItem>
                        ))}
                      </Accordion>
                    </CardContent>
                  </Card>
                )}

                {/* Tools Section */}
                {toolsArray.length > 0 && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <Wrench className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Tools ({toolsArray.length})</CardTitle>
                      </div>
                      <CardDescription>Shared tool definitions available to all prompts</CardDescription>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <Accordion type="multiple" className="w-full">
                        {toolsArray.map((tool) => (
                          <AccordionItem key={tool.name} value={tool.name}>
                            <AccordionTrigger className="hover:no-underline">
                              <div className="flex items-center gap-2">
                                <code className="text-sm font-medium">{tool.name}</code>
                              </div>
                            </AccordionTrigger>
                            <AccordionContent>
                              {tool.description && (
                                <p className="text-sm text-muted-foreground mb-3">{tool.description}</p>
                              )}
                              {tool.parameters && (
                                <div>
                                  <p className="text-sm font-medium mb-2">Parameters</p>
                                  <div className="bg-muted rounded-lg p-3 font-mono text-xs overflow-auto">
                                    <pre>{JSON.stringify(tool.parameters, null, 2)}</pre>
                                  </div>
                                </div>
                              )}
                            </AccordionContent>
                          </AccordionItem>
                        ))}
                      </Accordion>
                    </CardContent>
                  </Card>
                )}

                {/* Fragments Section */}
                {fragmentsEntries.length > 0 && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <FileText className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Fragments ({fragmentsEntries.length})</CardTitle>
                      </div>
                      <CardDescription>Reusable text blocks shared across prompts</CardDescription>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <Accordion type="multiple" className="w-full">
                        {fragmentsEntries.map(([key, value]) => (
                          <AccordionItem key={key} value={key}>
                            <AccordionTrigger className="hover:no-underline">
                              <code className="text-sm font-medium">{key}</code>
                            </AccordionTrigger>
                            <AccordionContent>
                              <div className="bg-muted rounded-lg p-3 font-mono text-xs whitespace-pre-wrap">
                                {value}
                              </div>
                            </AccordionContent>
                          </AccordionItem>
                        ))}
                      </Accordion>
                    </CardContent>
                  </Card>
                )}

                {/* Validators Section (pack-level) */}
                {validatorsArray.length > 0 && (
                  <Card>
                    <CardHeader className="pb-3">
                      <div className="flex items-center gap-2">
                        <Shield className="h-5 w-5 text-muted-foreground" />
                        <CardTitle className="text-base">Validators ({validatorsArray.length})</CardTitle>
                      </div>
                      <CardDescription>Safety guardrails and content filters</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <ValidatorsList validators={validatorsArray} />
                    </CardContent>
                  </Card>
                )}
              </>
            )}
          </TabsContent>

          <TabsContent value="usage" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Agents Using This PromptPack</CardTitle>
                <CardDescription>
                  {usingAgents?.length || 0} agent(s) reference this PromptPack
                </CardDescription>
              </CardHeader>
              <CardContent>
                {usingAgents && usingAgents.length > 0 ? (
                  <div className="space-y-2">
                    {usingAgents.map((agent) => (
                      <Link
                        key={agent.metadata.uid}
                        href={`/agents/${agent.metadata.name}?namespace=${agent.metadata.namespace}`}
                        className="flex items-center justify-between p-3 rounded-lg border hover:bg-muted transition-colors"
                      >
                        <div className="flex items-center gap-3">
                          <Bot className="h-5 w-5 text-muted-foreground" />
                          <div>
                            <p className="font-medium">{agent.metadata.name}</p>
                            <p className="text-xs text-muted-foreground">
                              {agent.metadata.namespace}
                            </p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          {agent.spec.promptPackRef?.version && (
                            <Badge variant="secondary" className="text-xs">
                              v{agent.spec.promptPackRef.version}
                            </Badge>
                          )}
                          {agent.spec.promptPackRef?.track && (
                            <Badge variant="outline" className="text-xs">
                              {agent.spec.promptPackRef.track}
                            </Badge>
                          )}
                          <StatusBadge phase={agent.status?.phase} />
                        </div>
                      </Link>
                    ))}
                  </div>
                ) : (
                  <p className="text-muted-foreground text-center py-8">
                    No agents are currently using this PromptPack
                  </p>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="config" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Full Configuration</CardTitle>
                <CardDescription>Complete PromptPack specification in YAML format</CardDescription>
              </CardHeader>
              <CardContent>
                <YamlBlock data={promptPack} className="max-h-[600px]" />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
