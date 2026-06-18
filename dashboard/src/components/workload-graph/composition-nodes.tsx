"use client";

import { memo } from "react";
import { Handle, Position } from "@xyflow/react";
import { Workflow, GitBranch, Layers, Wrench, Bot, Sparkles, ChevronUp } from "lucide-react";
import { cn } from "@/lib/utils";
import { nodeSize } from "./node-sizes";
import type { WorkloadNodeData } from "./to-flow";
import type { WorkloadNode } from "./types";

function stepIcon(kind?: string) {
  if (kind === "agent") return <Bot className="h-3.5 w-3.5 text-indigo-600" />;
  if (kind === "tool") return <Wrench className="h-3.5 w-3.5 text-amber-600" />;
  if (kind === "branch") return <GitBranch className="h-3.5 w-3.5 text-blue-600" />;
  return <Sparkles className="h-3.5 w-3.5 text-indigo-600" />;
}

// The translucent highlighted box (React Flow group node). Children render on top.
export const CompositionContainerNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onToggle } = data;
  return (
    <div className="box-border h-full w-full rounded-lg border-2 border-indigo-400 bg-indigo-50/40 dark:bg-indigo-950/20 shadow-sm">
      <Handle type="target" position={Position.Top} className="!bg-indigo-500" />
      <div className="flex items-center justify-between px-3 py-1.5">
        <span className="inline-flex items-center gap-1.5 text-sm font-medium text-indigo-900 dark:text-indigo-200">
          <Workflow className="h-4 w-4" />
          {node.detail.compositionName ?? node.label}
          <span className="rounded bg-indigo-200/70 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-indigo-800 dark:bg-indigo-800/50 dark:text-indigo-200">
            composition
          </span>
        </span>
        <button
          type="button"
          aria-label="Collapse composition"
          onClick={() => onToggle?.(node.id)}
          className="rounded p-0.5 text-indigo-700 hover:bg-indigo-200/60 dark:text-indigo-300"
        >
          <ChevronUp className="h-4 w-4" />
        </button>
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-indigo-500" />
    </div>
  );
});
CompositionContainerNode.displayName = "CompositionContainerNode";

// Parallel fan-out frame (also a group node); its branches render inside.
export const CompositionParallelNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node } = data;
  return (
    <div className="box-border h-full w-full rounded-md border border-dashed border-teal-400 bg-teal-50/40 dark:bg-teal-950/20">
      <Handle type="target" position={Position.Top} className="!bg-teal-500" />
      <div className="flex items-center gap-1.5 px-2 py-1 text-xs font-medium text-teal-800 dark:text-teal-200">
        <Layers className="h-3.5 w-3.5" />
        parallel
        {node.detail.reducer && <span className="font-mono text-[11px] text-teal-700 dark:text-teal-300">{node.detail.reducer}</span>}
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-teal-500" />
    </div>
  );
});
CompositionParallelNode.displayName = "CompositionParallelNode";

function branchShape(node: WorkloadNode) {
  return (
    <div className="box-border flex h-full w-full items-center justify-center">
      <div
        className="flex items-center justify-center border-2 border-blue-500 bg-card text-center"
        style={{ width: 56, height: 56, transform: "rotate(45deg)" }}
      >
        <span style={{ transform: "rotate(-45deg)" }} className="text-[11px] font-medium">{node.label}</span>
      </div>
    </div>
  );
}

function BranchStep({ node, onClick }: Readonly<{ node: WorkloadNode; onClick?: (id: string) => void }>) {
  const { width, height } = nodeSize(node.kind);
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Top} className="!bg-blue-500" />
      <button type="button" onClick={() => onClick?.(node.id)} className="h-full w-full cursor-pointer" aria-label={node.label}>
        {branchShape(node)}
      </button>
      {node.detail.predicateText && (
        <span className="absolute left-1/2 top-full mt-0.5 -translate-x-1/2 whitespace-nowrap rounded bg-background/85 px-1 text-[10px] text-muted-foreground">
          {node.detail.predicateText}
        </span>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-blue-500" />
    </div>
  );
}

export const CompositionStepNode = memo(({ data }: Readonly<{ data: WorkloadNodeData }>) => {
  const { node, onClick } = data;
  const { width, height } = nodeSize(node.kind);
  if (node.kind === "stepBranch") {
    return <BranchStep node={node} onClick={onClick} />;
  }
  return (
    <div className="relative" style={{ width, height }}>
      <Handle type="target" position={Position.Top} className="!bg-indigo-500" />
      <button
        type="button"
        onClick={() => onClick?.(node.id)}
        style={{ width, height }}
        className={cn("box-border flex flex-col justify-center gap-0.5 rounded-md border border-indigo-300 bg-card px-2 text-left shadow-sm cursor-pointer hover:shadow-md overflow-hidden")}
      >
        <span className="inline-flex items-center gap-1.5 text-xs font-medium truncate">
          {stepIcon(node.detail.stepKind)}
          {node.label}
        </span>
        {node.detail.promptTask && <span className="truncate text-[11px] text-muted-foreground">{node.detail.promptTask}</span>}
        {node.detail.toolRef && <span className="truncate font-mono text-[11px] text-muted-foreground">{node.detail.toolRef}</span>}
        {node.detail.termination && <span className="truncate text-[11px] text-muted-foreground">{node.detail.termination}</span>}
      </button>
      <Handle type="source" position={Position.Bottom} className="!bg-indigo-500" />
    </div>
  );
});
CompositionStepNode.displayName = "CompositionStepNode";
