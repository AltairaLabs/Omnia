"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { Wrench, Sparkles, RotateCcw, Variable, FileText } from "lucide-react";
import { cn } from "@/lib/utils";
import { nodeSize } from "./node-sizes";
import type { WorkloadNodeData } from "./to-flow";
import type { WorkloadBadge } from "./types";

function badgeIcon(icon?: WorkloadBadge["icon"]) {
  if (icon === "tool") return <Wrench className="h-3 w-3" />;
  if (icon === "skill") return <Sparkles className="h-3 w-3" />;
  if (icon === "loop") return <RotateCcw className="h-3 w-3" />;
  return null;
}

export const WorkflowStateNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize("state");
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Top} className="!bg-blue-500" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className="box-border flex flex-col justify-center rounded-full border-2 border-blue-500 bg-card px-4 shadow-sm text-left overflow-hidden cursor-pointer hover:shadow-md"
      >
        <span className="font-medium text-sm truncate">{node.label}</span>
        {node.badges.length > 0 && (
          <span className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
            {node.badges.map((b) => (
              <span key={`${b.icon ?? "b"}-${b.label}`} className="inline-flex items-center gap-0.5">
                {badgeIcon(b.icon)}{b.label}
              </span>
            ))}
          </span>
        )}
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-blue-500" />
    </div>
  );
});
WorkflowStateNode.displayName = "WorkflowStateNode";

export const InitialNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { width, height } = nodeSize("initial");
  return (
    <div className="relative" style={{ width, height }} data-node={data.node.id}>
      <div data-testid="marker-initial" style={{ width, height }} className="rounded-full bg-foreground" />
      <Handle type="source" position={Position.Bottom} className="!bg-foreground" />
    </div>
  );
});
InitialNode.displayName = "InitialNode";

export const FinalNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { width, height } = nodeSize("final");
  return (
    <div className="relative" style={{ width, height }} data-node={data.node.id}>
      <Handle type="target" position={Position.Top} className="!bg-foreground" />
      <div data-testid="marker-final" style={{ width, height }} className="rounded-full border-2 border-foreground flex items-center justify-center">
        <div className="rounded-full bg-foreground" style={{ width: width - 10, height: height - 10 }} />
      </div>
    </div>
  );
});
FinalNode.displayName = "FinalNode";

export const VariableNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize("variable");
  return (
    <div className="relative" style={{ width, height }}>
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className="box-border inline-flex items-center justify-center gap-1 rounded-full border border-dashed border-amber-500 bg-amber-50 px-2 text-xs text-amber-900 cursor-pointer overflow-hidden dark:bg-amber-950/30 dark:text-amber-200"
      >
        <Variable className="h-3 w-3 shrink-0" />
        <span className="truncate font-mono">{node.label}</span>
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-amber-500" />
    </div>
  );
});
VariableNode.displayName = "VariableNode";

export const ArtifactNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize("artifact");
  const muted = node.resolution === "unresolved";
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Top} className="!bg-teal-500" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height, transform: "skewX(-12deg)" }}
        className={cn(
          "box-border flex items-center justify-center gap-1 border-2 border-teal-500 bg-teal-50 px-2 cursor-pointer overflow-hidden dark:bg-teal-950/30",
          muted && "opacity-50 border-dashed",
        )}
      >
        <span style={{ transform: "skewX(12deg)" }} className="inline-flex items-center gap-1 text-xs text-teal-900 dark:text-teal-200">
          <FileText className="h-3 w-3 shrink-0" />
          <span className="truncate font-mono">{node.label}</span>
        </span>
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-teal-500" />
    </div>
  );
});
ArtifactNode.displayName = "ArtifactNode";
