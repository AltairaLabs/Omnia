"use client";

import Link from "next/link";
import { X, ExternalLink, Bot, FileText, Package, Zap } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { CostSparkline } from "@/components/cost";
import { ProviderIcon } from "./provider-icons";
import { formatCost, formatTokens } from "@/lib/pricing";
import { useProvider, useAgentCost } from "@/hooks";
import { useProviderMetrics } from "@/hooks/use-provider-metrics";
import type { AgentRuntime, PromptPack, ToolRegistry, Provider, ProviderType } from "@/types";

/** Selected node info for rendering the appropriate card */
export interface SelectedNode {
  type: "agent" | "promptpack" | "tools" | "provider";
  name: string;
  namespace: string;
}

interface NodeSummaryCardProps {
  selectedNode: SelectedNode;
  agents: AgentRuntime[];
  promptPacks: PromptPack[];
  toolRegistries: ToolRegistry[];
  providers: Provider[];
  onClose: () => void;
}

/** Status badge component */
function StatusBadge({ phase }: Readonly<{ phase?: string }>) {
  if (!phase) return <Badge variant="outline">Unknown</Badge>;

  switch (phase) {
    case "Running":
    case "Ready":
    case "Active":
      return <Badge className="bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/30">{phase}</Badge>;
    case "Pending":
    case "Canary":
      return <Badge className="bg-yellow-500/15 text-yellow-700 dark:text-yellow-400 border-yellow-500/30">{phase}</Badge>;
    case "Failed":
    case "Error":
    case "Degraded":
      return <Badge className="bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/30">{phase}</Badge>;
    default:
      return <Badge variant="outline">{phase}</Badge>;
  }
}

/** Type header styles matching topology graph nodes */
const typeHeaderStyles = {
  agent: {
    bg: "bg-blue-500",
    icon: Bot,
    label: "Agent",
  },
  promptpack: {
    bg: "bg-purple-500",
    icon: FileText,
    label: "PromptPack",
  },
  tools: {
    bg: "bg-orange-500",
    icon: Package,
    label: "ToolRegistry",
  },
  provider: {
    bg: "bg-green-500",
    icon: Zap,
    label: "Provider",
  },
} as const;

/** Type header component with icon and colored background */
function TypeHeader({
  type,
  onClose,
  providerType,
}: Readonly<{
  type: keyof typeof typeHeaderStyles;
  onClose: () => void;
  providerType?: ProviderType;
}>) {
  const style = typeHeaderStyles[type];
  const Icon = style.icon;

  return (
    <div className={`${style.bg} -mx-6 -mt-6 px-4 py-2 flex items-center justify-between rounded-t-lg`}>
      <div className="flex items-center gap-2 text-white">
        {type === "provider" && providerType ? (
          <ProviderIcon type={providerType} size={18} className="text-white" />
        ) : (
          <Icon className="h-4 w-4" />
        )}
        <span className="font-medium text-sm">{style.label}</span>
      </div>
      <Button
        variant="ghost"
        size="icon"
        className="h-6 w-6 text-white hover:bg-white/20 hover:text-white"
        onClick={onClose}
      >
        <X className="h-4 w-4" />
      </Button>
    </div>
  );
}

/** Reusable card layout for all summary card types */
interface SummaryCardLayoutProps {
  type: keyof typeof typeHeaderStyles;
  name: string;
  namespace: string;
  phase?: string;
  detailsHref: string;
  onClose: () => void;
  providerType?: ProviderType;
  children: React.ReactNode;
}

function SummaryCardLayout({
  type,
  name,
  namespace,
  phase,
  detailsHref,
  onClose,
  providerType,
  children,
}: Readonly<SummaryCardLayoutProps>) {
  return (
    <Card className="w-80 shadow-lg overflow-hidden">
      <CardHeader className="pb-2">
        <TypeHeader type={type} onClose={onClose} providerType={providerType} />
        <div className="flex items-start justify-between pt-3">
          <div>
            <CardTitle className="text-base">{name}</CardTitle>
            <p className="text-sm text-muted-foreground">{namespace}</p>
          </div>
          <StatusBadge phase={phase} />
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {children}
        <Link href={detailsHref}>
          <Button variant="outline" size="sm" className="w-full mt-2">
            <ExternalLink className="h-4 w-4 mr-2" />
            View Details
          </Button>
        </Link>
      </CardContent>
    </Card>
  );
}

/** Stat item for consistent grid display */
function StatItem({ label, value, title }: Readonly<{ label: string; value: string; title?: string }>) {
  return (
    <div>
      <p className="text-muted-foreground">{label}</p>
      <p className="font-medium truncate" title={title}>{value}</p>
    </div>
  );
}

/** Agent summary card */
function AgentSummaryCard({ agent, onClose }: Readonly<{ agent: AgentRuntime; onClose: () => void }>) {
  const { metadata, spec, status } = agent;
  const { data: provider } = useProvider(spec.providerRef?.name, metadata.namespace || "default");
  const { data: costData } = useAgentCost(metadata.namespace || "default", metadata.name);

  const sparklineData = costData?.timeSeries || [];
  const totalCost = costData?.totalCost || 0;
  const providerType = provider?.spec?.type || spec.provider?.type;
  const sparklineColor = providerType === "openai" ? "#8B5CF6" : "#3B82F6";
  const modelDisplay = (provider?.spec?.model || spec.provider?.model)?.split("-").slice(-2).join("-") || "-";
  const replicaDisplay = `${status?.replicas?.ready ?? 0}/${status?.replicas?.desired ?? spec.runtime?.replicas ?? 1}`;

  return (
    <SummaryCardLayout
      type="agent"
      name={metadata.name}
      namespace={metadata.namespace || "default"}
      phase={status?.phase}
      detailsHref={`/agents/${metadata.name}?namespace=${metadata.namespace}`}
      onClose={onClose}
    >
      {/* Cost Sparkline */}
      <div className="space-y-1">
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Cost (24h)</span>
          <span className="font-medium">{formatCost(totalCost)}</span>
        </div>
        <CostSparkline data={sparklineData} color={sparklineColor} height={28} />
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-3 text-sm">
        <StatItem label="Provider" value={providerType || "-"} />
        <StatItem label="Model" value={modelDisplay} title={provider?.spec?.model || spec.provider?.model} />
        <StatItem label="Replicas" value={replicaDisplay} />
        <StatItem label="Facade" value={spec.facade?.type || "websocket"} />
      </div>
    </SummaryCardLayout>
  );
}

