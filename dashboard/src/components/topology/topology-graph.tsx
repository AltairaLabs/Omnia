"use client";

import { useCallback, useMemo, useEffect, useState } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  Panel,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { nodeTypes } from "./nodes";
import { buildTopologyGraph, applyLayoutToGraph } from "./graph-builder";
import { usePersistedNodeLayout } from "@/hooks/use-persisted-node-layout";
import type { AgentRuntime, PromptPack, ToolRegistry, Provider } from "@/types";
import type { NotesMap } from "@/lib/notes-storage";

interface TopologyGraphProps {
  agents: AgentRuntime[];
  promptPacks: PromptPack[];
  toolRegistries: ToolRegistry[];
  providers: Provider[];
  onNodeClick?: (type: string, name: string, namespace: string) => void;
  notes?: NotesMap;
  onNoteEdit?: (type: string, namespace: string, name: string) => void;
  onNoteDelete?: (type: string, namespace: string, name: string) => void;
  className?: string;
  /** localStorage key for persisted drag layout (default "topology"). */
  storageKey?: string;
}

export function TopologyGraph({
  agents,
  promptPacks,
  toolRegistries,
  providers,
  onNodeClick,
  notes,
  onNoteEdit,
  onNoteDelete,
  className,
  storageKey = "topology",
}: Readonly<TopologyGraphProps>) {
  const { applyLayout, onNodeDragStop, reset } = usePersistedNodeLayout(storageKey);
  const [layoutNonce, setLayoutNonce] = useState(0);
  // Build the initial graph
  const initialGraph = useMemo(
    () => {
      const graph = buildTopologyGraph({
        agents,
        promptPacks,
        toolRegistries,
        providers,
        onNodeClick,
        notes,
        onNoteEdit,
        onNoteDelete,
      });
      return graph;
    },
    [agents, promptPacks, toolRegistries, providers, onNodeClick, notes, onNoteEdit, onNoteDelete]
  );

  const [nodes, setNodes, onNodesChange] = useNodesState(initialGraph.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialGraph.edges);

  // Apply ELK layout asynchronously for better edge routing
  useEffect(() => {
    let cancelled = false;

    async function runLayout() {
      const layoutedGraph = await applyLayoutToGraph(initialGraph);
      if (!cancelled) {
        // overlay any saved drag positions on the fresh elk layout
        setNodes(applyLayout(layoutedGraph.nodes));
        setEdges(layoutedGraph.edges);
      }
    }

    runLayout();

    return () => {
      cancelled = true;
    };
  }, [initialGraph, setNodes, setEdges, applyLayout, layoutNonce]);

  // Custom mini-map node color
  const nodeColor = useCallback((node: Node) => {
    switch (node.type) {
      case "agent":
        return "#3b82f6"; // blue
      case "promptPack":
        return "#8b5cf6"; // purple
      case "toolRegistry":
        return "#f97316"; // orange
      case "tool":
        return "#14b8a6"; // teal
      case "prompt":
        return "#a855f7"; // violet
      case "provider":
        return "#22c55e"; // green
      default:
        return "#6b7280"; // gray
    }
  }, []);

  return (
    <div style={{ width: "100%", height: "600px" }} className={className}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeDragStop={onNodeDragStop}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.2}
        maxZoom={2}
        defaultEdgeOptions={{
          type: "smoothstep",
        }}
      >
        <Panel position="bottom-right">
          <button
            type="button"
            onClick={() => { reset(); setLayoutNonce((n) => n + 1); }}
            className="text-xs text-muted-foreground bg-background/85 backdrop-blur rounded border px-2 py-1 hover:text-foreground"
          >
            Reset layout
          </button>
        </Panel>
        <Controls />
        <MiniMap
          nodeColor={nodeColor}
          nodeStrokeWidth={3}
          zoomable
          pannable
        />
        <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
      </ReactFlow>
    </div>
  );
}
