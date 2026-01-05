"use client";

import { memo } from "react";
import { Handle, Position, type Node } from "@xyflow/react";
import { Bot, FileText, Wrench, Package, MessageSquare } from "lucide-react";
import { cn } from "@/lib/utils";

// Base node styles
const baseNodeStyles = "px-4 py-3 rounded-lg border-2 shadow-sm min-w-[140px] cursor-pointer transition-all hover:shadow-md";

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

// Status colors
function getStatusColor(phase?: string): string {
  switch (phase) {
    case "Running":
    case "Ready":
    case "Active":
      return "border-green-500 bg-green-50 dark:bg-green-950/30";
    case "Pending":
    case "Canary":
      return "border-yellow-500 bg-yellow-50 dark:bg-yellow-950/30";
    case "Failed":
    case "Degraded":
      return "border-red-500 bg-red-50 dark:bg-red-950/30";
    default:
      return "border-gray-300 bg-gray-50 dark:border-gray-600 dark:bg-gray-900/30";
  }
}

// Agent Node Component
export const AgentNodeComponent = memo(({ data }: CustomNodeProps<AgentNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, getStatusColor(data.phase))}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-primary" />
      <div className="flex items-center gap-2">
        <Bot className="h-4 w-4 text-blue-600 dark:text-blue-400" />
        <div className="flex flex-col">
          <span className="font-medium text-sm">{data.label}</span>
          <span className="text-xs text-muted-foreground">{data.namespace}</span>
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-primary" />
    </div>
  );
});
AgentNodeComponent.displayName = "AgentNodeComponent";

// PromptPack Node Component
export const PromptPackNodeComponent = memo(({ data }: CustomNodeProps<PromptPackNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, getStatusColor(data.phase))}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-primary" />
      <div className="flex items-center gap-2">
        <FileText className="h-4 w-4 text-purple-600 dark:text-purple-400" />
        <div className="flex flex-col">
          <span className="font-medium text-sm">{data.label}</span>
          <span className="text-xs text-muted-foreground">
            {data.version ? `v${data.version}` : data.namespace}
          </span>
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-primary" />
    </div>
  );
});
PromptPackNodeComponent.displayName = "PromptPackNodeComponent";

// ToolRegistry Node Component
export const ToolRegistryNodeComponent = memo(({ data }: CustomNodeProps<ToolRegistryNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, getStatusColor(data.phase))}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-primary" />
      <div className="flex items-center gap-2">
        <Package className="h-4 w-4 text-orange-600 dark:text-orange-400" />
        <div className="flex flex-col">
          <span className="font-medium text-sm">{data.label}</span>
          <span className="text-xs text-muted-foreground">
            {data.toolCount !== undefined ? `${data.toolCount} tools` : data.namespace}
          </span>
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-primary" />
    </div>
  );
});
ToolRegistryNodeComponent.displayName = "ToolRegistryNodeComponent";

// Tool Node Component
export const ToolNodeComponent = memo(({ data }: CustomNodeProps<ToolNodeData>) => {
  const statusColor = data.status === "Available"
    ? "border-green-500 bg-green-50 dark:bg-green-950/30"
    : data.status === "Unavailable"
    ? "border-red-500 bg-red-50 dark:bg-red-950/30"
    : "border-gray-300 bg-gray-50 dark:border-gray-600 dark:bg-gray-900/30";

  return (
    <div
      className={cn(baseNodeStyles, statusColor)}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-primary" />
      <div className="flex items-center gap-2">
        <Wrench className="h-4 w-4 text-teal-600 dark:text-teal-400" />
        <div className="flex flex-col">
          <span className="font-medium text-sm">{data.label}</span>
          {data.handlerType && (
            <span className="text-xs text-muted-foreground">{data.handlerType}</span>
          )}
        </div>
      </div>
    </div>
  );
});
ToolNodeComponent.displayName = "ToolNodeComponent";

// Prompt Node Component (individual prompts within a PromptPack)
export const PromptNodeComponent = memo(({ data }: CustomNodeProps<PromptNodeData>) => {
  return (
    <div
      className={cn(baseNodeStyles, "border-violet-300 bg-violet-50 dark:border-violet-600 dark:bg-violet-950/30")}
      onClick={data.onClick}
    >
      <Handle type="target" position={Position.Left} className="!bg-primary" />
      <div className="flex items-center gap-2">
        <MessageSquare className="h-4 w-4 text-violet-600 dark:text-violet-400" />
        <span className="font-medium text-sm">{data.label}</span>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-primary" />
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
