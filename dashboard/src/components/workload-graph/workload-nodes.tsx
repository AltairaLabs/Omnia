"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { Bot, Workflow, Wrench, Sparkles, Play, Flag, RotateCcw } from "lucide-react";
import { cn } from "@/lib/utils";
import type { WorkloadNodeData } from "./to-flow";
import type { WorkloadBadge, WorkloadNode } from "./types";

const base =
  "px-4 py-3 rounded-lg border-2 shadow-sm min-w-[160px] cursor-pointer transition-all hover:shadow-md text-left bg-card";

function badgeIcon(icon?: WorkloadBadge["icon"]) {
  if (icon === "tool") return <Wrench className="h-3 w-3" />;
  if (icon === "skill") return <Sparkles className="h-3 w-3" />;
  if (icon === "loop") return <RotateCcw className="h-3 w-3" />;
  return null;
}

function nodeBorderClass(node: WorkloadNode): string {
  if (node.isEntry) return "border-emerald-500";
  if (node.isTerminal) return "border-zinc-400";
  return "border-blue-500";
}

function EndpointPill({ node }: Readonly<{ node: WorkloadNode }>) {
  if (node.isEntry) {
    return (
      <span className="absolute -top-2.5 left-2 z-10 inline-flex items-center gap-0.5 rounded bg-emerald-500 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white shadow">
        <Play className="h-2.5 w-2.5 fill-current" />Start
      </span>
    );
  }
  if (node.isTerminal) {
    return (
      <span className="absolute -top-2.5 left-2 z-10 inline-flex items-center gap-0.5 rounded bg-zinc-600 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white shadow">
        <Flag className="h-2.5 w-2.5" />End
      </span>
    );
  }
  return null;
}

function BadgeRow({ badges }: Readonly<{ badges: WorkloadBadge[] }>) {
  if (badges.length === 0) return null;
  return (
    <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
      {badges.map((b) => (
        <span key={`${b.icon ?? "badge"}-${b.label}`} className="inline-flex items-center gap-0.5">
          {badgeIcon(b.icon)}
          {b.label}
        </span>
      ))}
    </div>
  );
}

export const WorkloadAgentNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const muted = node.resolution === "unavailable";
  const iconColor = node.isEntry ? "text-emerald-600" : "text-blue-600";
  return (
    <div className="relative">
      <EndpointPill node={node} />
      <Handle type="target" position={Position.Left} className="!bg-blue-500" />
      <button
        type="button"
        className={cn(base, nodeBorderClass(node), muted && "opacity-50")}
        onClick={() => onClick?.(node.id)}
      >
        <div className="flex items-center gap-2">
          {node.kind === "state" ? (
            <Workflow className={cn("h-4 w-4", iconColor)} />
          ) : (
            <Bot className={cn("h-4 w-4", iconColor)} />
          )}
          <span className="font-medium text-sm">{node.label}</span>
        </div>
        <BadgeRow badges={node.badges} />
      </button>
      <Handle type="source" position={Position.Right} className="!bg-blue-500" />
    </div>
  );
});
WorkloadAgentNode.displayName = "WorkloadAgentNode";

export const WorkloadProviderNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  return (
    <div className="relative">
      <Handle type="target" position={Position.Left} className="!bg-green-500" />
      <button
        type="button"
        className={cn(base, "border-green-500 border-dashed")}
        onClick={() => onClick?.(node.id)}
      >
        <div className="flex flex-col">
          <span className="font-medium text-sm">{node.label}</span>
          <span className="text-xs text-muted-foreground">
            {node.detail.model || node.detail.providerType}
          </span>
          {node.detail.role && (
            <span className="text-xs text-muted-foreground">{node.detail.role}</span>
          )}
        </div>
      </button>
    </div>
  );
});
WorkloadProviderNode.displayName = "WorkloadProviderNode";

export const WorkloadSkillNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const muted = node.resolution === "unavailable" || node.resolution === "unresolved";
  const phaseLabel = node.badges[0]?.label ?? node.detail.skillPhase;
  return (
    <div className="relative">
      <Handle type="target" position={Position.Left} className="!bg-violet-500" />
      <button
        type="button"
        className={cn(base, "border-violet-500 border-dashed", muted && "opacity-60")}
        onClick={() => onClick?.(node.id)}
      >
        <div className="flex items-center gap-2">
          <Sparkles className="h-4 w-4 text-violet-600" />
          <span className="font-medium text-sm">{node.label}</span>
        </div>
        <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
          <span>{phaseLabel}</span>
          {node.detail.mountAs && <span className="font-mono">→ {node.detail.mountAs}</span>}
        </div>
      </button>
    </div>
  );
});
WorkloadSkillNode.displayName = "WorkloadSkillNode";

export const workloadNodeTypes = {
  workloadAgent: WorkloadAgentNode,
  workloadState: WorkloadAgentNode,
  workloadProvider: WorkloadProviderNode,
  workloadSkill: WorkloadSkillNode,
};