/** Provider summary card */
function ProviderSummaryCard({ provider, onClose }: Readonly<{ provider: Provider; onClose: () => void }>) {
  const { metadata, spec, status } = provider;
  const { data: metrics } = useProviderMetrics(metadata?.name || "", spec?.type);

  const providerColorMap: Record<string, string> = {
    anthropic: "#F97316",
    openai: "#22C55E",
    gemini: "#3B82F6",
    ollama: "#A855F7",
    bedrock: "#EAB308",
    mock: "#6B7280",
  };
  const sparklineColor = spec?.type ? providerColorMap[spec.type] || "#3B82F6" : "#3B82F6";
  const sparklineData = metrics?.costRate?.map(p => ({ value: p.value })) || [];
  const requestsDisplay = metrics?.totalRequests24h ? Math.round(metrics.totalRequests24h).toLocaleString() : "-";
  const tokensDisplay = metrics?.totalTokens24h ? formatTokens(metrics.totalTokens24h) : "-";

  return (
    <SummaryCardLayout
      type="provider"
      name={metadata?.name || ""}
      namespace={metadata?.namespace || "default"}
      phase={status?.phase}
      detailsHref={`/providers/${metadata?.name}?namespace=${metadata?.namespace || "default"}`}
      onClose={onClose}
      providerType={spec?.type as ProviderType}
    >
      {/* Cost Sparkline */}
      <div className="space-y-1">
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Cost (24h)</span>
          <span className="font-medium">{formatCost(metrics?.totalCost24h || 0)}</span>
        </div>
        <CostSparkline data={sparklineData} color={sparklineColor} height={28} />
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-3 text-sm">
        <StatItem label="Type" value={spec?.type || "-"} />
        <StatItem label="Model" value={spec?.model || "-"} title={spec?.model} />
        <StatItem label="Requests (24h)" value={requestsDisplay} />
        <StatItem label="Tokens (24h)" value={tokensDisplay} />
      </div>
    </SummaryCardLayout>
  );
}

/** PromptPack summary card */
function PromptPackSummaryCard({ promptPack, onClose }: Readonly<{ promptPack: PromptPack; onClose: () => void }>) {
  const { metadata, spec, status } = promptPack;

  return (
    <SummaryCardLayout
      type="promptpack"
      name={metadata.name}
      namespace={metadata.namespace || "default"}
      phase={status?.phase}
      detailsHref={`/promptpacks/${metadata.name}?namespace=${metadata.namespace}`}
      onClose={onClose}
    >
      <div className="grid grid-cols-2 gap-3 text-sm">
        <StatItem label="Version" value={spec.version || "-"} />
        <StatItem
          label="Source"
          value={spec.source?.configMapRef?.name || spec.source?.type || "-"}
          title={spec.source?.configMapRef?.name}
        />
      </div>
    </SummaryCardLayout>
  );
}

/** ToolRegistry summary card */
function ToolRegistrySummaryCard({ toolRegistry, onClose }: Readonly<{ toolRegistry: ToolRegistry; onClose: () => void }>) {
  const { metadata, spec, status } = toolRegistry;

  return (
    <SummaryCardLayout
      type="tools"
      name={metadata.name}
      namespace={metadata.namespace || "default"}
      phase={status?.phase}
      detailsHref={`/tools/${metadata.name}?namespace=${metadata.namespace}`}
      onClose={onClose}
    >
      <div className="grid grid-cols-2 gap-3 text-sm">
        <StatItem label="Tools" value={String(status?.discoveredToolsCount ?? 0)} />
        <StatItem label="Handlers" value={String(spec.handlers?.length ?? 0)} />
      </div>
    </SummaryCardLayout>
  );
}

/** Main component that renders the appropriate summary card */
export function NodeSummaryCard({
  selectedNode,
  agents,
  promptPacks,
  toolRegistries,
  providers,
  onClose,
}: Readonly<NodeSummaryCardProps>) {
  const { type, name, namespace } = selectedNode;

  // Find the resource
  if (type === "agent") {
    const agent = agents.find(a => a.metadata.name === name && a.metadata.namespace === namespace);
    if (agent) return <AgentSummaryCard agent={agent} onClose={onClose} />;
  }

  if (type === "provider") {
    const provider = providers.find(p => p.metadata?.name === name && p.metadata?.namespace === namespace);
    if (provider) return <ProviderSummaryCard provider={provider} onClose={onClose} />;
  }

  if (type === "promptpack") {
    const promptPack = promptPacks.find(p => p.metadata.name === name && p.metadata.namespace === namespace);
    if (promptPack) return <PromptPackSummaryCard promptPack={promptPack} onClose={onClose} />;
  }

  if (type === "tools") {
    const toolRegistry = toolRegistries.find(t => t.metadata.name === name && t.metadata.namespace === namespace);
    if (toolRegistry) return <ToolRegistrySummaryCard toolRegistry={toolRegistry} onClose={onClose} />;
  }

  return null;
}
