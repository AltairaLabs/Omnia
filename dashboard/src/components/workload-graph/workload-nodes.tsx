"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { Bot, Workflow, Wrench, Sparkles, Flag, DoorOpen, RotateCcw } from "lucide-react";
import { cn } from "@/lib/utils";
import type { WorkloadNodeData } from "./to-flow";
import type { WorkloadBadge } from "./types";

const base =
  "px-4 py-3 rounded-lg border-2 shadow-sm min-w-[160px] cursor-pointer transition-all hover:shadow-md text-left bg-card";

function badgeIcon(icon?: WorkloadBadge["icon"]) {
  if (icon === "tool") return <Wrench className="h-3 w-3" />;
  if (icon === "skill") return <Sparkles className="h-3 w-3" />;
  if (icon === "loop") return <RotateCcw className="h-3 w-3" />;
  return null;
}

function BadgeRow({
  badges,
  isEntry,
  isTerminal,
}: Readonly<{ badges: WorkloadBadge[]; isEntry?: boolean; isTerminal?: boolean }>) {
  return (
    <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
      {isEntry && (
        <span className="inline-flex items-center gap-0.5">
          <DoorOpen className="h-3 w-3" />entry
        </span>
      )}
      {isTerminal && (
        <span className="inline-flex items-center gap-0.5">
          <Flag className="h-3 w-3" />terminal
        </span>
      )}
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
  return (
    <div className="relative">
      <Handle type="target" position={Position.Left} className="!bg-blue-500" />
      <button
        type="button"
        className={cn(base, "border-blue-500", muted && "opacity-50")}
        onClick={() => onClick?.(node.id)}
      >
        <div className="flex items-center gap-2">
          {node.kind === "state" ? (
            <Workflow className="h-4 w-4 text-blue-600" />
          ) : (
            <Bot className="h-4 w-4 text-blue-600" />
          )}
          <span className="font-medium text-sm">{node.label}</span>
        </div>
        <BadgeRow badges={node.badges} isEntry={node.isEntry} isTerminal={node.isTerminal} />
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

export const workloadNodeTypes = {
  workloadAgent: WorkloadAgentNode,
  workloadState: WorkloadAgentNode,
  workloadProvider: WorkloadProviderNode,
};
