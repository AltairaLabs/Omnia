"use client";

import { use } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ExternalLink, Bot, GitBranch, Clock, FileCode } from "lucide-react";
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
import { usePromptPack, useAgents } from "@/hooks";

interface PromptPackDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

// Mock prompt content for demo
const MOCK_PROMPT_CONTENT = `# System Prompt

You are a helpful AI assistant specialized in customer support for our platform.

## Guidelines

1. Always be polite and professional
2. If you don't know something, say so honestly
3. Provide clear, step-by-step instructions when helping users

## Variables

- {{user_name}} - The customer's name
- {{account_type}} - Their subscription tier (free/pro/enterprise)
- {{context}} - Previous conversation context

## Response Format

Always structure your responses with:
- A greeting using the customer's name
- Clear explanation of the solution
- Next steps or follow-up actions
`;

export default function PromptPackDetailPage({ params }: PromptPackDetailPageProps) {
  const { name } = use(params);
  const searchParams = useSearchParams();
  const namespace = searchParams.get("namespace") || "production";

  const { data: promptPack, isLoading } = usePromptPack(name, namespace);
  const { data: allAgents } = useAgents();

  // Find agents that reference this PromptPack
  const usingAgents = allAgents?.filter(
    (agent) => agent.spec.promptPackRef?.name === name
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

          <TabsContent value="content" className="mt-4">
            <Card>
              <CardHeader>
                <CardTitle>Prompt Template</CardTitle>
                <CardDescription>
                  Content from ConfigMap: {spec.source.configMapRef?.name}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="bg-muted rounded-lg p-4 font-mono text-sm whitespace-pre-wrap max-h-[500px] overflow-auto">
                  {MOCK_PROMPT_CONTENT}
                </div>
                <p className="text-xs text-muted-foreground mt-3">
                  Variables are highlighted with {"{{variable}}"} syntax
                </p>
              </CardContent>
            </Card>
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
