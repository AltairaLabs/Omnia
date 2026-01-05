"use client";

import { use } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ExternalLink, Bot, GitBranch, Clock, FileCode, Wrench, MessageSquare, FileText, Shield, Variable } from "lucide-react";
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
import { usePromptPack, useAgents } from "@/hooks";

interface PromptPackDetailPageProps {
  params: Promise<{ name: string }>;
}

function formatDate(timestamp?: string): string {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

// Mock PromptPack content following promptpack.org spec
const MOCK_PROMPTPACK_CONTENT = {
  id: "support-prompts",
  name: "Customer Support Pack",
  version: "1.2.0",
  description: "Comprehensive prompt pack for customer support interactions",
  template_engine: {
    version: "1.1",
    syntax: "handlebars",
  },
  prompts: [
    {
      id: "general-support",
      name: "General Support",
      version: "1.2.0",
      system_template: `You are a helpful customer support agent for {{company_name}}.

Your role is to assist customers with their inquiries while maintaining a professional and friendly tone.

## Guidelines
- Always greet the customer by name: {{user_name}}
- Reference their account type ({{account_type}}) when relevant
- Be concise but thorough in your responses
- If you cannot help, escalate appropriately

{{#if premium_support}}
This customer has premium support - prioritize their requests.
{{/if}}`,
      variables: [
        { name: "company_name", type: "string", required: true },
        { name: "user_name", type: "string", required: true },
        { name: "account_type", type: "enum", values: ["free", "pro", "enterprise"], required: true },
        { name: "premium_support", type: "boolean", required: false },
      ],
      tools: ["search_knowledge_base", "create_ticket", "check_order_status"],
      parameters: {
        temperature: 0.7,
        max_tokens: 1024,
      },
      validators: ["pii_detection", "profanity_filter"],
    },
    {
      id: "technical-support",
      name: "Technical Support",
      version: "1.1.0",
      system_template: `You are a technical support specialist for {{company_name}}.

Help users troubleshoot technical issues with our products.

## Troubleshooting Approach
1. Gather system information
2. Reproduce the issue if possible
3. Check known issues database
4. Provide step-by-step solutions

User's system: {{system_info}}
Error code (if any): {{error_code}}`,
      variables: [
        { name: "company_name", type: "string", required: true },
        { name: "system_info", type: "object", required: false },
        { name: "error_code", type: "string", required: false },
      ],
      tools: ["search_knowledge_base", "check_system_status", "create_ticket"],
      parameters: {
        temperature: 0.3,
        max_tokens: 2048,
      },
      validators: ["pii_detection"],
    },
  ],
  tools: [
    {
      name: "search_knowledge_base",
      description: "Search the internal knowledge base for relevant articles",
      parameters: {
        type: "object",
        properties: {
          query: { type: "string", description: "Search query" },
          category: { type: "string", description: "Category filter" },
          limit: { type: "number", description: "Max results", default: 5 },
        },
        required: ["query"],
      },
    },
    {
      name: "create_ticket",
      description: "Create a support ticket for escalation",
      parameters: {
        type: "object",
        properties: {
          subject: { type: "string" },
          description: { type: "string" },
          priority: { type: "string", enum: ["low", "medium", "high", "urgent"] },
          category: { type: "string" },
        },
        required: ["subject", "description", "priority"],
      },
    },
    {
      name: "check_order_status",
      description: "Check the status of a customer order",
      parameters: {
        type: "object",
        properties: {
          order_id: { type: "string" },
          email: { type: "string" },
        },
        required: ["order_id"],
      },
    },
    {
      name: "check_system_status",
      description: "Check current system and service status",
      parameters: {
        type: "object",
        properties: {
          service: { type: "string", description: "Service name to check" },
        },
        required: [],
      },
    },
  ],
  fragments: {
    brand_voice: `Always maintain our brand voice:
- Friendly but professional
- Empathetic and understanding
- Solution-oriented
- Clear and concise`,
    escalation_notice: `If you cannot resolve this issue, please let the customer know you'll escalate to a specialist who will follow up within 24 hours.`,
    privacy_reminder: `Remember: Never ask for or store sensitive information like passwords, full credit card numbers, or social security numbers.`,
  },
  validators: [
    {
      id: "pii_detection",
      name: "PII Detection",
      description: "Detects and redacts personally identifiable information",
      config: {
        detect: ["ssn", "credit_card", "phone", "email"],
        action: "redact",
      },
    },
    {
      id: "profanity_filter",
      name: "Profanity Filter",
      description: "Filters inappropriate language from responses",
      config: {
        severity: "medium",
        action: "block",
      },
    },
  ],
};

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

          <TabsContent value="content" className="space-y-4 mt-4">
            {/* Metadata Card */}
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-base">Pack Metadata</CardTitle>
                  <Badge variant="outline">v{MOCK_PROMPTPACK_CONTENT.version}</Badge>
                </div>
                <CardDescription>{MOCK_PROMPTPACK_CONTENT.description}</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                  <div>
                    <p className="text-muted-foreground">ID</p>
                    <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{MOCK_PROMPTPACK_CONTENT.id}</code>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Template Engine</p>
                    <span className="font-medium">{MOCK_PROMPTPACK_CONTENT.template_engine.syntax} v{MOCK_PROMPTPACK_CONTENT.template_engine.version}</span>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Prompts</p>
                    <span className="font-medium">{MOCK_PROMPTPACK_CONTENT.prompts.length}</span>
                  </div>
                  <div>
                    <p className="text-muted-foreground">Tools</p>
                    <span className="font-medium">{MOCK_PROMPTPACK_CONTENT.tools.length}</span>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Prompts Section */}
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-center gap-2">
                  <MessageSquare className="h-5 w-5 text-muted-foreground" />
                  <CardTitle className="text-base">Prompts ({MOCK_PROMPTPACK_CONTENT.prompts.length})</CardTitle>
                </div>
                <CardDescription>Specialized prompts for different scenarios</CardDescription>
              </CardHeader>
              <CardContent className="pt-0">
                <Accordion type="multiple" className="w-full">
                  {MOCK_PROMPTPACK_CONTENT.prompts.map((prompt) => (
                    <AccordionItem key={prompt.id} value={prompt.id}>
                      <AccordionTrigger className="hover:no-underline">
                        <div className="flex items-center gap-3">
                          <span className="font-medium">{prompt.name}</span>
                          <Badge variant="secondary" className="text-xs">v{prompt.version}</Badge>
                        </div>
                      </AccordionTrigger>
                      <AccordionContent className="space-y-4">
                        {/* System Template */}
                        <div>
                          <p className="text-sm font-medium mb-2">System Template</p>
                          <div className="bg-muted rounded-lg p-3 font-mono text-xs whitespace-pre-wrap max-h-[200px] overflow-auto">
                            {prompt.system_template}
                          </div>
                        </div>

                        {/* Variables */}
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
                                {"values" in v && (
                                  <span className="text-muted-foreground">
                                    [{(v.values as string[]).join(", ")}]
                                  </span>
                                )}
                              </div>
                            ))}
                          </div>
                        </div>

                        {/* Tools & Parameters */}
                        <div className="grid md:grid-cols-2 gap-4">
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
                          <div>
                            <p className="text-sm font-medium mb-2">Parameters</p>
                            <div className="text-xs space-y-1">
                              <div className="flex justify-between">
                                <span className="text-muted-foreground">Temperature</span>
                                <span className="font-mono">{prompt.parameters.temperature}</span>
                              </div>
                              <div className="flex justify-between">
                                <span className="text-muted-foreground">Max Tokens</span>
                                <span className="font-mono">{prompt.parameters.max_tokens}</span>
                              </div>
                            </div>
                          </div>
                        </div>

                        {/* Validators */}
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
                      </AccordionContent>
                    </AccordionItem>
                  ))}
                </Accordion>
              </CardContent>
            </Card>

            {/* Tools Section */}
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-center gap-2">
                  <Wrench className="h-5 w-5 text-muted-foreground" />
                  <CardTitle className="text-base">Tools ({MOCK_PROMPTPACK_CONTENT.tools.length})</CardTitle>
                </div>
                <CardDescription>Shared tool definitions available to all prompts</CardDescription>
              </CardHeader>
              <CardContent className="pt-0">
                <Accordion type="multiple" className="w-full">
                  {MOCK_PROMPTPACK_CONTENT.tools.map((tool) => (
                    <AccordionItem key={tool.name} value={tool.name}>
                      <AccordionTrigger className="hover:no-underline">
                        <div className="flex items-center gap-2">
                          <code className="text-sm font-medium">{tool.name}</code>
                        </div>
                      </AccordionTrigger>
                      <AccordionContent>
                        <p className="text-sm text-muted-foreground mb-3">{tool.description}</p>
                        <div>
                          <p className="text-sm font-medium mb-2">Parameters</p>
                          <div className="bg-muted rounded-lg p-3 font-mono text-xs overflow-auto">
                            <pre>{JSON.stringify(tool.parameters, null, 2)}</pre>
                          </div>
                        </div>
                      </AccordionContent>
                    </AccordionItem>
                  ))}
                </Accordion>
              </CardContent>
            </Card>

            {/* Fragments Section */}
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-center gap-2">
                  <FileText className="h-5 w-5 text-muted-foreground" />
                  <CardTitle className="text-base">Fragments ({Object.keys(MOCK_PROMPTPACK_CONTENT.fragments).length})</CardTitle>
                </div>
                <CardDescription>Reusable text blocks shared across prompts</CardDescription>
              </CardHeader>
              <CardContent className="pt-0">
                <Accordion type="multiple" className="w-full">
                  {Object.entries(MOCK_PROMPTPACK_CONTENT.fragments).map(([key, value]) => (
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

            {/* Validators Section */}
            <Card>
              <CardHeader className="pb-3">
                <div className="flex items-center gap-2">
                  <Shield className="h-5 w-5 text-muted-foreground" />
                  <CardTitle className="text-base">Validators ({MOCK_PROMPTPACK_CONTENT.validators.length})</CardTitle>
                </div>
                <CardDescription>Safety guardrails and content filters</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {MOCK_PROMPTPACK_CONTENT.validators.map((validator) => (
                    <div key={validator.id} className="border rounded-lg p-3">
                      <div className="flex items-center justify-between mb-2">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{validator.name}</span>
                          <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{validator.id}</code>
                        </div>
                        <Badge variant="outline" className="text-xs">{validator.config.action}</Badge>
                      </div>
                      <p className="text-sm text-muted-foreground mb-2">{validator.description}</p>
                      {"detect" in validator.config && (
                        <div className="flex flex-wrap gap-1">
                          {(validator.config.detect as string[]).map((item) => (
                            <Badge key={item} variant="secondary" className="text-xs">{item}</Badge>
                          ))}
                        </div>
                      )}
                      {"severity" in validator.config && (
                        <div className="text-xs text-muted-foreground">
                          Severity: <span className="font-medium">{validator.config.severity as string}</span>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
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
