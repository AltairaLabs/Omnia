"use client";

import { use } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Bot, GitBranch, Clock, FileCode, Wrench, MessageSquare, FileText, Shield, Variable } from "lucide-react";
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
import type { PromptDefinition } from "@/lib/data/types";

interface PromptPackDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

// Helper to convert prompts Record to array for iteration
function promptsToArray(prompts?: Record<string, PromptDefinition>): Array<PromptDefinition & { id: string }> {
  if (!prompts) return [];
  return Object.entries(prompts).map(([id, prompt]) => ({ ...prompt, id }));
}

export default function PromptPackDetailPage({ params }: Readonly<PromptPackDetailPageProps>) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const namespace = searchParams.get("namespace") || "production";

  const { data: promptPack, isLoading } = usePromptPack(name, namespace);
  const { data: packContent, isLoading: isContentLoading } = usePromptPackContent(name, namespace);
  const { data: allAgents } = useAgents();

  // Convert prompts Record to array for iteration
  const promptsArray = promptsToArray(packContent?.prompts);
  const toolsArray = packContent?.tools || [];
  const fragmentsEntries = Object.entries(packContent?.fragments || {});
  const validatorsArray = packContent?.validators || [];

  // Find agents that reference this PromptPack
  // LocalObjectReference only contains name, so agents reference promptpacks in their own namespace
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
                    </div>
                  </CardContent>
                </Card>

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

                              {/* Validators */}
                              {prompt.validators && prompt.validators.length > 0 && (
                                <div>
                                  <p className="text-sm font-medium mb-2 flex items-center gap-1.5">
                                    <Shield className="h-4 w-4" />
                                    Validators ({prompt.validators.length})
                                  </p>
                                  <div className="flex flex-wrap gap-1">
                                    {prompt.validators.map((v) => (
                                      <Badge key={v} variant="outline" className="text-xs border-green-500/30 text-green-600 dark:text-green-400">{v}</Badge>
                                    ))}
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

                {/* Validators Section */}
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
                      <div className="space-y-3">
                        {validatorsArray.map((validator) => (
                          <div key={validator.id} className="border rounded-lg p-3">
                            <div className="flex items-center justify-between mb-2">
                              <div className="flex items-center gap-2">
                                <span className="font-medium">{validator.name || validator.id}</span>
                                <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{validator.id}</code>
                              </div>
                              {validator.config?.action ? (
                                <Badge variant="outline" className="text-xs">{String(validator.config.action)}</Badge>
                              ) : null}
                            </div>
                            {validator.description && (
                              <p className="text-sm text-muted-foreground mb-2">{validator.description}</p>
                            )}
                            {validator.config?.detect && Array.isArray(validator.config.detect) ? (
                              <div className="flex flex-wrap gap-1">
                                {(validator.config.detect as string[]).map((item) => (
                                  <Badge key={item} variant="secondary" className="text-xs">{item}</Badge>
                                ))}
                              </div>
                            ) : null}
                            {validator.config?.severity ? (
                              <div className="text-xs text-muted-foreground">
                                Severity: <span className="font-medium">{String(validator.config.severity)}</span>
                              </div>
                            ) : null}
                          </div>
                        ))}
                      </div>
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
