"use client";

import { memo } from "react";
import { Handle, Position, type Node } from "@xyflow/react";
import { Bot, FileText, Wrench, Package, MessageSquare, StickyNote, X } from "lucide-react";
import { cn } from "@/lib/utils";

// Base node styles
const baseNodeStyles = "px-4 py-3 rounded-lg border-2 shadow-sm min-w-[140px] cursor-pointer transition-all hover:shadow-md";

// Post-it note component attached to nodes
function PostItNote({
  note,
  onEdit,
  onDelete
}: Readonly<{
  note: string;
  onEdit: () => void;
  onDelete: () => void;
}>) {
  return (
    <div
      className="absolute -top-2 -right-2 translate-x-full max-w-[180px] z-10"
      onClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => e.stopPropagation()}
    >
      <div className="relative bg-yellow-200 dark:bg-yellow-300 text-yellow-900 p-2 rounded shadow-md text-xs transform rotate-2 hover:rotate-0 transition-transform">
        {/* Tape effect */}
        <div className="absolute -top-1.5 left-1/2 -translate-x-1/2 w-8 h-3 bg-yellow-100/80 dark:bg-yellow-200/80 rounded-sm" />

        {/* Delete button */}
        <button
          className="absolute -top-1 -right-1 w-4 h-4 bg-red-500 rounded-full flex items-center justify-center text-white hover:bg-red-600 opacity-0 hover:opacity-100 transition-opacity"
          onClick={onDelete}
          style={{ opacity: 'var(--delete-opacity, 0)' }}
        >
          <X className="h-2.5 w-2.5" />
        </button>

        {/* Note content - using button for semantic HTML */}
        <button
          type="button"
          className="cursor-text leading-tight line-clamp-4 hover:line-clamp-none text-left w-full bg-transparent border-none p-0 font-inherit text-inherit"
          onClick={onEdit}
          title="Click to edit"
        >
          {note}
        </button>
      </div>
    </div>
  );
}

// Add note button
function AddNoteButton({ onClick }: Readonly<{ onClick: () => void }>) {
  return (
    <button
      className="absolute -top-1 -right-1 w-5 h-5 bg-yellow-400 dark:bg-yellow-500 rounded-full flex items-center justify-center text-yellow-900 hover:scale-110 transition-transform opacity-0 group-hover:opacity-100 shadow-sm"
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      title="Add note"
    >
      <StickyNote className="h-3 w-3" />
    </button>
  );
}

// Type-based colors
const typeColors = {
  agent: "border-blue-500 bg-blue-50 dark:bg-blue-950/30",
  promptPack: "border-purple-500 bg-purple-50 dark:bg-purple-950/30",
  toolRegistry: "border-orange-500 bg-orange-50 dark:bg-orange-950/30",
  tool: "border-teal-500 bg-teal-50 dark:bg-teal-950/30",
  prompt: "border-violet-500 bg-violet-50 dark:bg-violet-950/30",
};

