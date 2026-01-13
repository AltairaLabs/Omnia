"use client";

import { useCallback, useMemo, useEffect } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { nodeTypes } from "./nodes";
import { buildTopologyGraph, applyLayoutToGraph } from "./graph-builder";
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
}: Readonly<TopologyGraphProps>) {
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

    async function applyLayout() {
      const layoutedGraph = await applyLayoutToGraph(initialGraph);
      if (!cancelled) {
        setNodes(layoutedGraph.nodes);
        setEdges(layoutedGraph.edges);
      }
    }

    applyLayout();

    return () => {
      cancelled = true;
    };
  }, [initialGraph, setNodes, setEdges]);

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
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.2}
        maxZoom={2}
        defaultEdgeOptions={{
          type: "smoothstep",
        }}
      >
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
