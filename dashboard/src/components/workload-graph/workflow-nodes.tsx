"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { Wrench, Sparkles, RotateCcw, Variable, FileText, Layers, Scale, User, ChevronDown, Workflow } from "lucide-react";
import { cn } from "@/lib/utils";
import { nodeSize } from "./node-sizes";
import type { WorkloadNodeData } from "./to-flow";
import type { WorkloadBadge, WorkloadNode } from "./types";

function badgeIcon(icon?: WorkloadBadge["icon"]) {
  if (icon === "tool") return <Wrench className="h-3 w-3" />;
  if (icon === "skill") return <Sparkles className="h-3 w-3" />;
  if (icon === "loop") return <RotateCcw className="h-3 w-3" />;
  return null;
}

// Collapsed composition state: a distinct "subprocess" shape — a stacked,
// rectangular indigo card with a top-right chevron-down (v) disclosure control —
// so it's visually obvious it expands into a sub-flow. The whole node toggles;
// `nodrag`/`nopan` keep React Flow's drag handler from swallowing repeat clicks.
function CollapsedCompositionNode({
  node,
  onToggle,
}: Readonly<{ node: WorkloadNode; onToggle?: (id: string) => void }>) {
  const { width, height } = nodeSize("state");
  const steps = node.detail.stepCount;
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Top} className="!bg-category-2" />
      {/* stacked layers behind, hinting "contains a sub-flow" */}
      <div aria-hidden="true" className="absolute inset-0 translate-x-[5px] translate-y-[5px] rounded-md border-2 border-category-2 bg-category-2/50 dark:border-category-2 dark:bg-category-2/20" />
      <div aria-hidden="true" className="absolute inset-0 translate-x-[2px] translate-y-[2px] rounded-md border-2 border-category-2 bg-category-2/70 dark:border-category-2 dark:bg-category-2/30" />
      <button
        type="button"
        onClick={() => onToggle?.(node.id)}
        aria-label="Expand composition"
        style={{ width, height }}
        className="nodrag nopan relative box-border flex flex-col justify-center rounded-md border-2 border-category-2 bg-card pl-4 pr-8 text-left shadow-sm cursor-pointer hover:shadow-md overflow-hidden"
      >
        <span className="inline-flex items-center gap-1.5 font-medium text-sm truncate">
          <Workflow className="h-4 w-4 shrink-0 text-category-2" />
          {node.label}
        </span>
        <span className="text-[11px] text-category-2 dark:text-category-2">
          composition{steps ? ` · ${steps} steps` : ""}
        </span>
        {/* top-right disclosure chevron: v = expand */}
        <ChevronDown className="absolute right-2 top-2 h-4 w-4 text-category-2" />
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-category-2" />
    </div>
  );
}

export const WorkflowStateNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick, onToggle, expandable } = data;
  if (expandable) return <CollapsedCompositionNode node={node} onToggle={onToggle} />;
  const { width, height } = nodeSize("state");
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Top} className="!bg-category-1" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className="box-border flex flex-col justify-center rounded-full border-2 border-category-1 bg-card px-4 shadow-sm text-left overflow-hidden cursor-pointer hover:shadow-md"
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
      <Handle type="source" position={Position.Bottom} className="!bg-category-1" />
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
        className="box-border inline-flex items-center justify-center gap-1 rounded-full border border-dashed border-category-4 bg-category-4/10 px-2 text-xs text-category-4 cursor-pointer overflow-hidden dark:bg-category-4/30 dark:text-category-4"
      >
        <Variable className="h-3 w-3 shrink-0" />
        <span className="truncate font-mono">{node.label}</span>
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-category-4" />
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
      <Handle type="target" position={Position.Top} className="!bg-category-6" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height, transform: "skewX(-12deg)" }}
        className={cn(
          "box-border flex items-center justify-center gap-1 border-2 border-category-6 bg-category-6/10 px-2 cursor-pointer overflow-hidden dark:bg-category-6/30",
          muted && "opacity-50 border-dashed",
        )}
      >
        <span style={{ transform: "skewX(12deg)" }} className="inline-flex items-center gap-1 text-xs text-category-6 dark:text-category-6">
          <FileText className="h-3 w-3 shrink-0" />
          <span className="truncate font-mono">{node.label}</span>
        </span>
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-category-6" />
    </div>
  );
});
ArtifactNode.displayName = "ArtifactNode";

export const ScenarioGroupNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize("scenario");
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="source" position={Position.Right} className="!bg-category-6" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className="box-border flex items-center gap-2 rounded-md border-2 border-category-6 bg-category-6/10 px-3 text-left cursor-pointer overflow-hidden hover:shadow-md dark:bg-category-6/30"
      >
        <Layers className="h-4 w-4 shrink-0 text-category-6" />
        <span className="font-medium text-sm text-category-6 truncate dark:text-category-6">{node.label}</span>
      </button>
    </div>
  );
});
ScenarioGroupNode.displayName = "ScenarioGroupNode";

export const JudgeNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize("judge");
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Left} className="!bg-category-3" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className="box-border flex flex-col justify-center rounded-md border-2 border-category-3 bg-card px-3 text-left cursor-pointer overflow-hidden hover:shadow-md"
      >
        <span className="inline-flex items-center gap-2 font-medium text-sm truncate">
          <Scale className="h-4 w-4 shrink-0 text-category-3" />
          {node.label}
        </span>
        {node.detail.judgeProvider && (
          <span className="text-xs text-muted-foreground truncate">{node.detail.judgeProvider}</span>
        )}
      </button>
    </div>
  );
});
JudgeNode.displayName = "JudgeNode";

export const PersonaNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize("persona");
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="source" position={Position.Right} className="!bg-category-4" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className="box-border flex items-center gap-2 rounded-md border-2 border-dashed border-category-4 bg-card px-3 text-left cursor-pointer overflow-hidden hover:shadow-md"
      >
        <User className="h-4 w-4 shrink-0 text-category-4" />
        <span className="font-medium text-sm truncate">{node.label}</span>
      </button>
      <Handle type="target" position={Position.Left} className="!bg-category-4" />
    </div>
  );
});
PersonaNode.displayName = "PersonaNode";
