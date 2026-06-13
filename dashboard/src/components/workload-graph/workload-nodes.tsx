"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { Bot, Workflow, Wrench, Sparkles, RotateCcw } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  WorkflowStateNode, InitialNode, FinalNode, VariableNode, ArtifactNode,
} from "./workflow-nodes";
import type { WorkloadNodeData } from "./to-flow";
import type { WorkloadBadge, WorkloadNode } from "./types";

// Fixed size so elk's layout/routing matches the rendered box exactly (no gaps
// between node borders and edge endpoints). border-box keeps the outer size at
// 200x68 regardless of border thickness, so entry/terminal can use thicker
// borders without breaking elk alignment.
const base =
  "box-border w-[200px] h-[68px] px-3 py-2 rounded-lg shadow-sm cursor-pointer transition-all hover:shadow-md text-left bg-card overflow-hidden flex flex-col justify-center";

function badgeIcon(icon?: WorkloadBadge["icon"]) {
  if (icon === "tool") return <Wrench className="h-3 w-3" />;
  if (icon === "skill") return <Sparkles className="h-3 w-3" />;
  if (icon === "loop") return <RotateCcw className="h-3 w-3" />;
  return null;
}

// Endpoints get a thick, strongly-coloured border (emerald=start, rose=end) so
// they read as the most important nodes; everything else is a thinner blue.
function nodeBorderClass(node: WorkloadNode): string {
  if (node.isEntry) return "border-4 border-emerald-500 ring-2 ring-emerald-500/20";
  if (node.isTerminal) return "border-4 border-rose-500 ring-2 ring-rose-500/20";
  return "border-2 border-blue-500";
}

function iconColorClass(node: WorkloadNode): string {
  if (node.isEntry) return "text-emerald-600";
  if (node.isTerminal) return "text-rose-600";
  return "text-blue-600";
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
  const iconColor = iconColorClass(node);
  return (
    <div className="relative">
      <Handle type="target" position={Position.Top} className="!bg-blue-500" />
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
      <Handle type="source" position={Position.Bottom} className="!bg-blue-500" />
    </div>
  );
});
WorkloadAgentNode.displayName = "WorkloadAgentNode";

export const WorkloadProviderNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  return (
    <div className="relative">
      <Handle type="target" position={Position.Top} className="!bg-green-500" />
      <button
        type="button"
        className={cn(base, "border-2 border-green-500 border-dashed")}
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
      <Handle type="target" position={Position.Top} className="!bg-violet-500" />
      <button
        type="button"
        className={cn(base, "border-2 border-violet-500 border-dashed", muted && "opacity-60")}
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
  workloadState: WorkloadAgentNode,       // legacy alias; states now use workflowState
  workloadProvider: WorkloadProviderNode,
  workloadSkill: WorkloadSkillNode,
  workflowState: WorkflowStateNode,
  workflowInitial: InitialNode,
  workflowFinal: FinalNode,
  workflowVariable: VariableNode,
  workflowArtifact: ArtifactNode,
};
