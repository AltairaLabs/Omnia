"use client";

/**
 * Custom React Flow nodes for the agent architecture diagram. Each node is a
 * small presentational box; layout/positions come from agent-topology-model.
 */

import { memo } from "react";
import Link from "next/link";
import { Handle, Position } from "@xyflow/react";
import { Radio, Cpu, FileText, Database, Brain } from "lucide-react";
import { cn } from "@/lib/utils";
import { FrameworkBadge } from "./framework-badge";
import type { FrameworkType } from "@/types";

const boxBase = "h-full w-full overflow-hidden rounded-lg border bg-card px-3 py-2 shadow-sm";
const labelCls = "flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground";

export interface FacadeNodeData {
  facadeType: string;
  port?: number;
}

export const AgentFacadeNode = memo(function AgentFacadeNode({ data }: { data: FacadeNodeData }) {
  return (
    <div className={cn(boxBase, "border-blue-500/40")}>
      <p className={labelCls}>
        <Radio className="h-3 w-3" />
        Facade
      </p>
      <p className="text-xs font-medium capitalize">{data.facadeType}</p>
      {data.port != null && <p className="text-xs text-muted-foreground">:{data.port}</p>}
      <Handle type="source" position={Position.Right} className="!bg-blue-500" />
    </div>
  );
});

export interface RuntimeNodeData {
  frameworkType: string;
  frameworkVersion?: string;
}

export const AgentRuntimeNode = memo(function AgentRuntimeNode({ data }: { data: RuntimeNodeData }) {
  return (
    <div className={cn(boxBase, "border-primary/40 bg-muted/30")}>
      <div className="flex items-center justify-between gap-2">
        <span className={labelCls}>
          <Cpu className="h-3 w-3" />
          Runtime
        </span>
        <FrameworkBadge framework={data.frameworkType as FrameworkType} />
      </div>
      {data.frameworkVersion && (
        <p className="mt-0.5 text-xs text-muted-foreground">{data.frameworkVersion}</p>
      )}
      <Handle type="target" position={Position.Left} className="!bg-primary" />
    </div>
  );
});

export interface PromptPackNodeData {
  name?: string;
  version?: string;
}

export const AgentPromptPackNode = memo(function AgentPromptPackNode({ data }: { data: PromptPackNodeData }) {
  return (
    <div className={cn(boxBase, "flex flex-col justify-center border-violet-500/40")}>
      <span className={labelCls}>
        <FileText className="h-3 w-3 shrink-0" />
        PromptPack
      </span>
      <span className="block text-xs font-medium truncate leading-tight">{data.name ?? "-"}</span>
      {data.version && (
        <span className="block text-xs text-muted-foreground truncate leading-tight">{data.version}</span>
      )}
    </div>
  );
});

export interface ContextNodeData {
  contextType?: string;
  ttl?: string;
}

export const AgentContextNode = memo(function AgentContextNode({ data }: { data: ContextNodeData }) {
  return (
    <div className={cn(boxBase, "border-slate-400/40")}>
      <p className={labelCls}>
        <Database className="h-3 w-3" />
        Context
      </p>
      <p className="text-xs font-medium">
        <span className="capitalize">{data.contextType ?? "memory"}</span>
        {data.ttl && <span className="text-muted-foreground"> · {data.ttl}</span>}
      </p>
    </div>
  );
});

export interface MemoryNodeData {
  enabled: boolean;
  agentName?: string;
}

export const AgentMemoryNode = memo(function AgentMemoryNode({ data }: { data: MemoryNodeData }) {
  const dot = (
    <span
      aria-hidden
      className={cn(
        "inline-block h-2 w-2 rounded-full",
        data.enabled ? "bg-green-500" : "bg-muted-foreground/40",
      )}
    />
  );
  return (
    <div className={cn(boxBase, "flex items-center justify-between", data.enabled ? "border-green-500/40" : "border-border")}>
      <span className={labelCls}>
        <Brain className="h-3 w-3" />
        Memory
      </span>
      {data.enabled ? (
        <Link
          href={`/memory-analytics?agent=${encodeURIComponent(data.agentName ?? "")}`}
          className="inline-flex items-center gap-1.5 text-xs font-medium text-primary hover:underline"
        >
          {dot}
          On
        </Link>
      ) : (
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          {dot}
          Off
        </span>
      )}
    </div>
  );
});

export const agentTopologyNodeTypes = {
  agentFacade: AgentFacadeNode,
  agentRuntime: AgentRuntimeNode,
  agentPromptPack: AgentPromptPackNode,
  agentContext: AgentContextNode,
  agentMemory: AgentMemoryNode,
};
