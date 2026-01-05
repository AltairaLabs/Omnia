"use client";

import { memo } from "react";
import { Handle, Position, type Node } from "@xyflow/react";
import { Bot, FileText, Wrench, Package, MessageSquare } from "lucide-react";
import { cn } from "@/lib/utils";

// Base node styles
const baseNodeStyles = "px-4 py-3 rounded-lg border-2 shadow-sm min-w-[140px] cursor-pointer transition-all hover:shadow-md";

// Type-based colors
const typeColors = {
  agent: "border-blue-500 bg-blue-50 dark:bg-blue-950/30",
  promptPack: "border-purple-500 bg-purple-50 dark:bg-purple-950/30",
  toolRegistry: "border-orange-500 bg-orange-50 dark:bg-orange-950/30",
  tool: "border-teal-500 bg-teal-50 dark:bg-teal-950/30",
  prompt: "border-violet-500 bg-violet-50 dark:bg-violet-950/30",
};

// Status indicator component
function StatusDot({ status }: { status?: string }) {
  let color = "bg-gray-400"; // unknown
  switch (status) {
    case "Running":
    case "Ready":
    case "Active":
    case "Available":
      color = "bg-green-500";
      break;
    case "Pending":
    case "Canary":
      color = "bg-yellow-500";
      break;
    case "Failed":
    case "Degraded":
    case "Unavailable":
      color = "bg-red-500";
      break;
  }
  return (
    <div className={cn("w-2.5 h-2.5 rounded-full shrink-0", color)} title={status || "Unknown"} />
  );
}

// Node data types
export interface AgentNodeData extends Record<string, unknown> {
  label: string;
  namespace: string;
  phase?: string;
  onClick?: () => void;
}

export interface PromptPackNodeData extends Record<string, unknown> {
  label: string;
  namespace: string;
  version?: string;
  phase?: string;
  onClick?: () => void;
}

export interface ToolRegistryNodeData extends Record<string, unknown> {
  label: string;
  namespace: string;
  toolCount?: number;
  phase?: string;
  onClick?: () => void;
}

export interface ToolNodeData extends Record<string, unknown> {
  label: string;
  handlerType?: string;
  status?: string;
  onClick?: () => void;
}

export interface PromptNodeData extends Record<string, unknown> {
  label: string;
  onClick?: () => void;
}

// Node types for React Flow
export type AgentNode = Node<AgentNodeData, "agent">;
export type PromptPackNode = Node<PromptPackNodeData, "promptPack">;
export type ToolRegistryNode = Node<ToolRegistryNodeData, "toolRegistry">;
export type ToolNode = Node<ToolNodeData, "tool">;
export type PromptNode = Node<PromptNodeData, "prompt">;

// Props types for custom node components
interface CustomNodeProps<T extends Record<string, unknown>> {
  data: T;
}

// Agent Node Component
export const AgentNodeComponent = memo(({ data }: CustomNodeProps<AgentNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, typeColors.agent)}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-blue-500" />
      <div className="flex items-center gap-2">
        <Bot className="h-4 w-4 text-blue-600 dark:text-blue-400" />
        <div className="flex flex-col flex-1">
          <span className="font-medium text-sm">{data.label}</span>
          <span className="text-xs text-muted-foreground">{data.namespace}</span>
        </div>
        <StatusDot status={data.phase} />
      </div>
      <Handle type="source" position={Position.Right} className="!bg-blue-500" />
    </div>
  );
});
AgentNodeComponent.displayName = "AgentNodeComponent";

// PromptPack Node Component
export const PromptPackNodeComponent = memo(({ data }: CustomNodeProps<PromptPackNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, typeColors.promptPack)}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-purple-500" />
      <div className="flex items-center gap-2">
        <FileText className="h-4 w-4 text-purple-600 dark:text-purple-400" />
        <div className="flex flex-col flex-1">
          <span className="font-medium text-sm">{data.label}</span>
          <span className="text-xs text-muted-foreground">
            {data.version ? `v${data.version}` : data.namespace}
          </span>
        </div>
        <StatusDot status={data.phase} />
      </div>
      <Handle type="source" position={Position.Right} className="!bg-purple-500" />
    </div>
  );
});
PromptPackNodeComponent.displayName = "PromptPackNodeComponent";

// ToolRegistry Node Component
export const ToolRegistryNodeComponent = memo(({ data }: CustomNodeProps<ToolRegistryNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, typeColors.toolRegistry)}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-orange-500" />
      <div className="flex items-center gap-2">
        <Package className="h-4 w-4 text-orange-600 dark:text-orange-400" />
        <div className="flex flex-col flex-1">
          <span className="font-medium text-sm">{data.label}</span>
          <span className="text-xs text-muted-foreground">
            {data.toolCount !== undefined ? `${data.toolCount} tools` : data.namespace}
          </span>
        </div>
        <StatusDot status={data.phase} />
      </div>
      <Handle type="source" position={Position.Right} className="!bg-orange-500" />
    </div>
  );
});
ToolRegistryNodeComponent.displayName = "ToolRegistryNodeComponent";

// Tool Node Component
export const ToolNodeComponent = memo(({ data }: CustomNodeProps<ToolNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, typeColors.tool)}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-teal-500" />
      <div className="flex items-center gap-2">
        <Wrench className="h-4 w-4 text-teal-600 dark:text-teal-400" />
        <div className="flex flex-col flex-1">
          <span className="font-medium text-sm">{data.label}</span>
          {data.handlerType && (
            <span className="text-xs text-muted-foreground">{data.handlerType}</span>
          )}
        </div>
        <StatusDot status={data.status} />
      </div>
    </div>
  );
});
ToolNodeComponent.displayName = "ToolNodeComponent";

// Prompt Node Component (individual prompts within a PromptPack)
export const PromptNodeComponent = memo(({ data }: CustomNodeProps<PromptNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, typeColors.prompt)}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-violet-500" />
      <div className="flex items-center gap-2">
        <MessageSquare className="h-4 w-4 text-violet-600 dark:text-violet-400" />
        <span className="font-medium text-sm">{data.label}</span>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-violet-500" />
    </div>
  );
});
PromptNodeComponent.displayName = "PromptNodeComponent";

// Export node types map for React Flow
export const nodeTypes = {
  agent: AgentNodeComponent,
  promptPack: PromptPackNodeComponent,
  toolRegistry: ToolRegistryNodeComponent,
  tool: ToolNodeComponent,
  prompt: PromptNodeComponent,
};