// Status indicator component
function StatusDot({ status }: Readonly<{ status?: string }>) {
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

// Note-related fields shared by nodes
interface NoteFields {
  note?: string;
  onNoteEdit?: (type: string, namespace: string, name: string) => void;
  onNoteDelete?: (type: string, namespace: string, name: string) => void;
}

// Node data types
export interface AgentNodeData extends Record<string, unknown>, NoteFields {
  label: string;
  namespace: string;
  phase?: string;
  onClick?: () => void;
}

export interface PromptPackNodeData extends Record<string, unknown>, NoteFields {
  label: string;
  namespace: string;
  version?: string;
  phase?: string;
  onClick?: () => void;
}

export interface ToolRegistryNodeData extends Record<string, unknown>, NoteFields {
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
export const AgentNodeComponent = memo(({ data }: Readonly<CustomNodeProps<AgentNodeData>>) => {
  return (
    <div className="relative group">
      <Handle type="target" position={Position.Left} className="!bg-blue-500" />
      <button
        type="button"
        className={cn(baseNodeStyles, typeColors.agent)}
        onClick={data.onClick}
      >
        <div className="flex items-center gap-2">
          <Bot className="h-4 w-4 text-blue-600 dark:text-blue-400" />
          <div className="flex flex-col flex-1">
            <span className="font-medium text-sm">{data.label}</span>
            <span className="text-xs text-muted-foreground">{data.namespace}</span>
          </div>
          <StatusDot status={data.phase} />
        </div>
      </button>
      <Handle type="source" position={Position.Right} className="!bg-blue-500" />

      {/* Post-it note or add button */}
      {data.note ? (
        <PostItNote
          note={data.note}
          onEdit={() => data.onNoteEdit?.("agent", data.namespace, data.label)}
          onDelete={() => data.onNoteDelete?.("agent", data.namespace, data.label)}
        />
      ) : (
        <AddNoteButton
          onClick={() => data.onNoteEdit?.("agent", data.namespace, data.label)}
        />
      )}
    </div>
  );
});
AgentNodeComponent.displayName = "AgentNodeComponent";

// PromptPack Node Component
export const PromptPackNodeComponent = memo(({ data }: Readonly<CustomNodeProps<PromptPackNodeData>>) => {
  return (
    <div className="relative group">
      <Handle type="target" position={Position.Left} className="!bg-purple-500" />
      <button
        type="button"
        className={cn(baseNodeStyles, typeColors.promptPack)}
        onClick={data.onClick}
      >
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
      </button>
      <Handle type="source" position={Position.Right} className="!bg-purple-500" />

      {/* Post-it note or add button */}
      {data.note ? (
        <PostItNote
          note={data.note}
          onEdit={() => data.onNoteEdit?.("promptpack", data.namespace, data.label)}
          onDelete={() => data.onNoteDelete?.("promptpack", data.namespace, data.label)}
        />
      ) : (
        <AddNoteButton
          onClick={() => data.onNoteEdit?.("promptpack", data.namespace, data.label)}
        />
      )}
    </div>
  );
});
PromptPackNodeComponent.displayName = "PromptPackNodeComponent";

// ToolRegistry Node Component
export const ToolRegistryNodeComponent = memo(({ data }: Readonly<CustomNodeProps<ToolRegistryNodeData>>) => {
  return (
    <div className="relative group">
      <Handle type="target" position={Position.Left} className="!bg-orange-500" />
      <button
        type="button"
        className={cn(baseNodeStyles, typeColors.toolRegistry)}
        onClick={data.onClick}
      >
        <div className="flex items-center gap-2">
          <Package className="h-4 w-4 text-orange-600 dark:text-orange-400" />
          <div className="flex flex-col flex-1">
            <span className="font-medium text-sm">{data.label}</span>
            <span className="text-xs text-muted-foreground">
              {data.toolCount === undefined ? data.namespace : `${data.toolCount} tools`}
            </span>
          </div>
          <StatusDot status={data.phase} />
        </div>
      </button>
      <Handle type="source" position={Position.Right} className="!bg-orange-500" />

      {/* Post-it note or add button */}
      {data.note ? (
        <PostItNote
          note={data.note}
          onEdit={() => data.onNoteEdit?.("toolregistry", data.namespace, data.label)}
          onDelete={() => data.onNoteDelete?.("toolregistry", data.namespace, data.label)}
        />
      ) : (
        <AddNoteButton
          onClick={() => data.onNoteEdit?.("toolregistry", data.namespace, data.label)}
        />
      )}
    </div>
  );
});
ToolRegistryNodeComponent.displayName = "ToolRegistryNodeComponent";

// Tool Node Component
export const ToolNodeComponent = memo(({ data }: Readonly<CustomNodeProps<ToolNodeData>>) => {
  return (
    <div className="relative">
      <Handle type="target" position={Position.Left} className="!bg-teal-500" />
      <button
        type="button"
        className={cn(baseNodeStyles, typeColors.tool)}
        onClick={data.onClick}
      >
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
      </button>
    </div>
  );
});
ToolNodeComponent.displayName = "ToolNodeComponent";

// Prompt Node Component (individual prompts within a PromptPack)
export const PromptNodeComponent = memo(({ data }: Readonly<CustomNodeProps<PromptNodeData>>) => {
  return (
    <div className="relative">
      <Handle type="target" position={Position.Left} className="!bg-violet-500" />
      <button
        type="button"
        className={cn(baseNodeStyles, typeColors.prompt)}
        onClick={data.onClick}
      >
        <div className="flex items-center gap-2">
          <MessageSquare className="h-4 w-4 text-violet-600 dark:text-violet-400" />
          <span className="font-medium text-sm">{data.label}</span>
        </div>
      </button>
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
